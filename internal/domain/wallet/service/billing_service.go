package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	walletentity "GPTBridge/internal/domain/wallet/entity"
	walletrepository "GPTBridge/internal/domain/wallet/repository"
	"GPTBridge/internal/infra/config"
	"GPTBridge/internal/infra/logging"
	"GPTBridge/internal/infra/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrMissingAPIKey       = errors.New("缺少 API Key")
	ErrInvalidAPIKey       = errors.New("API Key 无效")
	ErrDisabledAPIKey      = errors.New("API Key 已禁用")
	ErrInsufficientBalance = errors.New("余额不足")
)

// BillingService 负责 API Key 鉴权、用量解析和余额扣减。
type BillingService struct {
	enabled       bool
	requireAPIKey bool
	defaultModel  string
	defaultPrice  config.ModelPricing
	prices        map[string]config.ModelPricing
	repository    walletrepository.WalletRepository
	logger        *zap.Logger
}

// NewBillingService 创建计费服务。
func NewBillingService(cfg config.BillingConfig, repository walletrepository.WalletRepository, logger *zap.Logger) *BillingService {
	return &BillingService{
		enabled:       cfg.Enabled,
		requireAPIKey: cfg.RequireAPIKey,
		defaultModel:  cfg.DefaultModel,
		defaultPrice: config.ModelPricing{
			InputPricePer1K:  cfg.DefaultInputPricePer1K,
			OutputPricePer1K: cfg.DefaultOutputPricePer1K,
			RequestPrice:     cfg.RequestPrice,
		},
		prices:     cfg.Models,
		repository: repository,
		logger:     logger,
	}
}

// Enabled 返回是否启用计费。
func (s *BillingService) Enabled() bool {
	return s != nil && s.enabled
}

// RequireAPIKey 返回是否强制要求平台 API Key。
func (s *BillingService) RequireAPIKey() bool {
	return s.Enabled() && s.requireAPIKey
}

// Authenticate 校验 Authorization 中的平台 API Key。
func (s *BillingService) Authenticate(header http.Header) (walletentity.APIKeyAccount, error) {
	if !s.Enabled() {
		return walletentity.APIKeyAccount{}, nil
	}

	key := bearerToken(header.Get("Authorization"))
	if key == "" {
		return walletentity.APIKeyAccount{}, ErrMissingAPIKey
	}

	account, err := s.repository.FindAccountByAPIKey(context.Background(), key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return walletentity.APIKeyAccount{}, ErrInvalidAPIKey
	}
	if err != nil {
		return walletentity.APIKeyAccount{}, err
	}
	if !account.Enabled {
		return walletentity.APIKeyAccount{}, ErrDisabledAPIKey
	}
	if account.Balance <= 0 {
		return walletentity.APIKeyAccount{}, ErrInsufficientBalance
	}
	return account, nil
}

// Charge 根据上游响应中的 usage 扣费。
func (s *BillingService) Charge(ctx context.Context, account walletentity.APIKeyAccount, requestBody []byte, responseBody []byte) (walletentity.UsageRecord, error) {
	if !s.Enabled() || account.Key == "" {
		return walletentity.UsageRecord{}, nil
	}

	model := modelFromRequest(requestBody)
	usage := usageFromResponse(responseBody)
	if usage.Model != "" {
		model = usage.Model
	}
	if model == "" {
		model = s.defaultModel
	}
	usage.Model = model

	price := s.priceForModel(model)
	cost := price.RequestPrice +
		float64(usage.InputTokens)/1000*price.InputPricePer1K +
		float64(usage.OutputTokens)/1000*price.OutputPricePer1K

	record, err := s.repository.ChargeAccount(ctx, account, usage, cost, trace.TraceIDFromContext(ctx))
	if err != nil {
		return walletentity.UsageRecord{}, err
	}

	logging.WithContext(s.logger, ctx).Info("请求计费完成",
		zap.String("account", record.AccountName),
		zap.String("model", model),
		zap.Int("input_tokens", usage.InputTokens),
		zap.Int("output_tokens", usage.OutputTokens),
		zap.Float64("cost", cost),
		zap.Float64("balance_after", record.BalanceAfter),
	)
	return record, nil
}

func (s *BillingService) priceForModel(model string) config.ModelPricing {
	if price, ok := s.prices[model]; ok {
		return price
	}
	if price, ok := s.prices[s.defaultModel]; ok {
		return price
	}
	return s.defaultPrice
}

func bearerToken(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return value
}

func modelFromRequest(body []byte) string {
	var payload struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &payload)
	return payload.Model
}

func usageFromResponse(body []byte) walletentity.Usage {
	if bytes.Contains(body, []byte("data:")) {
		return usageFromSSE(body)
	}
	return usageFromJSON(body)
}

func usageFromJSON(body []byte) walletentity.Usage {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return walletentity.Usage{}
	}
	return usageFromMap(payload)
}

func usageFromSSE(body []byte) walletentity.Usage {
	var usage walletentity.Usage
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		next := usageFromMap(payload)
		if next.Model != "" {
			usage.Model = next.Model
		}
		if next.TotalTokens > 0 || next.InputTokens > 0 || next.OutputTokens > 0 {
			usage.InputTokens = next.InputTokens
			usage.OutputTokens = next.OutputTokens
			usage.TotalTokens = next.TotalTokens
		}
	}
	return usage
}

func usageFromMap(payload map[string]any) walletentity.Usage {
	usageMap, _ := payload["usage"].(map[string]any)
	usage := walletentity.Usage{
		Model: stringValue(payload["model"]),
	}
	if usageMap == nil {
		return usage
	}

	usage.InputTokens = intValue(usageMap["prompt_tokens"])
	if usage.InputTokens == 0 {
		usage.InputTokens = intValue(usageMap["input_tokens"])
	}
	usage.OutputTokens = intValue(usageMap["completion_tokens"])
	if usage.OutputTokens == 0 {
		usage.OutputTokens = intValue(usageMap["output_tokens"])
	}
	usage.TotalTokens = intValue(usageMap["total_tokens"])
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens > 0 {
		usage.InputTokens = usage.TotalTokens
	}
	return usage
}

func intValue(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
