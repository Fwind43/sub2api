package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestH3RecordUsagePreservesFrozenResolutionAndCost(t *testing.T) {
	for _, resolution := range []string{"720p", "768p", "1080p"} {
		t.Run(resolution, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
			quote := &CostBreakdown{TotalCost: 2.5, ActualCost: 0.75, BillingMode: string(BillingModeVideo)}
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				h3Cost: quote, h3Multiplier: 0.3, h3AccountRateMultiplier: 1,
				Result: &OpenAIForwardResult{
					RequestID: "minimax-h3:h3-test", ResponseID: "h3-test", Model: "MiniMax-H3",
					VideoCount: 1, VideoResolution: resolution, VideoDurationSeconds: 5, Duration: time.Second,
				},
				APIKey: &APIKey{ID: 2, UserID: 1}, User: &User{ID: 1},
				Account: &Account{ID: 3, Platform: PlatformMiniMax},
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.NotNil(t, usageRepo.lastLog.VideoResolution)
			require.Equal(t, resolution, *usageRepo.lastLog.VideoResolution)
			require.NotNil(t, usageRepo.lastLog.VideoDurationSeconds)
			require.Equal(t, 5, *usageRepo.lastLog.VideoDurationSeconds)
			require.Equal(t, 1, usageRepo.lastLog.VideoCount)
			require.Equal(t, "minimax-h3:h3-test", usageRepo.lastLog.RequestID)
			require.Equal(t, quote.TotalCost, usageRepo.lastLog.TotalCost)
			require.Equal(t, quote.ActualCost, usageRepo.lastLog.ActualCost)
			require.Equal(t, 0.3, usageRepo.lastLog.RateMultiplier)
			require.Equal(t, 1, billingRepo.calls)
			require.Zero(t, userRepo.deductCalls, "H3 must use durable billing only")
		})
	}
}
