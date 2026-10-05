//go:build unit

package service

import (
	"encoding/json"
	"strings"
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

func TestParseClinePassAvailableProvidersProvidersServingWording(t *testing.T) {
	raw := `{"error":"inference request failed: failed to invoke model 'z-ai/glm-5.3-flash' from Openrouter: request failed with status 404: {\"error\":{\"message\":\"No allowed providers are available for the selected model. Providers serving z-ai/glm-5.3-flash-20260826: relace, inference-net, sail-research, gmicloud, deepinfra, novita, but your request's provider.only preference permits only: __probe__.\"}}"}`
	got := ParseClinePassAvailableProviders(raw)
	require.Equal(t, []string{"relace", "inference-net", "sail-research", "gmicloud", "deepinfra", "novita"}, got)
}

func TestParseClinePassAvailableProvidersProvidersServingTruncated(t *testing.T) {
	// Real relay responses are truncated at the raw limit; JSON is invalid but
	// the wording match still yields the provider list (live glm-5.3-flash case).
	raw := `{"error":"inference request failed: failed to invoke model 'z-ai/glm-5.3-flash' from Openrouter: request failed with status 404: {\"error\":{\"message\":\"No allowed providers are available for the selected model. Providers serving z-ai/glm-5.3-flash-20260826: relace, inference-net, sail-research, open-inference, wafer, deepinfra, novita, streamlake, decart, gmicloud, dekallm, near-ai, phala, morph, modal, baseten, crusoe, coreweave, atlas-cloud, fireworks, friendli, siliconflow, digitalocean, together, reka, parasail, venice, z-ai, nextbit, inceptron, cloudflare, but your request's provider.o`
	got := ParseClinePassAvailableProviders(raw)
	require.Contains(t, got, "gmicloud")
	require.Contains(t, got, "cloudflare")
	require.Len(t, got, 31)
}

func TestParseClinePassAvailableProvidersNestedStringObject(t *testing.T) {
	// Newer relays nest the OpenRouter object JSON inside a string field.
	inner := `{"error":{"message":"No endpoints found","metadata":{"available_providers":["anthropic","deepseek","x-ai"]}}}`
	outer := `{"error":"request failed with status 404: ` + strings.ReplaceAll(inner, `"`, `\"`) + `"}`
	got := ParseClinePassAvailableProviders(outer)
	require.Equal(t, []string{"anthropic", "deepseek", "x-ai"}, got)
}

func TestParseClinePassAvailableProvidersSelfReferentialSafe(t *testing.T) {
	// A string carrier that embeds itself must terminate (depth limit).
	raw := `{"error":"{\"error\":\"{\\\"error\\\":\\\"{\\\\\\\"error\\\\\\\":\\\\\\\"{\\\\\\\\\\\"error\\\\\\\\\\\":\\\\\\\\\\\"x\\\\\\\\\\\"}\\\\\\\"}\\\"}\"}"}`
	require.Nil(t, ParseClinePassAvailableProviders(raw))
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
