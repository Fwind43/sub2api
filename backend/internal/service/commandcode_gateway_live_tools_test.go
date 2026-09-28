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
func TestCommandCodeGatewayLiveTools(t *testing.T) {
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

	forward := func(t *testing.T, payload map[string]any, streaming bool) map[string]any {
		t.Helper()
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &commandCodeHTTPTransport{client: client}}
		account := &Account{ID: 456, Name: "commandcode-live-tools", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
			Credentials: map[string]any{"api_key": key, "base_url": bridge.URL},
			Extra:       map[string]any{"provider": "commandcode_gateway", "openai_passthrough": true, "openai_apikey_responses_websockets_v2_mode": "off"},
			Status:      StatusActive, Schedulable: true, RateMultiplier: f64p(1)}
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		result, forwardErr := svc.Forward(ctx, c, account, body)
		text := strings.ReplaceAll(rec.Body.String(), key, "[REDACTED]")
		t.Logf("HTTP_STATUS=%d RESPONSE=%s", rec.Code, text)
		require.NoError(t, forwardErr)
		require.Equal(t, http.StatusOK, rec.Code)
		require.NotNil(t, result)
		info, err := json.Marshal(result)
		require.NoError(t, err)
		t.Logf("FORWARD_RESULT=%s", strings.ReplaceAll(string(info), key, "[REDACTED]"))
		var response map[string]any
		if !streaming {
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		} else {
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var event map[string]any
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil && event["type"] == "response.completed" {
					response, _ = event["response"].(map[string]any)
				}
			}
			require.Contains(t, text, "response.function_call_arguments")
		}
		require.NotNil(t, response, "missing complete response")
		require.Nil(t, response["error"])
		require.Equal(t, "completed", response["status"])
		return response
	}
	for _, streaming := range []bool{false, true} {
		name := "json"
		if streaming {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			instruction := "Call get_verification_code with label probe. Then reply with exactly the code returned by the tool. Do not invent a code."
			tools := []any{map[string]any{"type": "function", "name": "get_verification_code", "description": "Returns a verification code for the given label.", "strict": true, "parameters": map[string]any{"type": "object", "properties": map[string]any{"label": map[string]any{"type": "string"}}, "required": []string{"label"}, "additionalProperties": false}}}
			first := forward(t, map[string]any{"model": model, "input": instruction, "tools": tools, "tool_choice": map[string]any{"type": "function", "name": "get_verification_code"}, "max_output_tokens": 512, "stream": streaming}, streaming)
			items, ok := first["output"].([]any)
			require.True(t, ok)
			var call map[string]any
			for _, item := range items {
				m, ok := item.(map[string]any)
				if ok && m["type"] == "function_call" {
					require.Nil(t, call, "unexpected multiple calls")
					call = m
				}
			}
			require.NotNil(t, call, "no function call returned")
			require.Equal(t, "get_verification_code", call["name"])
			callID, ok := call["call_id"].(string)
			require.True(t, ok)
			require.NotEmpty(t, callID)
			args, ok := call["arguments"].(string)
			require.True(t, ok)
			var parsed map[string]any
			require.NoError(t, json.Unmarshal([]byte(args), &parsed))
			require.Equal(t, "probe", parsed["label"])
			const code = "CC_VERIFIED_7319"
			history := []any{map[string]any{"role": "user", "content": instruction}}
			history = append(history, items...)
			history = append(history, map[string]any{"type": "function_call_output", "call_id": callID, "output": code})
			second := forward(t, map[string]any{"model": model, "input": history, "tools": tools, "tool_choice": "none", "max_output_tokens": 512, "stream": false}, false)
			output, ok := second["output"].([]any)
			require.True(t, ok)
			var answer strings.Builder
			for _, item := range output {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				content, _ := m["content"].([]any)
				for _, part := range content {
					p, ok := part.(map[string]any)
					if ok && p["type"] == "output_text" {
						text, _ := p["text"].(string)
						answer.WriteString(text)
					}
				}
			}
			require.Equal(t, code, strings.TrimSpace(answer.String()))
			t.Logf("TOOL_ROUND_TRIP_PASS stream=%v call_id=%s arguments=%s answer=%s", streaming, callID, args, answer.String())
		})
	}
}
