package proxygateway

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"GPTBridge/internal/biz/contracts"
	billingrepository "GPTBridge/internal/domain/billing/repository"
	billingservice "GPTBridge/internal/domain/billing/service"
	"GPTBridge/internal/domain/proxy/entity"
	proxyservice "GPTBridge/internal/domain/proxy/service"
	upstreamrepository "GPTBridge/internal/domain/upstream/repository"
	upstreamservice "GPTBridge/internal/domain/upstream/service"
	"GPTBridge/internal/infra/config"
	"GPTBridge/internal/infra/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	policyVersion        = "mvp-1"
	settlementRelease    = "release"
	settlementEstimate   = "commit_estimate"
	settlementMinimumFee = "minimum_fee"
)

var (
	ErrUpstreamQuotaExhausted = errors.New("上游池容量耗尽")
	ErrStatefulUnavailable    = errors.New("stateful 资源原上游不可用")
)

type Gateway struct {
	db     *gorm.DB
	cfg    config.Config
	proxy  *proxyservice.ProxyService
	logger *zap.Logger
}

type Run struct {
	Response              *http.Response
	Route                 contracts.Route
	Features              contracts.RequestFeatures
	CustomerAccountID     uint
	BillingReservationID  string
	UpstreamReservationID string
}

func NewGateway(db *gorm.DB, cfg config.Config, proxy *proxyservice.ProxyService, logger *zap.Logger) *Gateway {
	return &Gateway{db: db, cfg: cfg, proxy: proxy, logger: logger}
}

func (g *Gateway) Start(ctx context.Context, req entity.ProxyRequest) (*Run, error) {
	features := g.parseFeatures(ctx, req)
	var run Run

	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		billing := billingservice.NewService(g.cfg.Billing, billingrepository.NewGormBillingRepository(tx), g.logger)
		upstream := upstreamservice.NewRouterService(upstreamrepository.NewGormUpstreamRepository(tx), g.logger)

		account, err := billing.AuthenticateHeader(ctx, http.Header(req.Headers))
		if err != nil {
			return err
		}
		billingReservation, soldCapacity, err := billing.ResolveAndReserve(ctx, account, features)
		if err != nil {
			return err
		}
		routeLease, err := upstream.ResolveAndReserve(ctx, upstreamservice.ResolveInput{
			CustomerAccountID:        account.ID,
			SoldCapacityMicroCredits: soldCapacity,
			Features:                 features,
		})
		if err != nil {
			return err
		}

		run = Run{
			Route:                 routeLease.Route,
			Features:              features,
			CustomerAccountID:     account.ID,
			BillingReservationID:  billingReservation.ReservationID,
			UpstreamReservationID: routeLease.ReservationID,
		}
		return nil
	})
	if err != nil {
		return nil, g.normalizeStartError(err)
	}

	resp, err := g.proxy.Forward(ctx, run.Route, req)
	if err != nil {
		_ = g.release(ctx, &run, "proxy_forward_error")
		_ = g.observeFailure(ctx, &run, 0, err.Error())
		return nil, err
	}
	run.Response = resp
	return &run, nil
}

func (g *Gateway) Finalize(ctx context.Context, run *Run, statusCode int, responseBody []byte, streamErr error) error {
	if run == nil {
		return nil
	}
	if streamErr != nil {
		_ = g.release(ctx, run, "stream_error")
		return streamErr
	}
	if statusCode >= http.StatusBadRequest {
		if err := g.release(ctx, run, "upstream_error"); err != nil {
			return err
		}
		return g.observeFailure(ctx, run, statusCode, string(responseBody))
	}

	usage := usageFromResponse(responseBody)
	finalMicroCredits, shouldCommit := g.finalCredits(run.Features, usage)
	if !shouldCommit {
		return g.release(ctx, run, "zero_cost_policy")
	}
	resources := observedResources(run.Features.Endpoint, responseBody)

	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		billing := billingservice.NewService(g.cfg.Billing, billingrepository.NewGormBillingRepository(tx), g.logger)
		upstream := upstreamservice.NewRouterService(upstreamrepository.NewGormUpstreamRepository(tx), g.logger)
		if err := billing.CommitUsage(ctx, run.BillingReservationID, finalMicroCredits); err != nil {
			return err
		}
		if err := upstream.CommitCapacity(ctx, run.UpstreamReservationID, finalMicroCredits); err != nil {
			return err
		}
		return upstream.RecordResourceOwners(ctx, run.Route, run.CustomerAccountID, run.Features.RequestID, resources)
	})
}

