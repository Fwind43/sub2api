package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandCodeModelSyncRequest(t *testing.T) {
	svc := &AccountTestService{}
	account := &Account{Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": " test-key ", "base_url": "https://untrusted.invalid"}}
	req, err := svc.buildUpstreamModelsRequest(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, http.MethodGet, req.Method)
	require.Equal(t, "https://api.commandcode.ai/provider/v1/models", req.URL.String())
	require.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, "production", req.Header.Get("x-cli-environment"))
	require.Equal(t, "1.54.1", req.Header.Get("x-command-code-version"))
	require.Equal(t, "command-code-cli/1.54.1", req.Header.Get("User-Agent"))
	require.Nil(t, req.Body)
}

func TestCommandCodeModelSyncRejectsInvalidCredentials(t *testing.T) {
	svc := &AccountTestService{}
	for _, account := range []*Account{
		nil,
		{Platform: PlatformCommandCode, Type: AccountTypeOAuth},
		{Platform: PlatformCommandCode, Type: AccountTypeAPIKey},
		{Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "  "}},
	} {
		req, err := svc.buildCommandCodeUpstreamModelsRequest(context.Background(), account)
		require.Error(t, err)
		require.Nil(t, req)
	}
}
