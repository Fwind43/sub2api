package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clinepass"
	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

// clinePassUsageSnapshot holds one credential-scoped ClinePass billing probe.
// Plan and rolling windows are fetched together; a failure of one does not
// discard the other, so the UI can still show partial data.
type clinePassUsageSnapshot struct {
	plan      *clinepass.PlanInfo
	limits    []clinepass.UsageLimit
	fetchedAt time.Time
	err       error
}

const (
	clinePassUsageTTL      = 3 * time.Minute
	clinePassUsageErrorTTL = 1 * time.Minute
)

// getClinePassUsage queries /users/me/plan and /users/me/plan/usage-limits with
// the account's own WorkOS access token. Never mutates account state.
func (s *AccountUsageService) getClinePassUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	token := account.GetClinePassAccessToken()
	if strings.TrimSpace(token) == "" {
		return &UsageInfo{Error: "ClinePass access token is missing"}, nil
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	baseURL := account.ClinePassBaseURL()
	cacheKey := fmt.Sprintf("clinepass:%d:%x", account.ID, sha256.Sum256([]byte(token+"\x00"+baseURL+"\x00"+proxyURL)))

	lookup := func() *clinePassUsageSnapshot {
		if s.cache == nil {
			return nil
		}
		value, ok := s.cache.clinePassCache.Load(cacheKey)
		if !ok {
			return nil
		}
		entry := value.(*clinePassUsageSnapshot)
		ttl := clinePassUsageTTL
		if entry.err != nil {
			ttl = clinePassUsageErrorTTL
		}
		if time.Since(entry.fetchedAt) >= ttl {
			return nil
		}
		return entry
	}

	fetch := func() (any, error) {
		if !force {
			if cached := lookup(); cached != nil {
				return cached, nil
			}
		}
		entry := &clinePassUsageSnapshot{}
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		client, err := httppool.GetClient(httppool.Options{ProxyURL: proxyURL, Timeout: 20 * time.Second, ResponseHeaderTimeout: 15 * time.Second})
		if err != nil {
			entry.err = fmt.Errorf("ClinePass proxy configuration is invalid")
		} else {
			upstream := &clinepass.Client{BaseURL: baseURL, HTTPClient: client}
			plan, planErr := upstream.FetchPlan(requestCtx, token)
			limits, limitsErr := upstream.FetchUsageLimits(requestCtx, token)
			entry.plan, entry.limits = plan, limits
			if planErr != nil {
				entry.err = planErr
			} else if limitsErr != nil {
				entry.err = limitsErr
			}
		}
		entry.fetchedAt = time.Now()
		if s.cache != nil {
			s.cache.clinePassCache.Store(cacheKey, entry)
		}
		return entry, nil
	}

	var entry *clinePassUsageSnapshot
	if !force {
		entry = lookup()
	}
	if entry == nil {
		var result any
		var err error
		if s.cache != nil {
			result, err, _ = s.cache.apiFlight.Do(cacheKey, fetch)
		} else {
			result, err = fetch()
		}
		if err != nil {
			return nil, err
		}
		entry = result.(*clinePassUsageSnapshot)
	}

	// A total failure (neither plan nor windows) is reported as an error, but a
	// plan-only or limits-only payload still renders.
	if entry.err != nil && entry.plan == nil && len(entry.limits) == 0 {
		return &UsageInfo{Error: entry.err.Error()}, nil
	}

	now := time.Now()
	fetchedAt := entry.fetchedAt
	usage := &UsageInfo{Source: "active", UpdatedAt: &fetchedAt}
	if entry.plan != nil {
		usage.SubscriptionTier, usage.SubscriptionTierRaw = clinePassPlanTier(entry.plan)
		usage.ClinePassPlan = strings.TrimSpace(entry.plan.Plan.DisplayName)
		usage.ClinePassPlanName = strings.TrimSpace(entry.plan.Plan.Name)
		usage.ClinePassPlanInterval = strings.TrimSpace(entry.plan.Plan.Interval)
		usage.ClinePassPlanActive = entry.plan.Plan.IsActive
		if entry.plan.CurrentPeriodEnd != "" {
			usage.ClinePassPeriodEnd = entry.plan.CurrentPeriodEnd
		}
		caps := entry.plan.Plan.Entitlements.ClinePass.InferenceCapThreshold
		usage.ClinePassCap = &ClinePassInferenceCap{
			FiveHourUSD:  microUSDToUSD(caps.Last5HoursUsageCostMicroUSD),
			SevenDayUSD:  microUSDToUSD(caps.Last7DaysUsageCostMicroUSD),
			ThirtyDayUSD: microUSDToUSD(caps.Last30DaysUsageCostMicroUSD),
		}
	}
	for _, limit := range entry.limits {
		progress := clinePassUsageWindow(limit, now)
		if progress == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(limit.Type)) {
		case clinepass.UsageLimitTypeFiveHour:
			usage.FiveHour = progress
		case clinepass.UsageLimitTypeWeekly:
			usage.SevenDay = progress
		case clinepass.UsageLimitTypeMonthly:
			usage.ThirtyDay = progress
		}
	}
	if entry.err != nil {
		usage.Error = entry.err.Error()
	}
	return usage, nil
}

func clinePassUsageWindow(limit clinepass.UsageLimit, now time.Time) *UsageProgress {
	if strings.TrimSpace(limit.Type) == "" {
		return nil
	}
	progress := &UsageProgress{Utilization: limit.PercentUsed}
	if reset, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(limit.ResetsAt)); err == nil && !reset.IsZero() {
		reset = reset.UTC()
		progress.ResetsAt = &reset
		if remaining := reset.Sub(now).Seconds(); remaining > 0 {
			progress.RemainingSeconds = int(remaining)
		}
	}
	return progress
}

// clinePassPlanTier exposes the upstream plan display name. ClinePass has no
// normalized tier vocabulary, so the display name is both the raw and the
// presentable value.
func clinePassPlanTier(plan *clinepass.PlanInfo) (string, string) {
	if plan == nil {
		return "", ""
	}
	display := strings.TrimSpace(plan.Plan.DisplayName)
	raw := strings.TrimSpace(plan.Plan.Name)
	if display == "" {
		display = raw
	}
	return display, raw
}

func microUSDToUSD(value *float64) *float64 {
	if value == nil {
		return nil
	}
	usd := *value / 1e8
	return &usd
}
