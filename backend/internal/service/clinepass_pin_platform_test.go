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

func TestApplyClinePassUpstreamPinAccountOverridesPlatform(t *testing.T) {
	acct := newClinePassPinAccount(pinExtra(t, map[string]any{
		"mode":     "strict",
		"upstream": "morph",
	}))
	body := []byte(`{"model":"cline-pass/deepseek-v4-pro","messages":[]}`)

	out := ApplyClinePassUpstreamPinWithPlatform(acct, deepSeekPlatformPin(), body)
	require.Equal(t, "morph", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
	require.Equal(t, "morph", gjson.GetBytes(out, "provider.only.0").String())
}

func TestApplyClinePassUpstreamPinAccountOffDisablesPlatform(t *testing.T) {
	acct := newClinePassPinAccount(pinExtra(t, map[string]any{"mode": "off"}))
	body := []byte(`{"model":"cline-pass/deepseek-v4-pro","messages":[]}`)
	require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(acct, deepSeekPlatformPin(), body))
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

func TestMergeClinePassUpstreamPinMergesModels(t *testing.T) {
	platform := &ClinePassUpstreamPinConfig{
		Mode:          "strict",
		Upstream:      "deepseek",
		MatchPrefixes: []string{"deepseek"},
		Models:        map[string]ClinePassUpstreamPinConfig{"deepseek-v4-pro": {Upstream: "deepseek"}},
	}
	account := &ClinePassUpstreamPinConfig{
		Models: map[string]ClinePassUpstreamPinConfig{"deepseek-v4.1-flash": {Upstream: "deepseek-flash"}},
	}
	merged := MergeClinePassUpstreamPin(platform, account)
	require.Equal(t, "strict", merged.Mode)
	require.Equal(t, "deepseek", merged.Upstream)
	require.Len(t, merged.Models, 2)
	require.Equal(t, "deepseek-flash", merged.Models["deepseek-v4.1-flash"].Upstream)
	require.Equal(t, "deepseek", merged.Models["deepseek-v4-pro"].Upstream)
}

func TestClinePassUpstreamPinDisabled(t *testing.T) {
	require.True(t, ClinePassUpstreamPinDisabled(pinExtra(t, map[string]any{"mode": "off"})))
	require.False(t, ClinePassUpstreamPinDisabled(pinExtra(t, map[string]any{"mode": "strict"})))
	require.False(t, ClinePassUpstreamPinDisabled(nil))
}

func TestValidateClinePassUpstreamPin(t *testing.T) {
	require.NoError(t, validateClinePassUpstreamPin(nil))
	require.NoError(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Mode: "strict", Upstream: "deepseek"}))
	require.NoError(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Mode: "off"}))
	require.Error(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Mode: "bogus", Upstream: "x"}))
	require.Error(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Pipelines: "bogus", Upstream: "x"}))
	require.Error(t, validateClinePassUpstreamPin(&ClinePassUpstreamPinConfig{Mode: "strict"}))
}
