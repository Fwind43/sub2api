package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Opt-in direct account test. No CPA gateway or loopback upstream bridge.
// This does not cover public authentication, database persistence, or deployment.
func TestCommandCodeNativeLive(t *testing.T) {
	if os.Getenv("COMMANDCODE_NATIVE_LIVE_TEST") != "1" {
		t.Skip("native live opt-in required; skip is not live acceptance")
	}
	key := os.Getenv("COMMANDCODE_NATIVE_API_KEY")
	model := os.Getenv("COMMANDCODE_NATIVE_MODEL")
	if key == "" || model == "" {
		t.Fatal("native API key and model are required; CPA credentials are not a fallback")
	}
	gin.SetMode(gin.TestMode)
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 60 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 70 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("responses_stream=%t", stream), func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"model": model, "input": "Reply exactly pong.", "max_output_tokens": 128, "stream": stream})
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
			c.Request.Header.Set("Content-Type", "application/json")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &commandCodeHTTPTransport{client: client}}
			account := &Account{ID: 901, Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": key}, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1)}
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			result, err := svc.Forward(ctx, c, account, body)
			if err != nil {
				t.Fatalf("native forward failed: %s", strings.ReplaceAll(err.Error(), key, "[REDACTED]"))
			}
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			output := rec.Body.String()
			require.True(t, strings.Contains(strings.ToLower(output), "pong"), "native response lacks expected text")
			if stream {
				require.True(t, strings.Contains(output, "response.completed"), "missing completion event")
			} else {
				require.True(t, json.Valid(rec.Body.Bytes()), "invalid JSON response")
			}
			t.Logf("DIRECT_NATIVE_HTTP_STATUS=%d STREAM=%t EXPECTED_TEXT=true", rec.Code, stream)
		})
	}
}
