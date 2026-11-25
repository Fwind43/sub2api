package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiResponseToChatCompletionsPreservesInlineData(t *testing.T) {
	tests := []struct {
		name  string
		parts []any
		want  string
	}{
		{
			name: "image only",
			parts: []any{
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="}},
			},
			want: "![image](data:image/png;base64,aW1hZ2U=)",
		},
		{
			name: "text and image",
			parts: []any{
				map[string]any{"text": "rendered image:\n"},
				map[string]any{"inlineData": map[string]any{"mimeType": "image/webp", "data": "d2VicA=="}},
			},
			want: "rendered image:\n![image](data:image/webp;base64,d2VicA==)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geminiResp := map[string]any{
				"candidates": []any{map[string]any{
					"content":      map[string]any{"parts": tt.parts},
					"finishReason": "STOP",
				}},
			}
			rawData, err := json.Marshal(geminiResp)
			require.NoError(t, err)

			got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
			require.NoError(t, err)
			require.Len(t, got.Choices, 1)

			var content string
			require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
			require.Equal(t, tt.want, content)
			require.Equal(t, "stop", got.Choices[0].FinishReason)
		})
	}
}

func TestGeminiResponseToChatCompletionsOmitsInvalidInlineData(t *testing.T) {
	tests := []struct {
		name       string
		inlineData map[string]any
	}{
		{
			name:       "unsupported MIME type",
			inlineData: map[string]any{"mimeType": "image/svg+xml", "data": "PHN2Zz48L3N2Zz4="},
		},
		{
			name:       "malformed MIME type",
			inlineData: map[string]any{"mimeType": "image/png; charset=utf-8", "data": "aW1hZ2U="},
		},
		{
			name:       "malformed base64",
			inlineData: map[string]any{"mimeType": "image/png", "data": "not-valid-base64!!!"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geminiResp := map[string]any{
				"candidates": []any{map[string]any{
					"content":      map[string]any{"parts": []any{map[string]any{"text": "before"}, map[string]any{"inlineData": tt.inlineData}, map[string]any{"text": "after"}}},
					"finishReason": "STOP",
				}},
			}
			rawData, err := json.Marshal(geminiResp)
			require.NoError(t, err)

			got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
			require.NoError(t, err)

			var content string
			require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
			require.Equal(t, "beforeafter", content)
		})
	}
}

func TestConvertGeminiToClaudeMessageOmitsInlineDataForAnthropicMessages(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "before"},
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="}},
				map[string]any{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Paris"}}},
				map[string]any{"text": "after"},
			}},
			"finishReason": "STOP",
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	withInlineData, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, true)
	contentWithInlineData, ok := withInlineData["content"].([]any)
	require.True(t, ok)
	require.Len(t, contentWithInlineData, 4)
	require.Equal(t, map[string]any{"type": "text", "text": "before"}, contentWithInlineData[0])
	require.Equal(t, map[string]any{"type": "text", "text": "![image](data:image/png;base64,aW1hZ2U=)"}, contentWithInlineData[1])
	toolUse, ok := contentWithInlineData[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUse["type"])
	require.Equal(t, "get_weather", toolUse["name"])
	require.Equal(t, map[string]any{"type": "text", "text": "after"}, contentWithInlineData[3])

	withoutInlineData, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, false)
	contentWithoutInlineData, ok := withoutInlineData["content"].([]any)
	require.True(t, ok)
	require.Len(t, contentWithoutInlineData, 3)
	require.Equal(t, map[string]any{"type": "text", "text": "before"}, contentWithoutInlineData[0])
	toolUseWithoutInlineData, ok := contentWithoutInlineData[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUseWithoutInlineData["type"])
	require.Equal(t, "get_weather", toolUseWithoutInlineData["name"])
	require.Equal(t, map[string]any{"type": "text", "text": "after"}, contentWithoutInlineData[2])
}

func TestGeminiResponseToChatCompletionsRetainsTextAndToolBehavior(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "checking"},
				map[string]any{"functionCall": map[string]any{
					"name": "get_weather",
					"args": map[string]any{"city": "Paris"},
				}},
			}},
			"finishReason": "STOP",
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
	require.NoError(t, err)
	require.Len(t, got.Choices, 1)

	choice := got.Choices[0]
	var content string
	require.NoError(t, json.Unmarshal(choice.Message.Content, &content))
	require.Equal(t, "checking", content)
	require.Equal(t, "tool_calls", choice.FinishReason)
	require.Len(t, choice.Message.ToolCalls, 1)
	require.Equal(t, "get_weather", choice.Message.ToolCalls[0].Function.Name)
	require.JSONEq(t, `{"city":"Paris"}`, choice.Message.ToolCalls[0].Function.Arguments)
}

