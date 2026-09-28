package commandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BillingCredits is the read-only upstream billing response. Pointer values
// preserve the distinction between an unavailable measurement and a real zero.
type BillingCredits struct {
	Credits *struct {
		MonthlyCredits *float64 `json:"monthlyCredits"`
	} `json:"credits"`
	WindowLimits *struct {
		FiveHour *BillingWindow `json:"fiveHour"`
		Weekly   *BillingWindow `json:"weekly"`
		Limited  bool           `json:"limited"`
	} `json:"windowLimits"`
}

type BillingWindow struct {
	Cap              *float64        `json:"cap"`
	Used             *float64        `json:"used"`
	Limit            *float64        `json:"limit"`
	Remaining        *float64        `json:"remaining"`
	RemainingPercent *float64        `json:"remainingPercent"`
	ResetAt          json.RawMessage `json:"resetAt"`
}

func (c *Client) BillingCredits(ctx context.Context, apiKey string) (*BillingCredits, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.commandcode.ai"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/alpha/billing/credits", nil)
	if err != nil {
		return nil, fmt.Errorf("invalid CommandCode billing URL")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-cli-environment", "production")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Version != "" {
		req.Header.Set("x-command-code-version", c.Version)
	}
	if c.HTTPClient == nil {
		return nil, fmt.Errorf("CommandCode HTTP client is required")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CommandCode billing request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CommandCode billing status %d", resp.StatusCode)
	}
	var result BillingCredits
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid CommandCode billing response")
	}
	return &result, nil
}
