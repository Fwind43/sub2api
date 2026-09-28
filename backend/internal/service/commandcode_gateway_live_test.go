package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Opt-in live test: a loopback TLS bridge forwards unchanged to the real gateway.
// No mock responses; no database, billing persistence, or public handler coverage.
func TestCommandCodeGatewayLive(t *testing.T) {
	if os.Getenv("COMMANDCODE_LIVE_TEST") != "1" {
		t.Skip("explicit live test opt-in required")
	}
	key := os.Getenv("CPA_COMMANDCODE_API_KEY")
	require.NotEmpty(t, key)
	target, err := url.Parse(os.Getenv("COMMANDCODE_LIVE_BASE_URL"))
	require.NoError(t, err)
	require.NotEmpty(t, target.Host)
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) { originalDirector(req); req.Host = target.Host }
	model := os.Getenv("COMMANDCODE_LIVE_MODEL")
	require.NotEmpty(t, model, "explicit model required for live tests")
	proxy.Transport = &http.Transport{Proxy: nil, ResponseHeaderTimeout: 65 * time.Second}
	bridge := httptest.NewTLSServer(proxy)
	defer bridge.Close()
	client := bridge.Client()
	client.Timeout = 70 * time.Second
	gin.SetMode(gin.TestMode)
	for _, streaming := range []bool{false, true} {
		name := "json"
		if streaming {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			payload := map[string]any{"model": model, "input": "Reply exactly pong.", "max_output_tokens": 512, "stream": streaming}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &commandCodeHTTPTransport{client: client}}
			account := &Account{ID: 456, Name: "commandcode-live-test", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": key, "base_url": bridge.URL},
				Extra:       map[string]any{"provider": "commandcode_gateway", "openai_passthrough": true, "openai_apikey_responses_websockets_v2_mode": "off"},
				Status:      StatusActive, Schedulable: true, RateMultiplier: f64p(1)}
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			result, forwardErr := svc.Forward(ctx, c, account, body)
			text := strings.ReplaceAll(rec.Body.String(), key, "[REDACTED]")
			t.Logf("HTTP_STATUS=%d RESPONSE=%s", rec.Code, text)
			require.NoError(t, forwardErr)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, strings.ToLower(text), "pong")
			if streaming {
				require.Contains(t, text, "response.completed")
			} else {
				require.True(t, json.Valid(rec.Body.Bytes()))
			}
			info, marshalErr := json.Marshal(result)
			require.NoError(t, marshalErr)
			t.Logf("FORWARD_RESULT=%s", strings.ReplaceAll(string(info), key, "[REDACTED]"))
		})
	}
}
