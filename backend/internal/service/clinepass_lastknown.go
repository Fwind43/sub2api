package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// ClinePassRouting describes which upstream provider actually served a clinepass
// request, as reported by the relay itself in the response payload.
//
// The relay has two pipelines:
//   - "planner" (Vercel AI Gateway): the chosen provider is reported at
//     choices[0].message.provider_metadata.gateway.routing.finalProvider.
//   - "direct" (OpenRouter): the chosen provider is reported at the top-level
//     `provider` field of the (aggregated) response.
//
// The semantics mirror parseRouting() in cline-pass-switcher/server.js.
type ClinePassRouting struct {
	Pipeline          string   `json:"pipeline"`
	FinalProvider     string   `json:"final_provider"`
	FinalProviderName string   `json:"final_provider_name"`
	CanonicalSlug     string   `json:"canonical_slug,omitempty"`
	Fallbacks         []string `json:"fallbacks,omitempty"`
	Plan              string   `json:"plan,omitempty"`
	Model             string   `json:"model,omitempty"`
}

// ClinePassLastKnown is the persisted "most recent actual upstream" observation
// for one (account, model) pair.
type ClinePassLastKnown struct {
	AccountID  int64     `json:"account_id"`
	Model      string    `json:"model"`
	Provider   string    `json:"provider"`
	Pipeline   string    `json:"pipeline"`
	Canonical  string    `json:"canonical_slug,omitempty"`
	Fallbacks  []string  `json:"fallbacks,omitempty"`
	Plan       string    `json:"plan,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

// ClinePassLastKnownStore persists the last observed upstream provider per
// (account, model). Implementations must be safe for concurrent use.
type ClinePassLastKnownStore interface {
	UpsertClinePassLastKnown(ctx context.Context, rec *ClinePassLastKnown) error
	ListClinePassLastKnown(ctx context.Context, accountID int64) ([]ClinePassLastKnown, error)
	GetClinePassLastKnown(ctx context.Context, accountID int64, model string) (*ClinePassLastKnown, error)
}

// clinePassSlugify mirrors the JS slugify(): lowercase and collapse whitespace
// runs into single dashes.
func clinePassSlugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), "-")
}

// ParseClinePassRouting extracts the actual upstream provider from a clinepass
// response payload. It accepts either a whole Chat Completions response body
// (non-streaming, aggregated) or a single SSE data chunk (streaming), including
// the `{"data": {...}}` envelope shape. Returns nil when no routing information
// is present.
func ParseClinePassRouting(payload []byte) *ClinePassRouting {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" || trimmed == "[DONE]" {
		return nil
	}
	if !gjson.Valid(trimmed) {
		return nil
	}

	root := gjson.Parse(trimmed)
	d := root
	if data := root.Get("data"); data.Exists() && data.Get("choices").Exists() {
		d = data
	}
	if !d.Exists() {
		return nil
	}

	msg := d.Get("choices.0.message")
	rt := msg.Get("provider_metadata.gateway.routing")
	if !rt.Exists() {
		rt = d.Get("provider_metadata.gateway.routing")
	}

	direct := ""
	if p := d.Get("provider"); p.Exists() && p.Type == gjson.String {
		direct = strings.TrimSpace(p.String())
	}

	finalProvider := strings.TrimSpace(rt.Get("finalProvider").String())
	model := strings.TrimSpace(d.Get("model").String())

	canonical := strings.TrimSpace(rt.Get("canonicalSlug").String())
	if canonical == "" && strings.Contains(model, "/") {
		canonical = model
	}

	pipeline := ""
	switch {
	case finalProvider != "":
		pipeline = "planner"
	case direct != "":
		pipeline = "direct"
	}

	slug := finalProvider
	name := finalProvider
	if slug == "" {
		if direct != "" {
			slug = clinePassSlugify(direct)
			name = direct
		}
	}
	if slug == "" && pipeline == "" {
		return nil
	}

	out := &ClinePassRouting{
		Pipeline:          pipeline,
		FinalProvider:     slug,
		FinalProviderName: name,
		CanonicalSlug:     canonical,
		Model:             model,
		Plan:              strings.TrimSpace(rt.Get("planningReasoning").String()),
	}
	if fb := rt.Get("fallbacksAvailable"); fb.Exists() && fb.IsArray() {
		for _, item := range fb.Array() {
			if v := strings.TrimSpace(item.String()); v != "" {
				out.Fallbacks = append(out.Fallbacks, v)
			}
		}
	}
	return out
}

// RecordClinePassRouting persists the observation for one (account, model).
// It is best-effort: callers run it off the hot path and only log failures.
func (s *OpenAIGatewayService) RecordClinePassRouting(ctx context.Context, accountID int64, model string, routing *ClinePassRouting) {
	if s == nil || s.clinePassLastKnown == nil || routing == nil || accountID <= 0 {
		return
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = routing.Model
	}
	if model == "" || strings.TrimSpace(routing.FinalProvider) == "" {
		return
	}
	rec := &ClinePassLastKnown{
		AccountID:  accountID,
		Model:      model,
		Provider:   routing.FinalProvider,
		Pipeline:   routing.Pipeline,
		Canonical:  routing.CanonicalSlug,
		Fallbacks:  routing.Fallbacks,
		Plan:       routing.Plan,
		ObservedAt: time.Now().UTC(),
	}
	if err := s.clinePassLastKnown.UpsertClinePassLastKnown(ctx, rec); err != nil {
		logger.L().Debug("clinepass last-known: persist failed",
			zap.Error(err),
			zap.Int64("account_id", accountID),
			zap.String("model", model),
		)
	}
}

// SetClinePassLastKnownStore wires the persistence backend after construction.
func (s *OpenAIGatewayService) SetClinePassLastKnownStore(store ClinePassLastKnownStore) {
	if s == nil {
		return
	}
	s.clinePassLastKnown = store
}
