package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGemini4ThinkingTierPricingAliases(t *testing.T) {
	tiers := []string{
		"-none",
		"-disabled",
		"-minimal",
		"-low",
		"-medium",
		"-high",
		"-xhigh",
		"-max",
		"-tiered",
	}

	models := []struct {
		base   string
		input  float64
		output float64
		read   float64
	}{
		{base: "gemini-4-argon", input: 2e-6, output: 10e-6, read: 0.1e-6},
		{base: "gemini-4-carbon", input: 2e-6, output: 10e-6, read: 0.1e-6},
		{base: "gemini-4-hydrogen", input: 1.5e-6, output: 7.5e-6, read: 0.075e-6},
		{base: "gemini-4-neon", input: 3e-6, output: 15e-6, read: 0.15e-6},
	}

	for _, m := range models {
		t.Run(m.base, func(t *testing.T) {
			baseCard := &LiteLLMModelPricing{
				InputCostPerToken:       m.input,
				OutputCostPerToken:      m.output,
				CacheReadInputTokenCost: m.read,
			}
			svc := &PricingService{
				pricingData: map[string]*LiteLLMModelPricing{
					m.base: baseCard,
				},
			}

			for _, tier := range tiers {
				alias := m.base + tier
				require.Equal(t, m.base, normalizeGeminiThinkingTierAlias(alias))
				require.Same(t, baseCard, svc.GetIdentifiedModelPricing(alias))
				require.Same(t, baseCard, svc.GetIdentifiedModelPricing("models/"+alias))
				require.Same(t, baseCard, svc.GetIdentifiedModelPricing("projects/p/locations/global/publishers/google/models/"+alias))
			}

			// An explicit tier entry must take precedence over base normalization.
			overrideCard := &LiteLLMModelPricing{InputCostPerToken: m.input * 2}
			svc.pricingData[m.base+"-xhigh"] = overrideCard
			require.Same(t, overrideCard, svc.GetIdentifiedModelPricing(m.base+"-xhigh"))

			// Cyber variants must not normalize down to the base model.
			require.Equal(t, m.base+"-cyber", normalizeGeminiThinkingTierAlias(m.base+"-cyber"))
		})
	}
}

func TestGemini4FallbackBillingAndPriority(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{
		InputTokens:     1_000_000,
		OutputTokens:    1_000_000,
		CacheReadTokens: 1_000_000,
	}
	tiers := []string{
		"",
		"-none",
		"-disabled",
		"-minimal",
		"-low",
		"-medium",
		"-high",
		"-xhigh",
		"-max",
		"-tiered",
	}

	cases := []struct {
		base   string
		input  float64
		output float64
		read   float64
	}{
		{base: "gemini-4-argon", input: 2.0, output: 10.0, read: 0.1},
		{base: "gemini-4-carbon", input: 2.0, output: 10.0, read: 0.1},
		{base: "gemini-4-hydrogen", input: 1.5, output: 7.5, read: 0.075},
		{base: "gemini-4-neon", input: 3.0, output: 15.0, read: 0.15},
	}

	for _, tc := range cases {
		for _, tier := range tiers {
			for _, prefix := range []string{"", "models/"} {
				model := prefix + tc.base + tier
				t.Run(model, func(t *testing.T) {
					cost, err := svc.CalculateCost(model, tokens, 1)
					require.NoError(t, err)
					require.InDelta(t, tc.input, cost.InputCost, 1e-12)
					require.InDelta(t, tc.output, cost.OutputCost, 1e-12)
					require.InDelta(t, tc.read, cost.CacheReadCost, 1e-12)
					require.InDelta(t, tc.input+tc.output+tc.read, cost.TotalCost, 1e-12)

					priority, err := svc.CalculateCostWithServiceTier(model, tokens, 1, "priority")
					require.NoError(t, err)
					require.InDelta(t, (tc.input+tc.output+tc.read)*1.8, priority.TotalCost, 1e-12)

					require.Zero(t, svc.CacheStoragePricePerTokenHour(model))
				})
			}
		}
	}

	// Cyber variants: Argon Cyber keeps its separate $4/$20/$0.20 fallback (+1.8x priority),
	// while Neon Cyber must not silently inherit the non-cyber Neon rate.
	cyberPriority, err := svc.CalculateCostWithServiceTier("gemini-4-argon-cyber-permissive", tokens, 1, "priority")
	require.NoError(t, err)
	require.InDelta(t, 24.2*1.8, cyberPriority.TotalCost, 1e-12)

	_, err = svc.CalculateCost("gemini-4-neon-cyber", tokens, 1)
	require.Error(t, err)
}

func TestDefaultPricingIncludesGemini4NeonRates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	pricingSvc := &PricingService{}
	pricingData, err := pricingSvc.parsePricingData(data)
	require.NoError(t, err)
	pricingSvc.pricingData = pricingData

	for _, model := range []string{
		"gemini-4-neon",
		"gemini-4-neon-xhigh",
		"gemini-4-neon-minimal",
		"gemini-4-neon-high",
		"models/gemini-4-neon-xhigh",
		"gemini/gemini-4-neon",
	} {
		t.Run(model, func(t *testing.T) {
			pricing := pricingSvc.GetIdentifiedModelPricing(model)
			require.NotNil(t, pricing)
			require.Equal(t, "gemini", pricing.LiteLLMProvider)
			require.InDelta(t, 3e-6, pricing.InputCostPerToken, 1e-15)
			require.InDelta(t, 15e-6, pricing.OutputCostPerToken, 1e-15)
			require.InDelta(t, 0.15e-6, pricing.CacheReadInputTokenCost, 1e-15)
			require.InDelta(t, 5.4e-6, pricing.InputCostPerTokenPriority, 1e-15)
			require.InDelta(t, 27e-6, pricing.OutputCostPerTokenPriority, 1e-15)
			require.InDelta(t, 0.27e-6, pricing.CacheReadInputTokenCostPriority, 1e-15)
			require.Zero(t, pricing.LongContextInputTokenThreshold)
			require.Zero(t, pricing.CacheStorageCostPerTokenPerHour)
		})
	}
	require.Contains(t, pricingSvc.ListModelNamesByProvider("gemini"), "gemini-4-neon")

	var catalog map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &catalog))
	require.Equal(t, float64(1048576), catalog["gemini-4-neon"]["max_input_tokens"])
	require.Equal(t, float64(65536), catalog["gemini-4-neon"]["max_output_tokens"])
	require.Equal(t, float64(65536), catalog["gemini-4-neon"]["max_tokens"])
}
