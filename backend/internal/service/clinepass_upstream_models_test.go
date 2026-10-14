package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClinePassModelSyncRequest(t *testing.T) {
	svc := &AccountTestService{}
	account := &Account{
		Platform: PlatformClinePass,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "  cline-token  ",
		},
	}

	req, err := svc.buildClinePassUpstreamModelsRequest(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, req)
	require.Equal(t, http.MethodGet, req.Method)
	require.Equal(t, "https://api.cline.bot/api/v1/ai/cline/recommended-models", req.URL.String())
	require.Equal(t, "application/json", req.Header.Get("Accept"))
	require.Equal(t, "Bearer workos:cline-token", req.Header.Get("Authorization"))
	require.Equal(t, "https://cline.bot", req.Header.Get("HTTP-Referer"))
	require.Equal(t, "Cline", req.Header.Get("X-Title"))
}

func TestClinePassModelSyncRequestHonorsBaseURLOverride(t *testing.T) {
	svc := &AccountTestService{}
	account := &Account{
		Platform: PlatformClinePass,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "workos:already-prefixed",
			"base_url":     "https://example.test/api/v1/",
		},
	}

	req, err := svc.buildClinePassUpstreamModelsRequest(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "https://example.test/api/v1/ai/cline/recommended-models", req.URL.String())
	require.Equal(t, "Bearer workos:already-prefixed", req.Header.Get("Authorization"))
}

func TestClinePassModelSyncRequestRejectsMissingToken(t *testing.T) {
	svc := &AccountTestService{}
	for _, account := range []*Account{
		nil,
		{Platform: PlatformClinePass, Type: AccountTypeOAuth},
		{Platform: PlatformClinePass, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "   "}},
	} {
		req, err := svc.buildClinePassUpstreamModelsRequest(context.Background(), account)
		require.Error(t, err)
		require.Nil(t, req)
	}
}

// The frontend hides the "sync upstream models" button based on
// supportsUpstreamModelSync (frontend/src/constants/platformCatalog.ts); the
// dispatcher below is the backend contract it mirrors.
func TestUpstreamModelsRequestDispatchCoversClinePass(t *testing.T) {
	svc := &AccountTestService{}
	account := &Account{
		Platform: PlatformClinePass,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "cline-token",
		},
	}

	req, err := svc.buildUpstreamModelsRequest(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "https://api.cline.bot/api/v1/ai/cline/recommended-models", req.URL.String())
}
