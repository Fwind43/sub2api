package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/commandcode"
	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

type commandCodeUsageCache struct {
	credits   *commandcode.BillingCredits
	fetchedAt time.Time
	err       error
}

// Query the account directly. Never route through CPA or change account status.
func (s *AccountUsageService) getCommandCodeUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	key := strings.TrimSpace(account.GetCredential("api_key"))
	if key == "" {
		return &UsageInfo{Error: "CommandCode API key is missing"}, nil
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	baseURL := account.GetCredential("base_url")
	cacheKey := fmt.Sprintf("commandcode:%d:%x", account.ID, sha256.Sum256([]byte(key+"\x00"+baseURL+"\x00"+proxyURL)))
	lookup := func() *commandCodeUsageCache {
		if s.cache == nil {
			return nil
		}
		value, ok := s.cache.commandCodeCache.Load(cacheKey)
		if !ok {
			return nil
		}
		entry := value.(*commandCodeUsageCache)
		ttl := time.Minute
		if entry.err != nil {
			ttl = 15 * time.Second
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
		entry := &commandCodeUsageCache{}
		requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		client, err := httppool.GetClient(httppool.Options{ProxyURL: proxyURL, Timeout: 15 * time.Second, ResponseHeaderTimeout: 10 * time.Second})
		if err != nil {
			entry.err = fmt.Errorf("CommandCode proxy configuration is invalid")
		} else {
			upstream := &commandcode.Client{BaseURL: baseURL, HTTPClient: client}
			entry.credits, entry.err = upstream.BillingCredits(requestCtx, key)
		}
		entry.fetchedAt = time.Now()
		if s.cache != nil {
			s.cache.commandCodeCache.Store(cacheKey, entry)
		}
		return entry, nil
	}
	var entry *commandCodeUsageCache
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
		entry = result.(*commandCodeUsageCache)
	}
	usage := &UsageInfo{Source: "active", UpdatedAt: &entry.fetchedAt}
	usage.SubscriptionTier, usage.SubscriptionTierRaw = commandCodeUsagePlan(account)
	if entry.err != nil {
		usage.Error = entry.err.Error()
		return usage, nil
	}
	if entry.credits != nil && entry.credits.Credits != nil {
		usage.CommandCodeMonthlyCredits = entry.credits.Credits.MonthlyCredits
	}
	if entry.credits != nil && entry.credits.WindowLimits != nil {
		usage.FiveHour = commandCodeUsageWindow(entry.credits.WindowLimits.FiveHour, time.Now())
		usage.SevenDay = commandCodeUsageWindow(entry.credits.WindowLimits.Weekly, time.Now())
	}
	return usage, nil
}

func commandCodeUsageWindow(window *commandcode.BillingWindow, now time.Time) *UsageProgress {
	if window == nil {
		return nil
	}
	var utilization float64
	cap := window.Cap
	if cap == nil {
		cap = window.Limit
	}
	switch {
	case window.Used != nil && cap != nil && *cap > 0:
		utilization = *window.Used / *cap * 100
	case window.RemainingPercent != nil:
		utilization = 100 - *window.RemainingPercent
	case window.Remaining != nil && cap != nil && *cap > 0:
		utilization = (1 - *window.Remaining / *cap) * 100
	default:
		return nil // Absent measurements must not appear as zero usage.
	}
	if math.IsNaN(utilization) || math.IsInf(utilization, 0) {
		return nil
	}
	progress := &UsageProgress{Utilization: math.Max(0, utilization)}
	var raw any
	if json.Unmarshal(window.ResetAt, &raw) == nil {
		var reset time.Time
		switch value := raw.(type) {
		case string:
			reset, _ = time.Parse(time.RFC3339Nano, value)
			if reset.IsZero() {
				if n, err := strconv.ParseFloat(value, 64); err == nil {
					reset = commandCodeResetTime(n)
				}
			}
		case float64:
			reset = commandCodeResetTime(value)
		}
		if !reset.IsZero() {
			progress.ResetsAt = &reset
			if remaining := reset.Sub(now).Seconds(); remaining > 0 {
				progress.RemainingSeconds = int(remaining)
			}
		}
	}
	return progress
}

func commandCodeResetTime(value float64) time.Time {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return time.Time{}
	}
	if value > 1e12 {
		value /= 1000
	}
	if value > 253402300799 {
		return time.Time{}
	}
	return time.Unix(int64(value), 0).UTC()
}

// Explicit account metadata may be populated by an independently verified import.
// API-key type and usage limits are never evidence of a subscription tier.
func commandCodeUsagePlan(account *Account) (string, string) {
	raw := strings.TrimSpace(account.GetExtraString("subscription_tier"))
	if raw == "" {
		raw = strings.TrimSpace(account.GetCredential("plan_type"))
	}
	switch strings.ToLower(raw) {
	case "go", "individual-go":
		return "GO", raw
	case "", "unknown":
		return "", ""
	default:
		return raw, raw
	}
}
