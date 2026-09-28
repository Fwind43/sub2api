package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Redirect only the test transport, leaving the production endpoint selection intact.
type commandCodeNativeLocalTransport struct {
	client   *http.Client
	target   *url.URL
	requests chan string
}

func (u *commandCodeNativeLocalTransport) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.requests <- fmt.Sprintf("%d %s %s", id, req.Method, req.URL.String())
	clone := req.Clone(req.Context())
	clone.URL.Scheme = u.target.Scheme
	clone.URL.Host = u.target.Host
	clone.Host = u.target.Host
	return u.client.Do(clone)
}
func (u *commandCodeNativeLocalTransport) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

func TestCommandCodeNativeServiceHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for index, key := range []string{"native-account-one", "native-account-two"} {
			t.Run(fmt.Sprintf("stream=%t/account=%d", stream, index), func(t *testing.T) {
				type received struct{ path, auth, body string }
				seen := make(chan received, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					b, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, err.Error(), 400)
						return
					}
					seen <- received{r.URL.Path, r.Header.Get("Authorization"), string(b)}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintln(w, `data: {"type":"text-delta","text":"native pong"}`)
					fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop","usage":{"inputTokens":3,"outputTokens":2}}`)
					fmt.Fprintln(w, "data: [DONE]")
				}))
				defer upstream.Close()
				target, err := url.Parse(upstream.URL)
				require.NoError(t, err)
				transport := &commandCodeNativeLocalTransport{client: upstream.Client(), target: target, requests: make(chan string, 1)}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: transport}
				account := &Account{ID: int64(701 + index), Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": key}}
				body := []byte(fmt.Sprintf(`{"model":"test/model","stream":%t,"messages":[{"role":"user","content":"ping"}]}`, stream))
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
				resp, err := svc.sendCommandCodeRequest(context.Background(), c, account, body, stream)
				require.NoError(t, err)
				require.NotNil(t, resp)
				defer resp.Body.Close()
				output, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.Contains(t, string(output), "native pong")
				require.Contains(t, string(output), `"prompt_tokens":3`)
				require.Contains(t, string(output), `"completion_tokens":2`)
				if stream {
					require.Contains(t, string(output), "data: [DONE]")
				} else {
					require.Contains(t, string(output), `"object":"chat.completion"`)
				}
				got := <-seen
				require.Equal(t, "/alpha/generate", got.path)
				require.Equal(t, "Bearer "+key, got.auth)
				require.Contains(t, got.body, `"params"`)
				require.Contains(t, got.body, `"model":"test/model"`)
				require.Equal(t, fmt.Sprintf("%d POST https://api.commandcode.ai/alpha/generate", account.ID), <-transport.requests)
			})
		}
	}
}

func TestCommandCodeNativeChatEntryHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for index, key := range []string{"native-account-one", "native-account-two"} {
			t.Run(fmt.Sprintf("stream=%t/account=%d", stream, index), func(t *testing.T) {
				type received struct{ path, auth, body string }
				seen := make(chan received, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					b, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, err.Error(), 400)
						return
					}
					seen <- received{r.URL.Path, r.Header.Get("Authorization"), string(b)}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintln(w, `data: {"type":"text-delta","text":"native pong"}`)
					fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop","usage":{"inputTokens":3,"outputTokens":2}}`)
					fmt.Fprintln(w, "data: [DONE]")
				}))
				defer upstream.Close()
				target, err := url.Parse(upstream.URL)
				require.NoError(t, err)
				transport := &commandCodeNativeLocalTransport{client: upstream.Client(), target: target, requests: make(chan string, 1)}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: transport}
				account := &Account{ID: int64(701 + index), Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": key}}
				body := []byte(fmt.Sprintf(`{"model":"test/model","stream":%t,"messages":[{"role":"user","content":"ping"}]}`, stream))
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
				result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, http.StatusOK, rec.Code)
				output := rec.Body.String()
				require.Contains(t, output, "native pong")
				if stream {
					require.Contains(t, output, "data: [DONE]")
				} else {
					require.Contains(t, output, `"object":"chat.completion"`)
				}
				got := <-seen
				require.Equal(t, "/alpha/generate", got.path)
				require.Equal(t, "Bearer "+key, got.auth)
				require.Contains(t, got.body, `"params"`)
				require.Contains(t, got.body, `"model":"test/model"`)
				require.Equal(t, fmt.Sprintf("%d POST https://api.commandcode.ai/alpha/generate", account.ID), <-transport.requests)
			})
		}
	}
}

func TestCommandCodeNativeResponsesEntryHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for index, key := range []string{"native-account-one", "native-account-two"} {
			t.Run(fmt.Sprintf("stream=%t/account=%d", stream, index), func(t *testing.T) {
				type received struct{ path, auth, body string }
				seen := make(chan received, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					b, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, err.Error(), 400)
						return
					}
					seen <- received{r.URL.Path, r.Header.Get("Authorization"), string(b)}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintln(w, `data: {"type":"text-delta","text":"native pong"}`)
					fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop","usage":{"inputTokens":3,"outputTokens":2}}`)
					fmt.Fprintln(w, "response.completed")
				}))
				defer upstream.Close()
				target, err := url.Parse(upstream.URL)
				require.NoError(t, err)
				transport := &commandCodeNativeLocalTransport{client: upstream.Client(), target: target, requests: make(chan string, 1)}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: transport}
				account := &Account{ID: int64(701 + index), Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": key}}
				body := []byte(fmt.Sprintf(`{"model":"test/model","stream":%t,"input":"ping"}`, stream))
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, http.StatusOK, rec.Code)
				output := rec.Body.String()
				require.Contains(t, output, "native pong")
				if stream {
					require.Contains(t, output, "response.completed")
				} else {
					require.Contains(t, output, `"object":"response"`)
				}
				got := <-seen
				require.Equal(t, "/alpha/generate", got.path)
				require.Equal(t, "Bearer "+key, got.auth)
				require.Contains(t, got.body, `"params"`)
				require.Contains(t, got.body, `"model":"test/model"`)
				require.Equal(t, fmt.Sprintf("%d POST https://api.commandcode.ai/alpha/generate", account.ID), <-transport.requests)
			})
		}
	}
}
