package proxygateway

import (
	"testing"

	"GPTBridge/internal/biz/contracts"
	"GPTBridge/internal/infra/config"
)

func TestSuccessWithoutUsageSettlementPolicy(t *testing.T) {
	gateway := &Gateway{cfg: config.Config{Billing: config.BillingConfig{
		DefaultInputPricePer1K:  0.001,
		DefaultOutputPricePer1K: 0.002,
		RequestPrice:            0,
	}}}

	final, commit := gateway.finalCredits(contracts.RequestFeatures{
		SettlementPolicy:      settlementRelease,
		EstimatedMicroCredits: 99,
	}, contracts.Usage{})
	if commit || final != 0 {
		t.Fatalf("release policy final=%d commit=%v, want zero release", final, commit)
	}

	final, commit = gateway.finalCredits(contracts.RequestFeatures{
		SettlementPolicy:      settlementEstimate,
		EstimatedMicroCredits: 99,
	}, contracts.Usage{})
	if !commit || final != 99 {
		t.Fatalf("estimate policy final=%d commit=%v, want estimated commit", final, commit)
	}

	final, commit = gateway.finalCredits(contracts.RequestFeatures{
		SettlementPolicy: settlementMinimumFee,
	}, contracts.Usage{})
	if !commit || final <= 0 {
		t.Fatalf("minimum policy final=%d commit=%v, want positive commit", final, commit)
	}
}

func TestStatefulRefsAndObservedResources(t *testing.T) {
	refs := statefulRefs(map[string]any{
		"previous_response_id": "resp_1",
		"conversation": map[string]any{
			"id": "conv_1",
		},
		"input": []any{
			map[string]any{"file_id": "file_1"},
		},
	})
	if len(refs) != 3 {
		t.Fatalf("refs len = %d, want 3: %#v", len(refs), refs)
	}

	resources := observedResources("/v1/responses", []byte(`{"id":"resp_2","conversation":{"id":"conv_2"}}`))
	if len(resources) != 2 {
		t.Fatalf("resources len = %d, want 2: %#v", len(resources), resources)
	}
}
