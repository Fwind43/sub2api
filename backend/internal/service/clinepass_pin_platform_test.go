//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func deepSeekPlatformPin() *ClinePassUpstreamPinConfig {
	return &ClinePassUpstreamPinConfig{
		Mode:          "strict",
		Upstream:      "deepseek",
		MatchPrefixes: []string{"deepseek"},
	}
}

func TestApplyClinePassUpstreamPinWithPlatformPinsDeepSeekOnly(t *testing.T) {
	acct := newClinePassPinAccount(nil)
	platform := deepSeekPlatformPin()

	// deepseek family -> pinned to the relay "deepseek" provider on both pipelines
	ds := []byte(`{"model":"cline-pass/deepseek-v4-pro","messages":[]}`)
	out := ApplyClinePassUpstreamPinWithPlatform(acct, platform, ds)
	require.Equal(t, "deepseek", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
	require.Equal(t, "deepseek", gjson.GetBytes(out, "provider.only.0").String())

	// non deepseek model -> untouched
	other := []byte(`{"model":"cline-pass/claude-sonnet-4-5","messages":[]}`)
	require.Equal(t, other, ApplyClinePassUpstreamPinWithPlatform(acct, platform, other))

	// free family -> never pinned, even when the basename starts with deepseek
	free := []byte(`{"model":"cline-free/deepseek-v4-pro","messages":[]}`)
	require.Equal(t, free, ApplyClinePassUpstreamPinWithPlatform(acct, platform, free))
}

func TestApplyClinePassUpstreamPinNoPlatformNoChange(t *testing.T) {
	acct := newClinePassPinAccount(nil)
	body := []byte(`{"model":"cline-pass/deepseek-v4-pro","messages":[]}`)
	require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(acct, nil, body))
}

func TestApplyClinePassUpstreamPinLegacyAccountCannotOverridePlatform(t *testing.T) {
	body := []byte(`{"model":"cline-pass/deepseek-v4-pro","messages":[]}`)
	for name, legacy := range map[string]any{
		"strict":         map[string]any{"mode": "strict", "upstream": "morph"},
		"preferred":      map[string]any{"mode": "preferred", "upstream": "morph", "only": []string{"morph"}, "pipelines": "vercel"},
		"off":            map[string]any{"mode": "off"},
		"model override": map[string]any{"models": map[string]any{"deepseek-v4-pro": map[string]any{"upstream": "morph"}}},
		"json string":    `{"mode":"off"}`,
		"invalid":        `invalid-json`,
	} {
		t.Run(name, func(t *testing.T) {
			acct := newClinePassPinAccount(map[string]any{ClinePassUpstreamPinExtraKey: legacy})
			out := ApplyClinePassUpstreamPinWithPlatform(acct, deepSeekPlatformPin(), body)
			require.Equal(t, "deepseek", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
			require.Equal(t, "deepseek", gjson.GetBytes(out, "provider.only.0").String())
			require.False(t, gjson.GetBytes(out, "provider.order").Exists())
			// With no platform pin, old account settings must not revive pinning.
			require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(acct, nil, body))
			require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(acct, &ClinePassUpstreamPinConfig{Mode: "off"}, body))
			require.Equal(t, []byte(`{"model":"glm-5.3"}`), ApplyClinePassUpstreamPinWithPlatform(acct, deepSeekPlatformPin(), []byte(`{"model":"glm-5.3"}`)))
		})
	}
}

func TestApplyClinePassUpstreamPinPlatformPerModelOverride(t *testing.T) {
	platform := deepSeekPlatformPin()
	platform.Models = map[string]ClinePassUpstreamPinConfig{
		"deepseek-v4.1-flash": {Mode: "strict", Upstream: "deepseek-flash"},
	}
	acct := newClinePassPinAccount(nil)

	out := ApplyClinePassUpstreamPinWithPlatform(acct, platform, []byte(`{"model":"cline-pass/deepseek-v4.1-flash"}`))
	require.Equal(t, "deepseek-flash", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())

	out = ApplyClinePassUpstreamPinWithPlatform(acct, platform, []byte(`{"model":"cline-pass/deepseek-v4-pro"}`))
	require.Equal(t, "deepseek", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
}

func TestApplyClinePassUpstreamPinGuardsAndSharedConfig(t *testing.T) {
	body := []byte(`{"model":"cline-pass/deepseek-v4-pro"}`)
	platform := &ClinePassUpstreamPinConfig{
		Mode: " STRICT ", Upstream: " deepseek ", Pipelines: " BOTH ",
		MatchPrefixes: []string{" DeepSeek ", "deepseek", ""},
	}
	before := *platform
	before.MatchPrefixes = append([]string(nil), platform.MatchPrefixes...)
	require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(nil, platform, body))
	require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(&Account{Platform: "openai"}, platform, body))
	require.Nil(t, ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), platform, nil))
	for i := 0; i < 2; i++ {
		out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), platform, body)
		require.Equal(t, "deepseek", gjson.GetBytes(out, "provider.only.0").String())
		require.Equal(t, "deepseek", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
		require.Equal(t, before, *platform, "a shared platform config must not be mutated")
		require.Equal(t, out, ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), platform, out))
	}
}

func TestParseClinePassUpstreamPinJSONNormalizesPrefixes(t *testing.T) {
	cfg, err := ParseClinePassUpstreamPinJSON(`{"mode":"STRICT","upstream":" deepseek ","match_prefixes":["DeepSeek","deepseek",""]}`)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, "strict", cfg.Mode)
	require.Equal(t, "deepseek", cfg.Upstream)
	require.Equal(t, []string{"deepseek"}, cfg.MatchPrefixes)

	empty, err := ParseClinePassUpstreamPinJSON("  ")
	require.NoError(t, err)
	require.Nil(t, empty)

	off, err := ParseClinePassUpstreamPinJSON(`{"mode":"off"}`)
	require.NoError(t, err)
	require.Nil(t, off)
}

func TestValidateClinePassUpstreamPin(t *testing.T) {
	require.NoError(t, validateClinePassUpstreamPin(nil))
	require.NoError(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Mode: "strict", Upstream: "deepseek"}))
	require.NoError(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Mode: "off"}))
	require.Error(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Mode: "bogus", Upstream: "x"}))
	require.Error(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Pipelines: "bogus", Upstream: "x"}))
	require.Error(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Mode: "strict"}))
}
