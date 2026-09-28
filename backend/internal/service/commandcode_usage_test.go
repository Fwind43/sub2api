package service

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/pkg/commandcode"
	"testing"
	"time"
)

func TestCommandCodeUsageWindow(t *testing.T) {
	now := time.Unix(1700000000, 0)
	tests := []struct {
		name, body string
		want       float64
		missing    bool
	}{
		{"cap", "{\"cap\":20,\"used\":5,\"resetAt\":1700003600000}", 25, false},
		{"remaining", "{\"remainingPercent\":30}", 70, false},
		{"overused", "{\"cap\":20,\"used\":25}", 125, false},
		{"zero", "{\"cap\":20,\"used\":0}", 0, false},
		{"missing", "{}", 0, true},
		{"zero cap", "{\"cap\":0,\"used\":3}", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var window commandcode.BillingWindow
			if err := json.Unmarshal([]byte(tt.body), &window); err != nil {
				t.Fatal(err)
			}
			got := commandCodeUsageWindow(&window, now)
			if tt.missing {
				if got != nil {
					t.Fatal("missing measurement presented as zero")
				}
				return
			}
			if got == nil || got.Utilization != tt.want {
				t.Fatalf("got %+v want %v", got, tt.want)
			}
			if tt.name == "cap" && (got.ResetsAt == nil || got.RemainingSeconds != 3600) {
				t.Fatalf("incorrect millisecond reset: %+v", got)
			}
		})
	}
	if commandCodeUsageWindow(nil, now) != nil {
		t.Fatal("nil window")
	}
}

func TestCommandCodeUsagePlan(t *testing.T) {
	cases := []struct{ name, extra, credential, want, raw string }{
		{"missing", "", "", "", ""},
		{"verified plan id", "individual-go", "", "GO", "individual-go"},
		{"explicit go", "GO", "", "GO", "GO"},
		{"credential fallback", "", "individual-go", "GO", "individual-go"},
		{"metadata precedence", "other-plan", "GO", "other-plan", "other-plan"},
		{"unknown", "UNKNOWN", "", "", ""},
		{"trim", "  individual-go  ", "", "GO", "individual-go"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Extra: map[string]any{"subscription_tier": tt.extra}, Credentials: map[string]any{"plan_type": tt.credential}}
			tier, raw := commandCodeUsagePlan(account)
			if tier != tt.want || raw != tt.raw {
				t.Fatalf("got %q/%q want %q/%q", tier, raw, tt.want, tt.raw)
			}
		})
	}
}