func (g *Gateway) release(ctx context.Context, run *Run, reason string) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		billing := billingservice.NewService(g.cfg.Billing, billingrepository.NewGormBillingRepository(tx), g.logger)
		upstream := upstreamservice.NewRouterService(upstreamrepository.NewGormUpstreamRepository(tx), g.logger)
		if err := billing.ReleaseReservation(ctx, run.BillingReservationID, reason); err != nil {
			return err
		}
		return upstream.ReleaseCapacity(ctx, run.UpstreamReservationID, reason)
	})
}

func (g *Gateway) observeFailure(ctx context.Context, run *Run, statusCode int, body string) error {
	upstream := upstreamservice.NewRouterService(upstreamrepository.NewGormUpstreamRepository(g.db), g.logger)
	return upstream.ObserveFailure(ctx, run.Route, contracts.UpstreamFailure{
		StatusCode: statusCode,
		Body:       body,
		OccurredAt: time.Now(),
	})
}

func (g *Gateway) parseFeatures(ctx context.Context, req entity.ProxyRequest) contracts.RequestFeatures {
	requestID := firstHeader(req.Headers, trace.HeaderRequestID)
	if requestID == "" {
		requestID = newID()
	}
	traceID := trace.TraceIDFromContext(ctx)
	if traceID == "" {
		traceID = firstHeader(req.Headers, trace.HeaderTraceID)
	}

	model, payload := modelFromRequest(req.Payload)
	policy := endpointPolicy(req.Path)
	estimated := g.estimateMicroCredits(req.Path, model, req.Payload, payload, policy)
	priceSnapshot := g.priceSnapshot(model, policy, estimated)

	return contracts.RequestFeatures{
		RequestID:             requestID,
		TraceID:               traceID,
		Method:                req.Method,
		Endpoint:              endpointOnly(req.Path),
		Model:                 model,
		EstimatedMicroCredits: estimated,
		SettlementPolicy:      policy,
		PolicyVersion:         policyVersion,
		PriceSnapshot:         priceSnapshot,
		StatefulRefs:          statefulRefs(payload),
		ExpectedResources:     expectedResources(req.Path),
		ExpiresAt:             time.Now().Add(15 * time.Minute),
	}
}

func (g *Gateway) normalizeStartError(err error) error {
	if errors.Is(err, upstreamrepository.ErrNoAvailablePool) {
		return ErrUpstreamQuotaExhausted
	}
	if errors.Is(err, upstreamrepository.ErrStatefulRouteUnavailable) {
		return ErrStatefulUnavailable
	}
	return err
}

func (g *Gateway) estimateMicroCredits(endpoint string, model string, body []byte, payload map[string]any, policy string) int64 {
	if policy == settlementRelease {
		return 1
	}
	price := g.priceForModel(model)
	inputTokens := len(body) / 4
	if inputTokens < 1 {
		inputTokens = 1
	}
	outputTokens := intValue(payload["max_tokens"])
	if outputTokens == 0 {
		outputTokens = intValue(payload["max_completion_tokens"])
	}
	if outputTokens == 0 {
		outputTokens = intValue(payload["max_output_tokens"])
	}
	if outputTokens == 0 && strings.Contains(endpointOnly(endpoint), "images") {
		outputTokens = 1000
	}
	requestCost := price.RequestPrice
	credits := requestCost +
		float64(inputTokens)/1000*price.InputPricePer1K +
		float64(outputTokens)/1000*price.OutputPricePer1K
	micro := creditsToMicro(credits)
	if micro <= 0 {
		if policy == settlementMinimumFee {
			return 1
		}
		return 1
	}
	return micro
}

func (g *Gateway) finalCredits(features contracts.RequestFeatures, usage contracts.Usage) (int64, bool) {
	if usage.TotalTokens > 0 || usage.InputTokens > 0 || usage.OutputTokens > 0 {
		price := g.priceForModel(firstNonEmpty(usage.Model, features.Model))
		credits := price.RequestPrice +
			float64(usage.InputTokens)/1000*price.InputPricePer1K +
			float64(usage.OutputTokens)/1000*price.OutputPricePer1K
		micro := creditsToMicro(credits)
		if micro <= 0 {
			micro = 1
		}
		return micro, true
	}
	switch features.SettlementPolicy {
	case settlementRelease:
		return 0, false
	case settlementMinimumFee:
		return maxInt64(1, creditsToMicro(g.priceForModel(features.Model).RequestPrice)), true
	default:
		return maxInt64(1, features.EstimatedMicroCredits), true
	}
}

