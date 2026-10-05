//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// clinePassAccountTestAccount builds an OAuth ClinePass account whose stored
// credential is a raw Cline JWT (no workos: prefix), matching what the device
// flow persists.
func clinePassAccountTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "clinepass-oauth",
		Platform:    PlatformClinePass,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "eyJhbGciOiJUTEST",
			"base_url":     "https://api.cline.bot/api/v1",
		},
	}
}

func clinePassAccountTestService(account *Account, responses ...*http.Response) (*AccountTestService, *httpUpstreamRecorder) {
	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{account.ID: account},
		},
	}
	upstream := &httpUpstreamRecorder{responses: responses}
	return &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}, upstream
}

func clinePassChatSSE() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")),
	}
}

// The regression this guards: ClinePass accounts used to fall through to the
// Anthropic tester, which posted the Cline token to api.anthropic.com and got
// `401 Invalid bearer token`.
func TestAccountTestService_ClinePassUsesChatCompletionsWithWorkOSCredential(t *testing.T) {
	account := clinePassAccountTestAccount(401)
	svc, upstream := clinePassAccountTestService(account, clinePassChatSSE())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "openai/gpt-6.1-sol", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://api.cline.bot/api/v1/chat/completions", req.URL.String())
	require.Equal(t, "Bearer workos:eyJhbGciOiJUTEST", req.Header.Get("Authorization"))
	require.Equal(t, "https://cline.bot", req.Header.Get("HTTP-Referer"))
	require.Equal(t, "Cline", req.Header.Get("X-Title"))
	require.Equal(t, "XMLHttpRequest", req.Header.Get("X-Requested-With"))
	require.Contains(t, string(upstream.bodies[0]), `"model":"openai/gpt-6.1-sol"`)
	require.Contains(t, string(upstream.bodies[0]), `"stream":true`)
	require.NotContains(t, recorder.Body.String(), "test_error")
}

func TestAccountTestService_ClinePassDefaultsToFreeModel(t *testing.T) {
	account := clinePassAccountTestAccount(402)
	svc, upstream := clinePassAccountTestService(account, clinePassChatSSE())
	c, _ := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Contains(t, string(upstream.bodies[0]), `"model":"`+ClinePassFallbackTestModel+`"`)
}

// Models are exposed to clients without the subscription vendor prefix, so the
// tester must restore `cline-pass/` before calling the relay (which rejects ids
// that are not vendor-qualified).
func TestAccountTestService_ClinePassRestoresVendorPrefixForBareModel(t *testing.T) {
	account := clinePassAccountTestAccount(403)
	svc, upstream := clinePassAccountTestService(account, clinePassChatSSE())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "deepseek-v4-pro", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Contains(t, string(upstream.bodies[0]), `"model":"cline-pass/deepseek-v4-pro"`)
	require.Contains(t, recorder.Body.String(), `"model":"cline-pass/deepseek-v4-pro"`)
}

// Ids from other families already carry a vendor prefix and must pass through
// untouched.
func TestAccountTestService_ClinePassKeepsForeignVendorPrefix(t *testing.T) {
	account := clinePassAccountTestAccount(406)
	svc, upstream := clinePassAccountTestService(account, clinePassChatSSE())
	c, _ := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "anthropic/claude-sonnet-5.5", "hi", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Contains(t, string(upstream.bodies[0]), `"model":"anthropic/claude-sonnet-5.5"`)
}

func TestClinePassModelPrefixRoundTrip(t *testing.T) {
	require.Equal(t, "deepseek-v4-pro", clinePassStripModelPrefix("cline-pass/deepseek-v4-pro"))
	require.Equal(t, "anthropic/claude-sonnet-5.5", clinePassStripModelPrefix("anthropic/claude-sonnet-5.5"))
	require.Equal(t, "deepseek-v4-pro", clinePassStripModelPrefix("  deepseek-v4-pro  "))
	require.Equal(t, "cline-pass/deepseek-v4-pro", clinePassRestoreModelPrefix("deepseek-v4-pro"))
	require.Equal(t, "cline-pass/deepseek-v4-pro", clinePassRestoreModelPrefix("cline-pass/deepseek-v4-pro"))
	require.Equal(t, "anthropic/claude-sonnet-5.5", clinePassRestoreModelPrefix("anthropic/claude-sonnet-5.5"))
	require.Equal(t, "", clinePassRestoreModelPrefix("   "))
	require.Equal(t, []string{"deepseek-v4-pro", "glm-5.3", "cline-free/x"},
		stripClinePassModelPrefixes([]string{"cline-pass/deepseek-v4-pro", "cline-pass/glm-5.3", "cline-free/x", "cline-pass/deepseek-v4-pro"}))
}

func TestAccountTestService_ClinePassSurfacesUpstream401(t *testing.T) {
	account := clinePassAccountTestAccount(404)
	svc, upstream := clinePassAccountTestService(account, newJSONResponse(http.StatusUnauthorized, `{"error":{"message":"Invalid bearer token"}}`))
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "openai/gpt-6.1-sol", "hi", AccountTestModeDefault)

	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
	require.Contains(t, err.Error(), "401")
	require.Contains(t, err.Error(), "Invalid bearer token")
	require.Contains(t, recorder.Body.String(), "Invalid bearer token")
}

func TestAccountTestService_ClinePassExplains402Credits(t *testing.T) {
	account := clinePassAccountTestAccount(405)
	svc, _ := clinePassAccountTestService(account, newJSONResponse(http.StatusPaymentRequired, `{"error":{"code":"insufficient_credits"}}`))
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "openai/gpt-6.1-sol", "hi", AccountTestModeDefault)

	require.Error(t, err)
	require.Contains(t, err.Error(), "402")
	require.Contains(t, err.Error(), "Cline Credits")
	require.Contains(t, recorder.Body.String(), "Cline Credits")
}
