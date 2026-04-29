package normalapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"GPTBridge/internal/biz/contracts"
	"GPTBridge/internal/domain/proxy/entity"
	"go.uber.org/zap"
)

func TestForwardUsesRouteCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer route-key" {
			t.Fatalf("Authorization = %q, want route key", got)
		}
		if got := r.URL.Path; got != "/v1/models" {
			t.Fatalf("path = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  "default-key",
		Timeout: time.Second,
	}, zap.NewNop())

	resp, err := client.Forward(context.Background(), contracts.Route{
		SourceType: contracts.SourceTypeNormal,
		BaseURL:    server.URL,
		APIKey:     "route-key",
	}, entity.ProxyRequest{
		Method: http.MethodGet,
		Path:   "/v1/models",
	})
	if err != nil {
		t.Fatalf("Forward error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	_, _ = io.ReadAll(resp.Body)
}
