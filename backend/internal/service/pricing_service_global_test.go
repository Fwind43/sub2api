package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// globalPricingCatalogJSON is the catalog served as model_pricing.json. gpt-5.4
// exists here; global-only does not.
const globalPricingCatalogJSON = `{
	"gpt-5.4": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06}
}`

// newGlobalPricingService builds a service with a catalog in DataDir and loads it.
func newGlobalPricingService(t *testing.T, overrideJSON string) *PricingService {
	t.Helper()
	dir := t.TempDir()
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.DataDir = dir
	require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(globalPricingCatalogJSON), 0644))
	if overrideJSON != "" {
		svc.cfg.Pricing.OverrideFile = filepath.Join(dir, "overrides.json")
		require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(overrideJSON), 0644))
	}
	return svc
}

func mustJSON(t *testing.T, body string) json.RawMessage {
	t.Helper()
	require.True(t, json.Valid([]byte(body)), "test fixture must be valid JSON: %s", body)
	return json.RawMessage(body)
}

// The admin-maintained table is the highest local layer: it must beat both the
// catalog and the fallback/override file, and take effect without a restart.
func TestGlobalPricingEntryOverridesCatalogAndOverrideFile(t *testing.T) {
	svc := newGlobalPricingService(t, `{"gpt-5.4": {"input_cost_per_token": 5e-06}}`)
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	require.InDelta(t, 5e-6, svc.pricingData["gpt-5.4"].InputCostPerToken, 1e-12,
		"fallback/override file alone sets 5e-6 before the global layer is added")

	require.NoError(t, svc.SaveGlobalPricingEntry("gpt-5.4", mustJSON(t, `{
		"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 3e-06, "output_cost_per_token": 4e-06
	}`)))

	got := svc.pricingData["gpt-5.4"]
	require.NotNil(t, got)
	require.InDelta(t, 3e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 4e-6, got.OutputCostPerToken, 1e-12)

	// Charging path must observe the new price right away.
	billing := NewBillingService(&config.Config{}, svc)
	cost, err := billing.CalculateCost("gpt-5.4", UsageTokens{InputTokens: 1000, OutputTokens: 500}, 1)
	require.NoError(t, err)
	require.InDelta(t, 1000*3e-6, cost.InputCost, 1e-12)
	require.InDelta(t, 500*4e-6, cost.OutputCost, 1e-12)
}

// A global entry names a model the catalog never had: it must be injected as a
// whole entry instead of waiting for the override-only merge path.
func TestGlobalPricingEntryInjectsModelAbsentFromCatalog(t *testing.T) {
	svc := newGlobalPricingService(t, "")
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	require.NotContains(t, svc.pricingData, "global-only")

	require.NoError(t, svc.SaveGlobalPricingEntry("global-only", mustJSON(t, `{
		"litellm_provider": "commandcode", "mode": "chat",
		"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06
	}`)))

	got := svc.pricingData["global-only"]
	require.NotNil(t, got)
	require.InDelta(t, 1e-6, got.InputCostPerToken, 1e-12)
	require.Equal(t, "commandcode", got.LiteLLMProvider)
}

// Deleting an override restores the catalog value for that model.
func TestDeleteGlobalPricingEntryRestoresCatalogValue(t *testing.T) {
	svc := newGlobalPricingService(t, "")
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	require.NoError(t, svc.SaveGlobalPricingEntry("gpt-5.4", mustJSON(t, `{"input_cost_per_token": 9e-06}`)))
	require.InDelta(t, 9e-6, svc.pricingData["gpt-5.4"].InputCostPerToken, 1e-12)

	require.NoError(t, svc.DeleteGlobalPricingEntry("gpt-5.4"))
	require.InDelta(t, 1e-6, svc.pricingData["gpt-5.4"].InputCostPerToken, 1e-12)

	// Deleting a model that is not present is a no-op.
	require.NoError(t, svc.DeleteGlobalPricingEntry("not-there"))
	require.Empty(t, svc.GlobalPricingEntries())
}

// Entries are persisted inside the pricing data dir and survive a reload.
func TestGlobalPricingEntryPersistsAndReloads(t *testing.T) {
	svc := newGlobalPricingService(t, "")
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	require.NoError(t, svc.SaveGlobalPricingEntry("gpt-5.4", mustJSON(t, `{"input_cost_per_token": 7e-06}`)))

	require.Equal(t, filepath.Join(svc.cfg.Pricing.DataDir, "global_model_prices.json"), svc.GlobalPricingFilePath())
	raw, err := os.ReadFile(svc.GlobalPricingFilePath())
	require.NoError(t, err)
	require.Contains(t, string(raw), "gpt-5.4")

	svc.pricingData = nil
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	require.InDelta(t, 7e-6, svc.pricingData["gpt-5.4"].InputCostPerToken, 1e-12)
}

// Invalid input is rejected and leaves the file untouched.
func TestGlobalPricingEntryRejectsInvalidInput(t *testing.T) {
	svc := newGlobalPricingService(t, "")
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))

	require.Error(t, svc.SaveGlobalPricingEntry("   ", mustJSON(t, `{}`)))
	require.Error(t, svc.SaveGlobalPricingEntry("gpt-5.4", mustJSON(t, `[1,2]`)))
	require.Error(t, svc.SaveGlobalPricingEntry("gpt-5.4", json.RawMessage(`not-json`)))
	require.Empty(t, svc.GlobalPricingEntries())
	_, statErr := os.Stat(svc.GlobalPricingFilePath())
	require.True(t, os.IsNotExist(statErr), "rejected writes must not create the file")
}
