package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ClinePassUpstreamPinExtraKey identifies ignored legacy account configuration.
// Platform settings are the sole source of upstream pinning.
const ClinePassUpstreamPinExtraKey = "clinepass_upstream_pin"

// ClinePassUpstreamPinConfig mirrors the pin configuration. Zero values mean
// "not set"; model-level overrides replace top-level fields when non-zero.
type ClinePassUpstreamPinConfig struct {
	Mode      string   `json:"mode"`
	Upstream  string   `json:"upstream"`
	Order     []string `json:"order"`
	Only      []string `json:"only"`
	Has       []string `json:"has,omitempty"`
	Exclude   []string `json:"exclude"`
	Upstreams []string `json:"upstreams"`
	Sort      string   `json:"sort"`
	Pipelines string   `json:"pipelines"`
	// MatchPrefixes restricts the pin to models whose bare id (the segment after
	// the last "/") starts with one of these prefixes. Empty means every model.
	MatchPrefixes []string                              `json:"match_prefixes"`
	Models        map[string]ClinePassUpstreamPinConfig `json:"models"`
}

// clinePassSortAliases maps the switcher's sort keys to the OpenRouter
// pipeline values; the Vercel AI Gateway accepts the raw keys.
var clinePassSortAliases = map[string]string{
	"cost": "price",
	"ttft": "latency",
	"tps":  "throughput",
}

func (c *ClinePassUpstreamPinConfig) normalize() *ClinePassUpstreamPinConfig {
	if c == nil {
		return nil
	}
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	c.Upstream = strings.TrimSpace(c.Upstream)
	c.Sort = strings.ToLower(strings.TrimSpace(c.Sort))
	c.Pipelines = strings.ToLower(strings.TrimSpace(c.Pipelines))
	c.Order = clinePassPinDedupe(c.Order)
	c.Only = clinePassPinDedupe(c.Only)
	c.Has = clinePassPinDedupe(c.Has)
	c.Exclude = clinePassPinDedupe(c.Exclude)
	c.Upstreams = clinePassPinDedupe(c.Upstreams)
	c.MatchPrefixes = clinePassPinDedupeLower(c.MatchPrefixes)

	if c.Mode == "off" {
		return nil
	}
	switch c.Mode {
	case "", "strict", "preferred":
	default:
		c.Mode = ""
	}
	switch c.Pipelines {
	case "", "both", "vercel", "openrouter":
	default:
		c.Pipelines = ""
	}
	return c
}

// clinePassPinNeverPinnedPrefixes lists model families that must never be
// pinned: free/relay-managed models keep their default upstream routing
// (mirrors the magpie pin gate).
var clinePassPinNeverPinnedPrefixes = []string{"cline-free"}

// clinePassModelBasename lowercases a model id and drops the vendor prefix:
// "cline-pass/deepseek-v4-pro" -> "deepseek-v4-pro".
func clinePassModelBasename(model string) string {
	base := strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	return base
}

// MatchesModel reports whether the pin applies to the given outbound model id.
// Models whose bare id starts with a never-pinned prefix (cline-free/) are
// always left untouched; when MatchPrefixes is empty every other model matches.
func (c *ClinePassUpstreamPinConfig) MatchesModel(model string) bool {
	if c == nil {
		return false
	}
	id := strings.ToLower(strings.TrimSpace(model))
	base := clinePassModelBasename(id)
	for _, prefix := range clinePassPinNeverPinnedPrefixes {
		if strings.HasPrefix(id, prefix) || strings.HasPrefix(base, prefix) {
			return false
		}
	}
	if len(c.MatchPrefixes) == 0 {
		return true
	}
	for _, prefix := range c.MatchPrefixes {
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix != "" && strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return false
}

// ParseClinePassUpstreamPinJSON decodes a platform-level pin configuration
// stored as a settings JSON string. Empty input yields nil (no pin).
func ParseClinePassUpstreamPinJSON(raw string) (*ClinePassUpstreamPinConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var cfg ClinePassUpstreamPinConfig
	if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return nil, fmt.Errorf("clinepass pin: invalid settings: %w", err)
	}
	return cfg.normalize(), nil
}

