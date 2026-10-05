//go:build unit

package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestGetRequestCredential_ClinePassOAuthResolvesAccountCredential guards the
// /v1/chat/completions regression where the shared credential prefetch fell
// through to OpenAITokenProvider for ClinePass OAuth accounts and failed with
// "not an openai oauth account" (502) before the request ever reached
// sendClinePassRequest, which resolves its own credential.
func TestGetRequestCredential_ClinePassOAuthResolvesAccountCredential(t *testing.T) {
	account := clinePassAccountTestAccount(406)
	// A non-nil OpenAI provider reproduces the production failure mode: without
	// an explicit ClinePass branch GetAccessToken delegates to it and errors.
	svc := &OpenAIGatewayService{openAITokenProvider: &OpenAITokenProvider{}}

	token, kind, err := svc.GetRequestCredential(context.Background(), nil, account)

	require.NoError(t, err)
	require.Equal(t, "workos:eyJhbGciOiJUTEST", token)
	require.Equal(t, "oauth", kind)
}

// TestGetAccessToken_ClinePassOAuthResolvesAccountCredential pins the direct
// GetAccessToken dispatch (used by alpha search / count_tokens callers).
func TestGetAccessToken_ClinePassOAuthResolvesAccountCredential(t *testing.T) {
	account := clinePassAccountTestAccount(407)
	svc := &OpenAIGatewayService{openAITokenProvider: &OpenAITokenProvider{}}

	token, kind, err := svc.GetAccessToken(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "workos:eyJhbGciOiJUTEST", token)
	require.Equal(t, "oauth", kind)
}

// TestForwardAsRawChatCompletions_ClinePassOAuthSendsUpstream pins the full
// /v1/chat/completions path for a ClinePass OAuth account: the shared
// credential prefetch must resolve the WorkOS credential (not fail with
// "not an openai oauth account") and the request must reach the ClinePass
// chat completions endpoint with the prefixed bearer token.
func TestForwardAsRawChatCompletions_ClinePassOAuthSendsUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: clinePassChatSSE()}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
		// Production shape: the OpenAI token provider exists and used to reject
		// non-OpenAI accounts during the prefetch.
		openAITokenProvider: &OpenAITokenProvider{},
	}
	account := clinePassAccountTestAccount(408)

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.cline.bot/api/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer workos:eyJhbGciOiJUTEST", upstream.lastReq.Header.Get("Authorization"))
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Contains(t, string(upstream.lastBody), "deepseek-v4.1-flash")
	require.Contains(t, rec.Body.String(), `"object":"chat.completion"`)
}
