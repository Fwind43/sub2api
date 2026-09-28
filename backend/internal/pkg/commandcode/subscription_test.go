package commandcode

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type subscriptionTestDoer func(*http.Request) (*http.Response, error)

func (f subscriptionTestDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestBillingSubscriptionAccountBoundary(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"active go", `{"success":true,"data":{"planId":"individual-go","status":"active","userId":"fixture-user"}}`, 200, true},
		{"different account", `{"success":true,"data":{"planId":"individual-go","status":"active","userId":"other-user"}}`, 200, false},
		{"inactive", `{"success":true,"data":{"planId":"individual-go","status":"canceled","userId":"fixture-user"}}`, 200, false},
		{"missing plan", `{"success":true,"data":{"status":"active","userId":"fixture-user"}}`, 200, false},
		{"no subscription", `{"success":true,"data":null}`, 200, false},
		{"failed envelope", `{"success":false,"data":{"planId":"individual-go","status":"active","userId":"fixture-user"}}`, 200, false},
		{"unauthorized", `secret-cookie`, 401, false},
		{"invalid body", `secret-cookie`, 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{HTTPClient: subscriptionTestDoer(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.String() != "https://api.commandcode.ai/internal/billing/subscriptions" || r.Header.Get("Cookie") != "secret-cookie" || r.Header.Get("Authorization") != "" {
					t.Fatal("incorrect subscription request")
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})}
			got, err := client.BillingSubscription(context.Background(), "secret-cookie", "fixture-user")
			if tt.valid {
				if err != nil || got == nil || got.PlanID != "individual-go" {
					t.Fatalf("unexpected subscription: %v %v", got, err)
				}
			} else if err == nil || got != nil {
				t.Fatal("unverified subscription accepted")
			}
			if err != nil && strings.Contains(err.Error(), "secret-cookie") {
				t.Fatal("credential disclosed")
			}
		})
	}
}

func TestBillingSubscriptionRejectsUnsafeInputs(t *testing.T) {
	calls := 0
	client := Client{HTTPClient: subscriptionTestDoer(func(*http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("secret-cookie") })}
	for _, args := range [][3]string{{"", "fixture-user", ""}, {"secret-cookie", "", ""}, {"secret-cookie", "fixture-user", "https://example.invalid"}} {
		client.BaseURL = args[2]
		if _, err := client.BillingSubscription(context.Background(), args[0], args[1]); err == nil {
			t.Fatal("unsafe input accepted")
		}
	}
	if calls != 0 {
		t.Fatal("unsafe request sent")
	}
	client.BaseURL = ""
	_, err := client.BillingSubscription(context.Background(), "secret-cookie", "fixture-user")
	if err == nil || strings.Contains(err.Error(), "secret-cookie") {
		t.Fatal("transport error not redacted")
	}
}
