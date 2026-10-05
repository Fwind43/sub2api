//go:build unit

package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// newGlobalTimePricingService 构造一个带目录 + 全局统一价条目的服务。
func newGlobalTimePricingService(t *testing.T, entryJSON string) (*PricingService, *BillingService, *ModelPricingResolver) {
	t.Helper()
	dir := t.TempDir()
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.DataDir = dir
	require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(globalPricingCatalogJSON), 0644))
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	require.NoError(t, svc.SaveGlobalPricingEntry("deepseek-v4-flash", mustPricingRawJSON(t, entryJSON)))

	billing := NewBillingService(&config.Config{}, svc)
	return svc, billing, NewModelPricingResolver(nil, billing)
}

const globalTimePricingEntryJSON = `{
	"litellm_provider": "deepseek", "mode": "chat",
	"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06,
	"time_pricing": {"timezone": "UTC", "weekdays_only": false, "periods": [
		{"start_time": "00:00", "end_time": "23:59", "multiplier": 0.5}
	]}
}`

// 全局统一价条目的 time_pricing 必须按条目自带倍率缩放计费。
func TestGlobalPricingTimePricingScalesCost(t *testing.T) {
	_, billing, resolver := newGlobalTimePricingService(t, globalTimePricingEntryJSON)

	cost, err := billing.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "deepseek-v4-flash",
		Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500},
		RateMultiplier: 1.0, Resolver: resolver,
		PricingAt: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.InDelta(t, 1000*1e-6*0.5, cost.InputCost, 1e-12)
	require.InDelta(t, 500*2e-6*0.5, cost.OutputCost, 1e-12)
	require.InDelta(t, 1000*1e-6*0.5+500*2e-6*0.5, cost.TotalCost, 1e-12)
}

// 全局统一价来源不受 DeepSeek 官方强制价/官方峰谷影响：即使在官方高峰时段，
// 也只按运营者配置的价格 × 全局时段倍率计费。
func TestGlobalPricingTimePricingSkipsDeepSeekOfficialRates(t *testing.T) {
	_, billing, resolver := newGlobalTimePricingService(t, globalTimePricingEntryJSON)

	// 2026-09-14 04:00 UTC 为周一（官方高峰窗口外）→ 官方口径会给低谷价；
	// 2026-09-14 06:00 UTC 为官方高峰 → 官方口径会翻倍。两者都必须与全局价无关。
	for _, at := range []time.Time{
		time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC),
	} {
		cost, err := billing.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "deepseek-v4-flash",
			Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500},
			RateMultiplier: 1.0, Resolver: resolver,
			PricingAt: at,
		})
		require.NoError(t, err)
		require.InDelta(t, 1000*1e-6*0.5, cost.InputCost, 1e-12,
			"global entry must win over official deepseek rates at %s", at)
		require.InDelta(t, 500*2e-6*0.5, cost.OutputCost, 1e-12)
	}
}

// time_pricing 不得泄漏到计费价格解析：条目里的字段必须被剥离，
// 目录价仍按全局价生效，且其他模型不受影响。
func TestGlobalPricingTimePricingDoesNotLeakIntoPricingData(t *testing.T) {
	svc, _, _ := newGlobalTimePricingService(t, globalTimePricingEntryJSON)

	raw, err := json.Marshal(svc.pricingData["deepseek-v4-flash"])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "time_pricing")

	require.InDelta(t, 1e-6, svc.pricingData["deepseek-v4-flash"].InputCostPerToken, 1e-15)
	require.InDelta(t, 1e-6, svc.pricingData["gpt-5.4"].InputCostPerToken, 1e-15,
		"catalog entries without a global override stay untouched")
}

// 非法 time_pricing 必须被保存接口拒绝，且不写入文件。
func TestSaveGlobalPricingEntryRejectsInvalidTimePricing(t *testing.T) {
	dir := t.TempDir()
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.DataDir = dir
	require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(globalPricingCatalogJSON), 0644))
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))

	require.Error(t, svc.SaveGlobalPricingEntry("gpt-5.4", mustPricingRawJSON(t, `{
		"input_cost_per_token": 1e-06,
		"time_pricing": {"timezone": "Not/AZone", "periods": [
			{"start_time": "01:00", "end_time": "02:00", "multiplier": 0.5}
		]}
	}`)))

	// 重叠区间同样被拒绝。
	require.Error(t, svc.SaveGlobalPricingEntry("gpt-5.4", mustPricingRawJSON(t, `{
		"input_cost_per_token": 1e-06,
		"time_pricing": {"timezone": "UTC", "periods": [
			{"start_time": "01:00", "end_time": "03:00", "multiplier": 0.5},
			{"start_time": "02:00", "end_time": "04:00", "multiplier": 0.6}
		]}
	}`)))

	_, statErr := os.Stat(svc.GlobalPricingFilePath())
	require.True(t, os.IsNotExist(statErr), "rejected time_pricing must not create the file")
}
