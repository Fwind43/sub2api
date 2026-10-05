package clinepass

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Usage window types returned by GET /users/me/plan/usage-limits.
const (
	UsageLimitTypeFiveHour = "five_hour"
	UsageLimitTypeWeekly   = "weekly"
	UsageLimitTypeMonthly  = "monthly"
)

// PlanThreshold mirrors plan.entitlements.cline_pass.inferenceCapThreshold.
// Values are micro-USD (1e8 units per USD), matching the /usages ledger.
type PlanThreshold struct {
	Last5HoursUsageCostMicroUSD *float64 `json:"last5HoursUsageCostUSDPerUser,omitempty"`
	Last7DaysUsageCostMicroUSD  *float64 `json:"last7daysUsageCostUSDPerUser,omitempty"`
	Last30DaysUsageCostMicroUSD *float64 `json:"last30daysUsageCostUSDPerUser,omitempty"`
}

// Plan describes the subscription attached to a ClinePass account.
type Plan struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Type         string `json:"type"`
	Interval     string `json:"interval"`
	PricePerSeat *int64 `json:"pricePerSeatCents,omitempty"`
	IsActive     bool   `json:"isActive"`

	Entitlements struct {
		ClinePass struct {
			Enabled               bool          `json:"enabled"`
			InferenceCapThreshold PlanThreshold `json:"inferenceCapThreshold"`
		} `json:"cline_pass"`
	} `json:"entitlements"`
}

// PlanInfo is the current subscription state for the authenticated account.
type PlanInfo struct {
	Plan               Plan   `json:"plan"`
	SubscriptionID     string `json:"subscriptionId"`
	CurrentPeriodStart string `json:"currentPeriodStart"`
	CurrentPeriodEnd   string `json:"currentPeriodEnd"`
	CancelAt           string `json:"cancelAt"`
	CanceledAt         string `json:"canceledAt"`
}

// UsageLimit is one rolling-window quota entry.
type UsageLimit struct {
	Type        string  `json:"type"`
	PercentUsed float64 `json:"percentUsed"`
	ResetsAt    string  `json:"resetsAt"`
}

// FetchPlan returns the current plan for the authenticated ClinePass account.
func (c *Client) FetchPlan(ctx context.Context, accessToken string) (*PlanInfo, error) {
	raw, status, err := c.doRawJSON(ctx, "GET", ResolveURL(c.baseURL(), "/users/me/plan"), nil, accessToken)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("clinepass: plan request failed with status %d: %s", status, truncateBody(raw))
	}
	var envelope struct {
		Data PlanInfo `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("clinepass: decode plan response: %w", err)
	}
	return &envelope.Data, nil
}

// FetchUsageLimits returns the rolling 5h / weekly / monthly quota windows.
func (c *Client) FetchUsageLimits(ctx context.Context, accessToken string) ([]UsageLimit, error) {
	raw, status, err := c.doRawJSON(ctx, "GET", ResolveURL(c.baseURL(), "/users/me/plan/usage-limits"), nil, accessToken)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("clinepass: usage-limits request failed with status %d: %s", status, truncateBody(raw))
	}
	var envelope struct {
		Data struct {
			Limits []UsageLimit `json:"limits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("clinepass: decode usage-limits response: %w", err)
	}
	return envelope.Data.Limits, nil
}

func truncateBody(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) > 300 {
		return trimmed[:300] + "..."
	}
	return trimmed
}