// clinePassPinDedupeLower trims/lowercases prefix entries and drops blanks.
func clinePassPinDedupeLower(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// effectiveForModel merges one model rule over the top-level fields.
// Bare and cline-pass/ qualified keys are aliases. Exact IDs win over trailing-*
// prefix rules; otherwise the longest matching prefix wins, then defaults.
func (c *ClinePassUpstreamPinConfig) effectiveForModel(model string) *ClinePassUpstreamPinConfig {
	eff := *c
	eff.Models = nil
	stripped := clinePassStripModelPrefix(strings.TrimSpace(model))
	var override *ClinePassUpstreamPinConfig
	bestPrefixLen := -1
	for key, rule := range c.Models {
		k := clinePassStripModelPrefix(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		if k == stripped {
			m := rule
			override = &m
			break
		}
		if strings.Count(k, "*") == 1 && strings.HasSuffix(k, "*") {
			prefix := strings.TrimSuffix(k, "*")
			if prefix != "" && strings.HasPrefix(stripped, prefix) && len(prefix) > bestPrefixLen {
				m := rule
				override = &m
				bestPrefixLen = len(prefix)
			}
		}
	}
	if override == nil {
		return &eff
	}
	// An explicit model rule is independent of the default prefix gate.
	eff.MatchPrefixes = nil
	if override.Mode != "" {
		eff.Mode = strings.ToLower(strings.TrimSpace(override.Mode))
	}
	if override.Upstream != "" {
		eff.Upstream = strings.TrimSpace(override.Upstream)
	}
	if len(override.Order) > 0 {
		eff.Order = override.Order
	}
	if len(override.Only) > 0 {
		eff.Only = override.Only
	}
	if override.Has != nil {
		eff.Has = override.Has
	}
	if len(override.Exclude) > 0 {
		eff.Exclude = override.Exclude
	}
	if len(override.Upstreams) > 0 {
		eff.Upstreams = override.Upstreams
	}
	if override.Sort != "" {
		eff.Sort = override.Sort
	}
	if override.Pipelines != "" {
		eff.Pipelines = strings.ToLower(strings.TrimSpace(override.Pipelines))
	}
	return &eff
}

// ApplyClinePassUpstreamPinWithPlatform injects only platform routing settings
// for ClinePass accounts. Legacy account pins, including mode=off, are ignored.
func ApplyClinePassUpstreamPinWithPlatform(account *Account, platform *ClinePassUpstreamPinConfig, body []byte) []byte {
	if account == nil || !account.IsClinePass() || platform == nil || len(body) == 0 {
		return body
	}
	local := *platform
	cfg := local.normalize()
	if cfg == nil {
		return body
	}
	out, err := applyClinePassUpstreamPinToBody(body, cfg)
	if err != nil {
		slog.Warn("clinepass_upstream_pin_apply_failed", "account_id", account.ID, "error", err)
		return body
	}
	return out
}

func applyClinePassUpstreamPinToBody(body []byte, cfg *ClinePassUpstreamPinConfig) ([]byte, error) {
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	eff := cfg.effectiveForModel(model)
	if eff == nil || eff.Mode == "off" {
		return body, nil
	}
	if !eff.MatchesModel(model) {
		return body, nil
	}

	useVercel := eff.Pipelines == "" || eff.Pipelines == "both" || eff.Pipelines == "vercel"
	useOpenRouter := eff.Pipelines == "" || eff.Pipelines == "both" || eff.Pipelines == "openrouter"
	if !useVercel && !useOpenRouter {
		return body, nil
	}

	upstream := eff.Upstream
	strict := eff.Mode != "preferred"

	// The relay ignores "exclude"/"ignore" fields (verified upstream), so an
	// exclusion list is expanded into an allow-list using the known provider
	// slugs. Explicit "only" takes precedence over the exclusion expansion.
	allowList := eff.Only
	if len(allowList) == 0 && len(eff.Exclude) > 0 && len(eff.Upstreams) > 0 {
		// Mirror the switcher: the pinned upstream is never excluded by
		// accident (excludeList.filter(u => u !== upstream)).
		exclude := make([]string, 0, len(eff.Exclude))
		for _, e := range eff.Exclude {
			if e == upstream {
				continue
			}
			exclude = append(exclude, e)
		}
		expanded := clinePassPinDedupe(eff.Upstreams)
		allowList = filterOut(expanded, exclude...)
	}
	if !(upstream != "" || eff.Sort != "" || len(allowList) > 0) {
		return body, nil
	}

	// Build the per-pipeline routing intent once; both pipelines share the
	// same selection semantics, sort keys differ (OR aliases).
	type routingIntent struct {
		only  []string
		order []string
		sort  string
	}
	intent := routingIntent{sort: eff.Sort}
	if upstream != "" {
		if strict {
			intent.only = []string{upstream}
			intent.order = nil
		} else {
			order := make([]string, 0, 1+len(eff.Order))
			order = append(order, upstream)
			order = append(order, eff.Order...)
			intent.order = clinePassPinDedupe(order)
			if len(allowList) > 0 {
				intent.only = allowList
			}
		}
	} else if len(allowList) > 0 {
		intent.only = allowList
	}

	updated := body
	var err error
	if useVercel {
		updated, err = applyClinePassRoutingIntent(updated, "providerOptions.gateway", intent.only, intent.order, intent.sort)
		if err != nil {
			return body, err
		}
		if len(eff.Has) > 0 {
			updated, err = sjson.SetBytes(updated, "providerOptions.gateway.has", eff.Has)
			if err != nil {
				return body, fmt.Errorf("clinepass pin: set providerOptions.gateway.has: %w", err)
			}
		}
	}
	if useOpenRouter {
		orSort := intent.sort
		if alias, ok := clinePassSortAliases[orSort]; ok {
			orSort = alias
		}
		updated, err = applyClinePassRoutingIntent(updated, "provider", intent.only, intent.order, orSort)
		if err != nil {
			return body, err
		}
	}
	return updated, nil
}

// applyClinePassRoutingIntent merges {only|order|sort} into a nested routing
// object (e.g. "providerOptions.gateway" or "provider"), preserving any other
// keys already present on the object.
func applyClinePassRoutingIntent(body []byte, basePath string, only []string, order []string, sort string) ([]byte, error) {
	updated := body
	var err error
	if len(only) > 0 {
		updated, err = sjson.SetBytes(updated, basePath+".only", only)
		if err != nil {
			return body, fmt.Errorf("clinepass pin: set %s.only: %w", basePath, err)
		}
	}
	if len(order) > 0 {
		updated, err = sjson.SetBytes(updated, basePath+".order", order)
		if err != nil {
			return body, fmt.Errorf("clinepass pin: set %s.order: %w", basePath, err)
		}
	}
	if sort != "" {
		updated, err = sjson.SetBytes(updated, basePath+".sort", sort)
		if err != nil {
			return body, fmt.Errorf("clinepass pin: set %s.sort: %w", basePath, err)
		}
	}
	return updated, nil
}

func clinePassPinDedupe(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	out := make([]string, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func filterOut(list []string, remove ...string) []string {
	if len(list) == 0 {
		return nil
	}
	rm := make(map[string]struct{}, len(remove))
	for _, r := range remove {
		r = strings.TrimSpace(r)
		if r != "" {
			rm[r] = struct{}{}
		}
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := rm[item]; ok {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