func TestGeminiResponseToChatCompletionsOmitsThoughtParts(t *testing.T) {
	rawData := []byte(`{"candidates":[{"content":{"role":"model","parts":[` +
		`{"text":"**Solving the puzzle**\n\nThe ball costs 0.05.\n\n\n","thought":true},` +
		`{"text":"0.05","thoughtSignature":"sig_1"}]},"finishReason":"STOP","index":0}],` +
		`"usageMetadata":{"promptTokenCount":36,"candidatesTokenCount":4,"thoughtsTokenCount":217,"totalTokenCount":257}}`)
	var geminiResp map[string]any
	require.NoError(t, json.Unmarshal(rawData, &geminiResp))

	got, usage, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
	require.NoError(t, err)
	require.Len(t, got.Choices, 1)

	var content string
	require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
	require.Equal(t, "0.05", content)
	require.Empty(t, got.Choices[0].Message.ReasoningContent)
	require.Equal(t, "stop", got.Choices[0].FinishReason)
	require.Equal(t, 221, usage.OutputTokens)
}

func TestGeminiResponseToChatCompletionsThoughtOnlyResponse(t *testing.T) {
	rawData := []byte(`{"candidates":[{"content":{"role":"model","parts":[` +
		`{"text":"Still reasoning about","thought":true}]},"finishReason":"MAX_TOKENS","index":0}],` +
		`"usageMetadata":{"promptTokenCount":36,"candidatesTokenCount":0,"thoughtsTokenCount":64,"totalTokenCount":100}}`)
	var geminiResp map[string]any
	require.NoError(t, json.Unmarshal(rawData, &geminiResp))

	got, usage, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
	require.NoError(t, err)
	require.Len(t, got.Choices, 1)
	require.Empty(t, got.Choices[0].Message.Content)
	require.Empty(t, got.Choices[0].Message.ReasoningContent)
	require.Equal(t, "length", got.Choices[0].FinishReason)
	require.Equal(t, 64, usage.OutputTokens)
}

func TestConvertGeminiToClaudeMessageOmitsThoughtParts(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "planning the call", "thought": true},
				map[string]any{"text": "checking", "thought": false},
				map[string]any{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Paris"}}, "thoughtSignature": "sig_1"},
			}},
			"finishReason": "STOP",
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	got, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, false)
	content, ok := got["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	require.Equal(t, map[string]any{"type": "text", "text": "checking"}, content[0])
	toolUse, ok := content[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUse["type"])
	require.Equal(t, "get_weather", toolUse["name"])
	require.Equal(t, "tool_use", got["stop_reason"])
}

type failingChatStreamWriter struct {
	header      http.Header
	statusCode  int
	failOnWrite bool
	mu          sync.Mutex
	buf         bytes.Buffer
	flushCount  int
}

func (w *failingChatStreamWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingChatStreamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failOnWrite {
		return 0, errors.New("client write failed")
	}
	return w.buf.Write(p)
}

func (w *failingChatStreamWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func (w *failingChatStreamWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushCount++
}

func (w *failingChatStreamWriter) setFailOnWrite(v bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failOnWrite = v
}

func (w *failingChatStreamWriter) bodyString() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func newTestGinContext(w http.ResponseWriter) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func TestHandleChatCompletionsStreamingResponseFromGemini_EmitsKeepaliveDuringSilence(t *testing.T) {
	prev := geminiChatCompletionsKeepaliveIntervalOverride
	geminiChatCompletionsKeepaliveIntervalOverride = 25 * time.Millisecond
	t.Cleanup(func() { geminiChatCompletionsKeepaliveIntervalOverride = prev })

	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = pw.Close() }()
		time.Sleep(80 * time.Millisecond)
		_, _ = io.WriteString(pw, `data: {"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3}}`+"\n\n")
		time.Sleep(80 * time.Millisecond)
		_, _ = io.WriteString(pw, `data: {"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`+"\n\n")
	}()

	rec := httptest.NewRecorder()
	c := newTestGinContext(rec)
	svc := &GeminiMessagesCompatService{
		cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 10}},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	}

	res, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-4-neon-xhigh", false, true)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.clientDisconnect)
	require.True(t, res.finished)
	require.Equal(t, 10, res.usage.InputTokens)
	require.Equal(t, 5, res.usage.OutputTokens)

	body := rec.Body.String()
	require.GreaterOrEqual(t, strings.Count(body, ":\n\n"), 2, "expected keepalive comments during silent intervals, got:\n%s", body)
	require.Contains(t, body, `"content":"hello"`)
	require.Contains(t, body, `"content":" world"`)
	require.Contains(t, body, "data: [DONE]\n\n")
}

