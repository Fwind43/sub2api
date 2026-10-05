package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clinepass"
)

// clinePassPlanBody mirrors the live /users/me/plan envelope (display name is
// intentionally non-generic in the fixture; live accounts see e.g.
// "Cline Pass (Monthly)"). The threshold values are micro-USD.
const clinePassPlanBody = `{"data":{"plan":{"id":"p1","name":"clinepass-pro-monthly","displayName":"Cline Pass (Monthly)","type":"subscription","interval":"Monthly","isActive":true,"entitlements":{"cline_pass":{"enabled":true,"inferenceCapThreshold":{"last5HoursUsageCostUSDPerUser":2000000000,"last7daysUsageCostUSDPerUser":10000000000,"last30daysUsageCostUSDPerUser":50000000000}}}},"subscriptionId":"sub_1","currentPeriodStart":"2026-09-01T00:00:00Z","currentPeriodEnd":"2026-10-01T00:00:00Z"}}`

const clinePassLimitsBody = `{"data":{"limits":[{"type":"five_hour","percentUsed":0,"resetsAt":"2026-10-05T12:00:00Z"},{"type":"weekly","percentUsed":33.5,"resetsAt":"2026-10-08T00:00:00Z"},{"type":"monthly","percentUsed":12,"resetsAt":"2026-11-01T00:00:00Z"}]}}`

func clinePassTestAccount(baseURL string) *Account {
	return &Account{
		Platform: PlatformClinePass,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "test-token",
			"base_url":     baseURL,
		},
	}
}

func TestClinePassUsageSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer workos:test-token" {
			t.Errorf("unexpected credential: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/me/plan":
			_, _ = w.Write([]byte(clinePassPlanBody))
		case "/users/me/plan/usage-limits":
			_, _ = w.Write([]byte(clinePassLimitsBody))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	service := &AccountUsageService{}
	got, err := service.getClinePassUsage(context.Background(), clinePassTestAccount(server.URL), true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Error != "" {
		t.Fatalf("usage: %+v", got)
	}
	if got.ClinePassPlan != "Cline Pass (Monthly)" || got.ClinePassPlanName != "clinepass-pro-monthly" {
		t.Fatalf("plan: %q / %q", got.ClinePassPlan, got.ClinePassPlanName)
	}
	if got.SubscriptionTier != "Cline Pass (Monthly)" || got.SubscriptionTierRaw != "clinepass-pro-monthly" {
		t.Fatalf("tier: %q / %q", got.SubscriptionTier, got.SubscriptionTierRaw)
	}
	if !got.ClinePassPlanActive || got.ClinePassPlanInterval != "Monthly" {
		t.Fatalf("plan state: %+v", got)
	}
	if got.ClinePassPeriodEnd != "2026-10-01T00:00:00Z" {
		t.Fatalf("period end: %q", got.ClinePassPeriodEnd)
	}
	if got.FiveHour == nil || got.FiveHour.Utilization != 0 {
		t.Fatalf("five_hour: %+v", got.FiveHour)
	}
	if got.SevenDay == nil || got.SevenDay.Utilization != 33.5 {
		t.Fatalf("seven_day: %+v", got.SevenDay)
	}
	if got.ThirtyDay == nil || got.ThirtyDay.Utilization != 12 {
		t.Fatalf("thirty_day: %+v", got.ThirtyDay)
	}
	if got.ClinePassCap == nil || got.ClinePassCap.FiveHourUSD == nil || *got.ClinePassCap.FiveHourUSD != 20 {
		t.Fatalf("cap five hour: %+v", got.ClinePassCap)
	}
	if got.ClinePassCap.ThirtyDayUSD == nil || *got.ClinePassCap.ThirtyDayUSD != 500 {
		t.Fatalf("cap thirty day: %+v", got.ClinePassCap)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"clinepass_plan", "five_hour", "seven_day", "thirty_day", "clinepass_inference_cap"} {
		if _, ok := wire[key]; !ok {
			t.Fatalf("missing %s in %s", key, body)
		}
	}
}

func TestClinePassUsageWindowResets(t *testing.T) {
	now := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)
	future := clinePassUsageWindow(limitOf("five_hour", 10, "2026-10-05T12:00:00Z"), now)
	if future == nil || future.ResetsAt == nil || future.RemainingSeconds != 3600 {
		t.Fatalf("future reset: %+v", future)
	}
	past := clinePassUsageWindow(limitOf("weekly", 5, "2026-10-05T10:00:00Z"), now)
	if past == nil || past.ResetsAt == nil || past.RemainingSeconds != 0 {
		t.Fatalf("past reset must not expose negative remaining: %+v", past)
	}
	invalid := clinePassUsageWindow(limitOf("monthly", 5, "not-a-timestamp"), now)
	if invalid == nil || invalid.ResetsAt != nil {
		t.Fatalf("invalid reset parsed: %+v", invalid)
	}
	if clinePassUsageWindow(limitOf("", 5, "2026-10-05T12:00:00Z"), now) != nil {
		t.Fatal("empty window type must be skipped")
	}
}

func TestClinePassUsageFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid bearer token"}}`))
	}))
	defer server.Close()

	service := &AccountUsageService{}
	got, err := service.getClinePassUsage(context.Background(), clinePassTestAccount(server.URL), true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Error == "" {
		t.Fatalf("expected degraded error payload, got %+v", got)
	}
	if got.FiveHour != nil || got.SevenDay != nil || got.ThirtyDay != nil {
		t.Fatalf("windows must stay empty on failure: %+v", got)
	}

	missing := &Account{Platform: PlatformClinePass, Type: AccountTypeOAuth}
	got, err = service.getClinePassUsage(context.Background(), missing, true)
	if err != nil || got == nil || !strings.Contains(got.Error, "token") {
		t.Fatalf("missing token: %+v err=%v", got, err)
	}
}

func limitOf(windowType string, percent float64, resetsAt string) clinepass.UsageLimit {
	return clinepass.UsageLimit{Type: windowType, PercentUsed: percent, ResetsAt: resetsAt}
}
