package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the gateway service over a real local TLS connection, not a canned transport.
// This is a compatible-gateway integration test, not a live CommandCode test.
type commandCodeHTTPTransport struct{ client *http.Client }

func (u *commandCodeHTTPTransport) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
}
func (u *commandCodeHTTPTransport) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

func TestCommandCodeGatewayHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, streaming := range []bool{false, true} {
		name := "json"
		if streaming {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			type received struct{ path, auth, body string }
			requests := make(chan received, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				requests <- received{req.URL.Path, req.Header.Get("Authorization"), string(body)}
				w.Header().Set("X-Request-Id", "commandcode-local-test")
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_local\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"resp_local","output":[{"type":"message","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`)
				}
			}))
			defer upstream.Close()
			body := []byte(`{"model":"commandcode-test-model","stream":false,"input":"ping"}`)
			if streaming {
				body = []byte(`{"model":"commandcode-test-model","stream":true,"input":"ping"}`)
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &commandCodeHTTPTransport{client: upstream.Client()}}
			account := &Account{ID: 456, Name: "commandcode-local-test", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": "test-gateway-key", "base_url": upstream.URL},
				Extra:       map[string]any{"provider": "commandcode_gateway", "openai_passthrough": true, "openai_apikey_responses_websockets_v2_mode": "off"},
				Status:      StatusActive, Schedulable: true, RateMultiplier: f64p(1)}
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), "pong")
			select {
			case got := <-requests:
				require.Equal(t, "/v1/responses", got.path)
				require.Equal(t, "Bearer test-gateway-key", got.auth)
				require.JSONEq(t, string(body), got.body)
			default:
				t.Fatal("No upstream request received")
			}
			if streaming {
				require.Contains(t, rec.Body.String(), "response.completed")
			}
		})
	}
}
