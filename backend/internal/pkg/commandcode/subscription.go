package commandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BillingSubscription contains only the fields needed for account-bound plan lookup.
type BillingSubscription struct {
	PlanID string `json:"planId"`
	Status string `json:"status"`
	UserID string `json:"userId"`
}

// BillingSubscription reads a separately authorized web session, not an API key.
// The caller must obtain expectedUserID from the account being queried.
// No cookie, response body, or upstream transport error is returned in errors.
func (c *Client) BillingSubscription(ctx context.Context, cookie, expectedUserID string) (*BillingSubscription, error) {
	if strings.TrimSpace(cookie) == "" || strings.TrimSpace(expectedUserID) == "" {
		return nil, fmt.Errorf("CommandCode subscription requires a session and account identity")
	}
	if c.HTTPClient == nil {
		return nil, fmt.Errorf("CommandCode HTTP client is required")
	}
	// Never send browser credentials to an account-configurable API endpoint.
	if base := strings.TrimRight(c.BaseURL, "/"); base != "" && base != "https://api.commandcode.ai" {
		return nil, fmt.Errorf("CommandCode subscription requires the official endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.commandcode.ai/internal/billing/subscriptions", nil)
	if err != nil {
		return nil, fmt.Errorf("invalid CommandCode subscription request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", cookie)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CommandCode subscription request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CommandCode subscription status %d", resp.StatusCode)
	}
	var envelope struct {
		Success bool                 `json:"success"`
		Data    *BillingSubscription `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("invalid CommandCode subscription response")
	}
	if !envelope.Success || envelope.Data == nil {
		return nil, fmt.Errorf("CommandCode subscription unavailable")
	}
	if envelope.Data.UserID != expectedUserID {
		return nil, fmt.Errorf("CommandCode subscription account mismatch")
	}
	if envelope.Data.Status != "active" || strings.TrimSpace(envelope.Data.PlanID) == "" {
		return nil, fmt.Errorf("CommandCode active subscription unavailable")
	}
	return envelope.Data, nil
}
