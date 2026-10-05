//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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
