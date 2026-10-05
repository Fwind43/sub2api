package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/sjson"
)

// ClinePassUpstreamProbeResult reports the upstream provider slugs available
// for one model on each of the ClinePass relay's two routing pipelines.
//
//   - Vercel AI Gateway (planner pipeline): "Available providers are: ..." text
//   - OpenRouter (direct pipeline): error JSON metadata.available_providers
//
// Raw previews are kept so operators can inspect unexpected relay responses.
type ClinePassUpstreamProbeResult struct {
	Model         string   `json:"model"`
	Vercel        []string `json:"vercel,omitempty"`
	OpenRouter    []string `json:"openrouter,omitempty"`
	VercelRaw     string   `json:"vercel_raw,omitempty"`
	OpenRouterRaw string   `json:"openrouter_raw,omitempty"`
}

var (
	clinePassAvailableProvidersRe = regexp.MustCompile(`Available providers are:\s*([^.]+)`)
	clinePassProviderSlugRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	clinePassProbeRawLimit        = int64(64 * 1024)
)

// ParseClinePassAvailableProviders extracts provider slugs from a relay error
// body. It understands both pipelines: the Vercel AI Gateway plain-text
// "Available providers are: a, b." sentence (which may embed JSON fragments -
// tokens are slug-filtered) and the OpenRouter JSON shape
// error.metadata.available_providers (string items or {slug} objects).
func ParseClinePassAvailableProviders(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if m := clinePassAvailableProvidersRe.FindStringSubmatch(raw); len(m) == 2 {
		if toks := clinePassSlugTokens(m[1]); len(toks) > 0 {
			return toks
		}
	}
	start := strings.Index(raw, "{")
	if start < 0 {
		return nil
	}
	var payload struct {
		Error struct {
			Message  string `json:"message"`
			Metadata struct {
				AvailableProviders []json.RawMessage `json:"available_providers"`
			} `json:"metadata"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw[start:]), &payload); err != nil {
		return nil
	}
	var out []string
	for _, item := range payload.Error.Metadata.AvailableProviders {
		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
				continue
			}
		}
		var obj struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(item, &obj); err == nil {
			if slug := strings.TrimSpace(obj.Slug); slug != "" {
				out = append(out, slug)
			}
		}
	}
	if len(out) > 0 {
		return clinePassPinDedupe(out)
	}
	// Some relays wrap the gateway's plain-text message inside error.message;
	// recurse once on that text so the "Available providers are:" path applies.
	if msg := strings.TrimSpace(payload.Error.Message); msg != "" && msg != raw {
		if m := clinePassAvailableProvidersRe.FindStringSubmatch(msg); len(m) == 2 {
			return clinePassSlugTokens(m[1])
		}
	}
	return nil
}

func clinePassSlugTokens(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || !clinePassProviderSlugRe.MatchString(p) {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return clinePassPinDedupe(out)
}

// buildClinePassUpstreamProbeBody builds the routing-level probe payload: a
// minimal chat request carrying a non-existent provider slug in the pipeline's
// only-list, which makes the relay list the actually available providers
// without billing any tokens.
func buildClinePassUpstreamProbeBody(model string, routingPath string) ([]byte, error) {
	payload := map[string]any{
		"model":      model,
		"messages":   []map[string]any{{"role": "user", "content": "hi"}},
		"max_tokens": 16,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("clinepass probe: marshal payload: %w", err)
	}
	body, err = sjson.SetBytes(body, routingPath+".only", []string{"__probe__"})
	if err != nil {
		return nil, fmt.Errorf("clinepass probe: set %s.only: %w", routingPath, err)
	}
	return body, nil
}

// ProbeClinePassUpstreams harvests the available provider slugs for a model by
// probing both pipelines with a non-existent provider pin. The account's own
// credential and proxy are used; routing errors happen before token billing.
func (s *AccountTestService) ProbeClinePassUpstreams(ctx context.Context, account *Account, modelID string) (*ClinePassUpstreamProbeResult, error) {
	if s == nil || account == nil {
		return nil, fmt.Errorf("clinepass probe: unavailable")
	}
	if account.Platform != PlatformClinePass {
		return nil, fmt.Errorf("clinepass probe: account %d is not a clinepass account", account.ID)
	}
	model := strings.TrimSpace(modelID)
	if model == "" {
		model = ClinePassFallbackTestModel
	}
	model = account.GetMappedModel(model)
	model = clinePassRestoreModelPrefix(model)

	probeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result := &ClinePassUpstreamProbeResult{Model: model}
	rawVercel, err := s.probeClinePassPipeline(probeCtx, account, model, "providerOptions.gateway")
	if err != nil {
		result.VercelRaw = err.Error()
	} else {
		result.VercelRaw = truncateString(rawVercel, 600)
		result.Vercel = ParseClinePassAvailableProviders(rawVercel)
	}
	rawOR, err := s.probeClinePassPipeline(probeCtx, account, model, "provider")
	if err != nil {
		result.OpenRouterRaw = err.Error()
	} else {
		result.OpenRouterRaw = truncateString(rawOR, 600)
		result.OpenRouter = ParseClinePassAvailableProviders(rawOR)
	}
	return result, nil
}

func (s *AccountTestService) probeClinePassPipeline(ctx context.Context, account *Account, model string, routingPath string) (string, error) {
	credential, _ := account.ClinePassCredential()
	if strings.TrimSpace(credential) == "" {
		return "", fmt.Errorf("clinepass probe: no credential available")
	}
	body, err := buildClinePassUpstreamProbeBody(model, routingPath)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, account.ClinePassChatCompletionsURL(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("clinepass probe: build request: %w", err)
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	applyClinePassHeaders(req.Header)
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, clinePassProbeRawLimit))
	return string(raw), nil
}