func (g *Gateway) priceForModel(model string) config.ModelPricing {
	if model != "" {
		if price, ok := g.cfg.Billing.Models[model]; ok {
			return price
		}
	}
	if g.cfg.Billing.DefaultModel != "" {
		if price, ok := g.cfg.Billing.Models[g.cfg.Billing.DefaultModel]; ok {
			return price
		}
	}
	return config.ModelPricing{
		InputPricePer1K:  g.cfg.Billing.DefaultInputPricePer1K,
		OutputPricePer1K: g.cfg.Billing.DefaultOutputPricePer1K,
		RequestPrice:     g.cfg.Billing.RequestPrice,
	}
}

func (g *Gateway) priceSnapshot(model string, policy string, estimated int64) string {
	price := g.priceForModel(model)
	raw, _ := json.Marshal(map[string]any{
		"model":                   model,
		"input_price_per_1k":      price.InputPricePer1K,
		"output_price_per_1k":     price.OutputPricePer1K,
		"request_price":           price.RequestPrice,
		"settlement_policy":       policy,
		"policy_version":          policyVersion,
		"estimated_micro_credits": estimated,
	})
	return string(raw)
}

func endpointPolicy(path string) string {
	switch endpointOnly(path) {
	case "/health", "/v1/models", "/v1/files":
		return settlementRelease
	case "/v1/responses", "/v1/chat/completions", "/v1/images/generations", "/v1/images/edits":
		return settlementEstimate
	default:
		if strings.HasPrefix(endpointOnly(path), "/v1/") {
			return settlementMinimumFee
		}
		return settlementRelease
	}
}

func expectedResources(path string) []string {
	switch endpointOnly(path) {
	case "/v1/responses":
		return []string{"response_id", "conversation_id"}
	case "/v1/files":
		return []string{"file_id"}
	default:
		return nil
	}
}

func usageFromResponse(body []byte) contracts.Usage {
	if bytes.Contains(body, []byte("data:")) {
		return usageFromSSE(body)
	}
	return usageFromJSON(body)
}

func usageFromJSON(body []byte) contracts.Usage {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return contracts.Usage{}
	}
	return usageFromMap(payload)
}

func usageFromSSE(body []byte) contracts.Usage {
	var usage contracts.Usage
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

func usageFromMap(payload map[string]any) contracts.Usage {
	usageMap, _ := payload["usage"].(map[string]any)
	usage := contracts.Usage{
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

func observedResources(endpoint string, body []byte) []contracts.ObservedResource {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	var resources []contracts.ObservedResource
	id := stringValue(payload["id"])
	switch endpointOnly(endpoint) {
	case "/v1/responses":
		if id != "" {
			resources = append(resources, contracts.ObservedResource{ResourceType: "response_id", ResourceID: id})
		}
		if conversationID := conversationID(payload["conversation"]); conversationID != "" {
			resources = append(resources, contracts.ObservedResource{ResourceType: "conversation_id", ResourceID: conversationID})
		}
	case "/v1/files":
		if id != "" {
			resources = append(resources, contracts.ObservedResource{ResourceType: "file_id", ResourceID: id})
		}
	}
	return resources
}

func statefulRefs(payload map[string]any) []contracts.StatefulRef {
	if payload == nil {
		return nil
	}
	seen := make(map[string]bool)
	var refs []contracts.StatefulRef
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, item := range typed {
				switch key {
				case "previous_response_id":
					addRef(&refs, seen, "response_id", stringValue(item))
				case "conversation":
					addRef(&refs, seen, "conversation_id", conversationID(item))
				case "file_id":
					addRef(&refs, seen, "file_id", stringValue(item))
				case "file_ids":
					walk(item)
				default:
					walk(item)
				}
			}
		case []any:
			for _, item := range typed {
				if id := stringValue(item); id != "" {
					addRef(&refs, seen, "file_id", id)
				}
				walk(item)
			}
		}
	}
	walk(payload)
	return refs
}

func addRef(refs *[]contracts.StatefulRef, seen map[string]bool, resourceType string, resourceID string) {
	if resourceType == "" || resourceID == "" {
		return
	}
	key := resourceType + ":" + resourceID
	if seen[key] {
		return
	}
	seen[key] = true
	*refs = append(*refs, contracts.StatefulRef{ResourceType: resourceType, ResourceID: resourceID})
}

func modelFromRequest(body []byte) (string, map[string]any) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil
	}
	return stringValue(payload["model"]), payload
}

func conversationID(value any) string {
	if id := stringValue(value); id != "" {
		return id
	}
	if mapped, ok := value.(map[string]any); ok {
		return stringValue(mapped["id"])
	}
	return ""
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

func endpointOnly(path string) string {
	if index := strings.Index(path, "?"); index >= 0 {
		return path[:index]
	}
	return path
}

func firstHeader(headers map[string][]string, key string) string {
	for name, values := range headers {
		if strings.EqualFold(name, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func creditsToMicro(value float64) int64 {
	return int64(value * 1_000_000)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func newID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
