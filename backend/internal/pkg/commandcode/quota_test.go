package commandcode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBillingCredits(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alpha/billing/credits" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Errorf("incorrect request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"credits":{"monthlyCredits":0},"windowLimits":{"fiveHour":{"used":2,"cap":4,"resetAt":1790600000000},"weekly":{"used":3,"cap":10}}}`))
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL, HTTPClient: s.Client()}
	v, e := c.BillingCredits(context.Background(), "fixture")
	if e != nil || v.Credits == nil || v.Credits.MonthlyCredits == nil || *v.Credits.MonthlyCredits != 0 || v.WindowLimits == nil || v.WindowLimits.FiveHour.Cap == nil || *v.WindowLimits.FiveHour.Cap != 4 {
		t.Fatalf("bad credits: %v %v", v, e)
	}
}
func TestBillingCreditsRedactsErrors(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429); w.Write([]byte("secret fixture")) }))
	defer s.Close()
	c := Client{BaseURL: s.URL, HTTPClient: s.Client()}
	_, e := c.BillingCredits(context.Background(), "fixture")
	if e == nil || !strings.Contains(e.Error(), "429") || strings.Contains(e.Error(), "fixture") {
		t.Fatalf("unsafe error: %v", e)
	}
}
