//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClinePassPlatformMultiModelPins(t *testing.T) {
	cfg := &ClinePassUpstreamPinConfig{
		Mode: "strict", MatchPrefixes: []string{"deepseek"},
		Models: map[string]ClinePassUpstreamPinConfig{
			"deepseek-v4-pro":    {Upstream: "deepseek", Pipelines: "both"},
			"cline-pass/glm-5.3": {Mode: " STRICT ", Upstream: " z-ai ", Pipelines: " BOTH "},
			"disabled":           {Mode: "off"},
		},
	}
	require.NoError(t, validateClinePassUpstreamPin(cfg))
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	loaded, err := ParseClinePassUpstreamPinJSON(string(raw))
	require.NoError(t, err)
	for model, provider := range map[string]string{
		"cline-pass/deepseek-v4-pro": "deepseek", "glm-5.3": "z-ai",
		"cline-pass/glm-5.3": "z-ai", "other": "", "disabled": "",
		"cline-free/deepseek-v4-pro": "",
	} {
		t.Run(model, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"model": model, "messages": []any{}})
			require.NoError(t, err)
			out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), loaded, body)
			if provider == "" {
				require.Equal(t, body, out)
				return
			}
			for _, path := range []string{"providerOptions.gateway.only", "provider.only"} {
				require.Equal(t, `["`+provider+`"]`, gjson.GetBytes(out, path).Raw)
			}
			require.Equal(t, out, ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), loaded, out))
		})
	}
	after, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.Equal(t, raw, after, "validation and injection must not mutate shared model rules")
}

func TestClinePassModelRuleBypassesOnlyDefaultPrefix(t *testing.T) {
	cfg := deepSeekPlatformPin()
	cfg.Models = map[string]ClinePassUpstreamPinConfig{"glm-5.3": {Upstream: "z-ai"}, "deepseek-disabled": {Mode: "off"}}
	body := []byte(`{"model":"cline-pass/glm-5.3"}`)
	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), cfg, body)
	require.Equal(t, "z-ai", gjson.GetBytes(out, "provider.only.0").String())
	for _, model := range []string{"claude", "deepseek-disabled"} {
		body, _ := json.Marshal(map[string]string{"model": model})
		require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), cfg, body))
	}
}

func TestValidateClinePassMultiModelRules(t *testing.T) {
	for name, models := range map[string]map[string]ClinePassUpstreamPinConfig{
		"empty-id":         {" ": {Upstream: "a"}},
		"prefix-only":      {"cline-pass/": {Upstream: "a"}},
		"duplicate-alias":  {"glm": {Upstream: "a"}, "cline-pass/glm": {Upstream: "b"}},
		"invalid-mode":     {"glm": {Mode: "typo", Upstream: "a"}},
		"invalid-pipeline": {"glm": {Pipelines: "typo", Upstream: "a"}},
		"missing-route":    {"glm": {}},
		"blank-route":      {"glm": {Upstream: "  ", Only: []string{" "}}},
		"nested-rules":     {"glm": {Models: map[string]ClinePassUpstreamPinConfig{"x": {Upstream: "a"}}}},
		"free-model":       {"cline-free/glm": {Upstream: "a"}},
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Models: models}))
		})
	}
	require.NoError(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{
		Upstream: "default", Models: map[string]ClinePassUpstreamPinConfig{"glm": {}, "cline-free/glm": {Mode: "off"}},
	}))
	require.NoError(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{
		Models: map[string]ClinePassUpstreamPinConfig{"glm": {Only: []string{"a", "b"}}},
	}))
}