func TestHandleChatCompletionsStreamingResponseFromGemini_NoKeepaliveWhenDisabled(t *testing.T) {
	prev := geminiChatCompletionsKeepaliveIntervalOverride
	geminiChatCompletionsKeepaliveIntervalOverride = 0
	t.Cleanup(func() { geminiChatCompletionsKeepaliveIntervalOverride = prev })

	pr, pw := io.Pipe()
	go func() {
		defer func() { _ = pw.Close() }()
		time.Sleep(40 * time.Millisecond)
		_, _ = io.WriteString(pw, `data: {"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1}}`+"\n\n")
	}()

	rec := httptest.NewRecorder()
	c := newTestGinContext(rec)
	svc := &GeminiMessagesCompatService{
		cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 0}},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	}

	res, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-4-neon-xhigh", false, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.finished)
	require.NotContains(t, rec.Body.String(), ":\n\n")
	require.Contains(t, rec.Body.String(), `"content":"ok"`)
}

func TestHandleChatCompletionsStreamingResponseFromGemini_KeepaliveDisconnectStopsCleanly(t *testing.T) {
	prev := geminiChatCompletionsKeepaliveIntervalOverride
	geminiChatCompletionsKeepaliveIntervalOverride = 20 * time.Millisecond
	t.Cleanup(func() { geminiChatCompletionsKeepaliveIntervalOverride = prev })

	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()

	writer := &failingChatStreamWriter{}
	writer.setFailOnWrite(true)
	c := newTestGinContext(writer)
	svc := &GeminiMessagesCompatService{
		cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 10}},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	}

	done := make(chan struct{})
	var (
		res *geminiStreamResult
		err error
	)
	go func() {
		defer close(done)
		res, err = svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-4-neon-xhigh", false, false)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("streaming handler did not exit promptly after keepalive write error")
	}

	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.clientDisconnect)
}

func TestHandleChatCompletionsStreamingResponseFromGemini_ThinkingDeltaPassthrough(t *testing.T) {
	prev := geminiChatCompletionsKeepaliveIntervalOverride
	geminiChatCompletionsKeepaliveIntervalOverride = 0
	t.Cleanup(func() { geminiChatCompletionsKeepaliveIntervalOverride = prev })

	// Verify request-side thinkingConfig enables includeThoughts without overriding -xhigh model tier.
	baseReq := `{"contents":[{"role":"user","parts":[{"text":"solve"}]}],"generationConfig":{"maxOutputTokens":4096}}`
	outReq := applyGeminiCompatThinkingConfig([]byte(baseReq), []byte(`{"model":"gemini-4-neon-xhigh"}`), "gemini-4-neon-xhigh", true)
	require.True(t, gjson.GetBytes(outReq, "generationConfig.thinkingConfig.includeThoughts").Bool())
	require.False(t, gjson.GetBytes(outReq, "generationConfig.thinkingConfig.thinkingLevel").Exists())

	sse := strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"step 1 ","thought":true}]}}]}`,
		``,
		`data: {"candidates":[{"content":{"parts":[{"text":"step 2","thought":true}]}}]}`,
		``,
		`data: {"candidates":[{"content":{"parts":[{"text":"final answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":20,"thoughtsTokenCount":15}}`,
		``,
	}, "\n") + "\n"

	rec := httptest.NewRecorder()
	c := newTestGinContext(rec)
	svc := &GeminiMessagesCompatService{
		cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 0}},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}

	res, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-4-neon-xhigh", false, true)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.finished)
	require.Equal(t, 12, res.usage.InputTokens)
	require.Equal(t, 35, res.usage.OutputTokens)

	var reasoningBuf strings.Builder
	var contentBuf strings.Builder
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		if r := gjson.Get(payload, "choices.0.delta.reasoning_content"); r.Exists() {
			reasoningBuf.WriteString(r.String())
		}
		if txt := gjson.Get(payload, "choices.0.delta.content"); txt.Exists() {
			contentBuf.WriteString(txt.String())
		}
	}
	require.Equal(t, "step 1 step 2", reasoningBuf.String())
	require.Equal(t, "final answer", contentBuf.String())
	require.Contains(t, rec.Body.String(), "data: [DONE]\n\n")
}
