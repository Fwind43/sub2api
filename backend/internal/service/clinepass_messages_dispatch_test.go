//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ClinePass upstream exposes only an OpenAI-compatible /chat/completions endpoint
// (and only implements streaming generation), so inbound /v1/messages must always
// be converted to Chat Completions - for API-key and OAuth accounts alike.
func TestResolveUpstreamProtocol_ClinePassAlwaysChatCompletions(t *testing.T) {
	cases := map[string]*Account{
		"apikey": {ID: 501, Platform: PlatformClinePass, Type: AccountTypeAPIKey},
		"oauth":  clinePassAccountTestAccount(502),
	}
	for name, account := range cases {
		t.Run(name, func(t *testing.T) {
			require.True(t, shouldForwardOpenAIResponsesViaRawChatCompletions(account))
			require.Equal(t, APIProtocolChatCompletions,
				resolveUpstreamProtocol(account, APIProtocolAnthropic, "claude-sonnet-4-5", nil))
		})
	}
}

// The group-level Messages dispatch mapping defaults (gpt-5.4 / gpt-5.3-codex) are
// openai-only; a ClinePass group must never rewrite a claude-* model.
func TestResolveMessagesDispatchModel_ClinePassSkipsOpenAIDefaults(t *testing.T) {
	group := &Group{Platform: PlatformClinePass}
	require.Empty(t, group.ResolveMessagesDispatchModel("claude-sonnet-4-5"))
	require.Empty(t, group.ResolveMessagesDispatchModel("claude-opus-4-1"))
}

func clinePassMessagesUpstreamSSE() string {
	return strings.Join([]string{
		`data: {"id":"chatcmpl_cp","object":"chat.completion.chunk","model":"cline-pass/anthropic/claude-sonnet-4-5","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_cp","object":"chat.completion.chunk","model":"cline-pass/anthropic/claude-sonnet-4-5","choices":[{"index":0,"delta":{"content":"he"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_cp","object":"chat.completion.chunk","model":"cline-pass/anthropic/claude-sonnet-4-5","choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_cp","object":"chat.completion.chunk","model":"cline-pass/anthropic/claude-sonnet-4-5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_cp","object":"chat.completion.chunk","model":"cline-pass/anthropic/claude-sonnet-4-5","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
}

func newClinePassMessagesTestContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

func newClinePassMessagesTestService(rid string) (*OpenAIGatewayService, *httpUpstreamRecorder) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{rid}},
		Body:       io.NopCloser(strings.NewReader(clinePassMessagesUpstreamSSE())),
	}}
	return &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}, upstream
}

// A non-streaming /v1/messages request against a ClinePass group: the upstream
// answer is always SSE, so the gateway has to aggregate it back into a single
// Anthropic Messages body (previously the raw CC body was written out verbatim).
func TestForwardAsAnthropic_ClinePassNonStreamingAggregatesUpstreamStream(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":false}`)
	c, rec := newClinePassMessagesTestContext(body)
	svc, upstream := newClinePassMessagesTestService("rid_cp_nonstream")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, clinePassAccountTestAccount(503), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "https://api.cline.bot/api/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	require.Contains(t, string(upstream.lastBody), "claude-sonnet-4-5")

	out := rec.Body.String()
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "assistant", gjson.Get(out, "role").String())
	require.Equal(t, "hello", gjson.Get(out, "content.0.text").String())
	require.Equal(t, "end_turn", gjson.Get(out, "stop_reason").String())
	require.Equal(t, "claude-sonnet-4-5", gjson.Get(out, "model").String())
	require.False(t, result.Stream)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
}

// The streaming half: the same upstream SSE must be re-encoded as Anthropic
// Messages SSE events for the client.
func TestForwardAsAnthropic_ClinePassStreamingEmitsAnthropicSSE(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	c, rec := newClinePassMessagesTestContext(body)
	svc, upstream := newClinePassMessagesTestService("rid_cp_stream")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, clinePassAccountTestAccount(504), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())

	out := rec.Body.String()
	require.Contains(t, out, "event: message_start")
	require.Contains(t, out, `"text":"he"`)
	require.Contains(t, out, `"text":"llo"`)
	require.Contains(t, out, `"stop_reason":"end_turn"`)
	require.Contains(t, out, "event: message_stop")
	require.True(t, result.Stream)
}
