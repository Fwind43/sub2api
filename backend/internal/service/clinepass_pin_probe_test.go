//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseClinePassAvailableProvidersVercelPlainText(t *testing.T) {
	raw := `{"error":"Bad Request","message":"No provider available. Available providers are: anthropic, google-vertex, openai. Choose one."}`
	got := ParseClinePassAvailableProviders(raw)
	require.Equal(t, []string{"anthropic", "google-vertex", "openai"}, got)
}

func TestParseClinePassAvailableProvidersOpenRouterJSON(t *testing.T) {
	raw := `{"error":{"message":"No endpoints found","metadata":{"available_providers":["anthropic","deepseek","x-ai"]}}}`
	got := ParseClinePassAvailableProviders(raw)
	require.Equal(t, []string{"anthropic", "deepseek", "x-ai"}, got)
}

func TestParseClinePassAvailableProvidersJSONObjects(t *testing.T) {
	raw := `{"error":{"metadata":{"available_providers":[{"slug":"anthropic"},{"slug":"openai"}]}}}`
	got := ParseClinePassAvailableProviders(raw)
	require.Equal(t, []string{"anthropic", "openai"}, got)
}

func TestParseClinePassAvailableProvidersEmptyAndGarbage(t *testing.T) {
	require.Nil(t, ParseClinePassAvailableProviders(""))
	require.Nil(t, ParseClinePassAvailableProviders("plain text with no keywords"))
	// slug filter rejects tokens with spaces or uppercase
	require.Nil(t, ParseClinePassAvailableProviders("Available providers are: Not A Slug."))
}

func TestParseClinePassAvailableProvidersDedupesTokens(t *testing.T) {
	raw := `Available providers are: anthropic, anthropic, openai.`
	got := ParseClinePassAvailableProviders(raw)
	require.Equal(t, []string{"anthropic", "openai"}, got)
}

func TestBuildClinePassUpstreamProbeBodyVercelPath(t *testing.T) {
	body, err := buildClinePassUpstreamProbeBody("cline-pass/deepseek-v4-pro", "providerOptions.gateway")
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	require.Equal(t, "cline-pass/deepseek-v4-pro", m["model"])
	only := m["providerOptions"].(map[string]any)["gateway"].(map[string]any)["only"].([]any)
	require.Equal(t, []any{"__probe__"}, only)
}

func TestBuildClinePassUpstreamProbeBodyOpenRouterPath(t *testing.T) {
	body, err := buildClinePassUpstreamProbeBody("cline-pass/x", "provider")
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	only := m["provider"].(map[string]any)["only"].([]any)
	require.Equal(t, []any{"__probe__"}, only)
}
