package commandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func modelRouteAlias(id string) string {
	if i := strings.IndexAny(id, ":/"); i > 0 && i < len(id)-1 {
		return id[i+1:]
	}
	return id
}

// Repair only a rejected route, using this account's catalog, never a guessed provider.
func (c *Client) resolveRejectedModel(ctx context.Context, apiKey, model string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.commandcode.ai"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/provider/v1/models", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Version != "" {
		req.Header.Set("x-command-code-version", c.Version)
	}
	req.Header.Set("x-cli-environment", "production")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model catalog status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil {
		return "", err
	}
	if len(data) > 4*1024*1024 {
		return "", fmt.Errorf("model catalog too large")
	}
	var envelope struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	var entries []json.RawMessage
	if err = json.Unmarshal(data, &envelope); err == nil {
		entries = append(envelope.Data, envelope.Models...)
	} else if err = json.Unmarshal(data, &entries); err != nil {
		return "", err
	}
	matches := map[string]bool{}
	alias := modelRouteAlias(strings.TrimSpace(model))
	for _, raw := range entries {
		var id string
		if json.Unmarshal(raw, &id) != nil {
			var entry struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &entry) != nil {
				continue
			}
			id = strings.TrimSpace(entry.ID)
			if id == "" {
				id = strings.TrimSpace(entry.Slug)
			}
			if id == "" {
				id = strings.TrimSpace(entry.Name)
			}
		}
		id = strings.TrimSpace(id)
		if id == model {
			return model, nil
		}
		if id != "" && modelRouteAlias(id) == alias {
			matches[id] = true
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("model catalog has %d matching routes", len(matches))
	}
	for id := range matches {
		return id, nil
	}
	return "", fmt.Errorf("model route not found")
}
