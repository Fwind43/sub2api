//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newClinePassPinAccount(extra map[string]any) *Account {
	return &Account{
		ID:       1,
		Platform: PlatformClinePass,
		Extra:    extra,
	}
}

func pinConfig(t *testing.T, cfg map[string]any) *ClinePassUpstreamPinConfig {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	parsed, err := ParseClinePassUpstreamPinJSON(string(raw))
	require.NoError(t, err)
	return parsed
}

func TestApplyClinePassUpstreamPinNoop(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-pro"}`)
	cfg := &ClinePassUpstreamPinConfig{Mode: "strict", Upstream: "deepseek"}
	require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(nil, cfg, body))
	acct := newClinePassPinAccount(nil)
	acct.Platform = PlatformOpenAI
	require.Equal(t, body, ApplyClinePassUpstreamPinWithPlatform(acct, cfg, body))
	require.Empty(t, ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), cfg, nil))
}

func TestApplyClinePassUpstreamPinStrictBothPipelines(t *testing.T) {
	acct := pinConfig(t, map[string]any{
		"mode":     "strict",
		"upstream": "morph",
	})
	body := []byte(`{"model":"deepseek-v4-pro","messages":[]}`)

	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, body)

	require.Equal(t, "morph", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
	require.Equal(t, "morph", gjson.GetBytes(out, "provider.only.0").String())
	// strict mode must not set order
	require.False(t, gjson.GetBytes(out, "providerOptions.gateway.order").Exists())
	require.False(t, gjson.GetBytes(out, "provider.order").Exists())
}

func TestApplyClinePassUpstreamPinPreferredOrder(t *testing.T) {
	acct := pinConfig(t, map[string]any{
		"mode":     "preferred",
		"upstream": "morph",
		"order":    []string{"anthropic", "openai"},
		"only":     []string{"morph", "anthropic", "openai", "google"},
	})
	body := []byte(`{"model":"deepseek-v4-pro"}`)

	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, body)

	require.Equal(t, "morph", gjson.GetBytes(out, "providerOptions.gateway.order.0").String())
	require.Equal(t, "anthropic", gjson.GetBytes(out, "providerOptions.gateway.order.1").String())
	require.Equal(t, "openai", gjson.GetBytes(out, "providerOptions.gateway.order.2").String())
	// allow-list mirrored into only
	require.Equal(t, "morph", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
	require.Equal(t, "google", gjson.GetBytes(out, "providerOptions.gateway.only.3").String())
	require.Equal(t, "morph", gjson.GetBytes(out, "provider.order.0").String())
	require.Equal(t, "morph", gjson.GetBytes(out, "provider.only.0").String())
}

func TestApplyClinePassUpstreamPinExcludeExpandedToOnly(t *testing.T) {
	acct := pinConfig(t, map[string]any{
		"exclude":   []string{"openai"},
		"upstreams": []string{"morph", "openai", "anthropic"},
	})
	body := []byte(`{"model":"deepseek-v4-pro"}`)

	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, body)

	only := gjson.GetBytes(out, "providerOptions.gateway.only").Array()
	require.Len(t, only, 2)
	require.Equal(t, "morph", only[0].String())
	require.Equal(t, "anthropic", only[1].String())
	onlyOR := gjson.GetBytes(out, "provider.only").Array()
	require.Len(t, onlyOR, 2)
	require.Equal(t, "morph", onlyOR[0].String())
}

func TestApplyClinePassUpstreamPinSortBothPipelines(t *testing.T) {
	acct := pinConfig(t, map[string]any{
		"sort": "cost",
	})
	body := []byte(`{"model":"deepseek-v4-pro"}`)

	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, body)

	require.Equal(t, "cost", gjson.GetBytes(out, "providerOptions.gateway.sort").String())
	require.Equal(t, "price", gjson.GetBytes(out, "provider.sort").String())

	// ttft and tps aliases
	for raw, alias := range map[string]string{"ttft": "latency", "tps": "throughput"} {
		out2 := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), pinConfig(t, map[string]any{"sort": raw}), body)
		require.Equal(t, raw, gjson.GetBytes(out2, "providerOptions.gateway.sort").String())
		require.Equal(t, alias, gjson.GetBytes(out2, "provider.sort").String())
	}
}

func TestApplyClinePassUpstreamPinPipelineSelection(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-pro"}`)

	vercel := pinConfig(t, map[string]any{
		"mode": "strict", "upstream": "morph", "pipelines": "vercel",
	})
	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), vercel, body)
	require.True(t, gjson.GetBytes(out, "providerOptions.gateway.only").Exists())
	require.False(t, gjson.GetBytes(out, "provider").Exists())

	openrouter := pinConfig(t, map[string]any{
		"mode": "strict", "upstream": "morph", "pipelines": "openrouter",
	})
	out = ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), openrouter, body)
	require.False(t, gjson.GetBytes(out, "providerOptions.gateway").Exists())
	require.Equal(t, "morph", gjson.GetBytes(out, "provider.only.0").String())
}

func TestApplyClinePassUpstreamPinPerModelOverride(t *testing.T) {
	acct := pinConfig(t, map[string]any{
		"mode":     "strict",
		"upstream": "morph",
		"models": map[string]any{
			"deepseek-v4-pro": map[string]any{"upstream": "anthropic"},
			"cline-pass/other-model": map[string]any{
				"mode": "off",
			},
		},
	})

	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, []byte(`{"model":"deepseek-v4-pro"}`))
	require.Equal(t, "anthropic", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())

	// vendor-prefixed key variant
	out = ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, []byte(`{"model":"other-model"}`))
	require.Empty(t, gjson.GetBytes(out, "providerOptions.gateway.only").Array())

	// model without override falls back to top-level
	out = ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, []byte(`{"model":"deepseek-v4.1-flash"}`))
	require.Equal(t, "morph", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
}

func TestApplyClinePassUpstreamPinMergesExistingRouting(t *testing.T) {
	acct := pinConfig(t, map[string]any{
		"mode": "strict", "upstream": "morph",
	})
	body := []byte(`{"model":"m","providerOptions":{"gateway":{"someExisting":true},"other":1},"provider":{"order":["x"]}}`)

	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, body)

	// existing keys preserved
	require.True(t, gjson.GetBytes(out, "providerOptions.gateway.someExisting").Bool())
	require.Equal(t, int64(1), gjson.GetBytes(out, "providerOptions.other").Int())
	// pinned
	require.Equal(t, "morph", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
	// openrouter side gets only; its old order remains untouched (not cleared)
	require.Equal(t, "x", gjson.GetBytes(out, "provider.order.0").String())
	require.Equal(t, "morph", gjson.GetBytes(out, "provider.only.0").String())
}

func TestParseClinePassUpstreamPinJSONRouting(t *testing.T) {
	cfg := map[string]any{"mode": "strict", "upstream": "morph"}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)

	acct, err := ParseClinePassUpstreamPinJSON(string(raw))
	require.NoError(t, err)
	out := ApplyClinePassUpstreamPinWithPlatform(newClinePassPinAccount(nil), acct, []byte(`{"model":"m"}`))
	require.Equal(t, "morph", gjson.GetBytes(out, "providerOptions.gateway.only.0").String())
}
