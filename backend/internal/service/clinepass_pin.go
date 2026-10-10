package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ClinePassUpstreamPinExtraKey is the account.Extra JSONB key that carries the
// upstream pin configuration for clinepass accounts. The value mirrors the
// cline-pass-switcher injectPrefs semantics: it is injected into the outbound
// Chat Completions body so the relay routes the request to a fixed upstream
// provider (Vercel AI Gateway pipeline or the OpenRouter direct pipeline).
//
// Example value (account.Extra["clinepass_upstream_pin"]):
//
//	{
//	  "mode": "strict",              // strict | preferred | off
//	  "upstream": "morph",           // pinned provider slug
//	  "order": ["morph", "..."],     // fallback order (preferred mode)
//	  "only": ["morph"],             // explicit allow-list (overrides exclude)
//	  "exclude": ["openai"],         // expanded to allow-list via "upstreams"
//	  "upstreams": ["a","b","c"],    // known provider slugs (probe result)
//	  "sort": "cost",                // cost | ttft | tps
//	  "pipelines": "both",           // both | vercel | openrouter
//	  "match_prefixes": ["deepseek"],// only pin models whose bare id starts so
//	  "models": {                    // per-model overrides (bare or prefixed id)
//	    "deepseek-v4-pro": { "upstream": "morph" }
//	  }
//	}
const ClinePassUpstreamPinExtraKey = "clinepass_upstream_pin"

// ClinePassUpstreamPinConfig mirrors the pin configuration. Zero values mean
// "not set"; model-level overrides replace top-level fields when non-zero.
type ClinePassUpstreamPinConfig struct {
	Mode      string   `json:"mode"`
	Upstream  string   `json:"upstream"`
	Order     []string `json:"order"`
	Only      []string `json:"only"`
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

// ParseClinePassUpstreamPin extracts the pin configuration from account.Extra.
// It accepts both an object value (JSONB) and a JSON string. Returns nil when
// no configuration is present or the configuration is unusable.
func ParseClinePassUpstreamPin(extra map[string]any) *ClinePassUpstreamPinConfig {
	if len(extra) == 0 {
		return nil
	}
	raw, ok := extra[ClinePassUpstreamPinExtraKey]
	if !ok || raw == nil {
		return nil
	}
	cfg, err := decodeClinePassUpstreamPin(raw)
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.normalize()
}

func decodeClinePassUpstreamPin(raw any) (*ClinePassUpstreamPinConfig, error) {
	var data []byte
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		data = []byte(v)
	case []byte:
		if len(v) == 0 {
			return nil, nil
		}
		data = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("clinepass pin: marshal config: %w", err)
		}
		data = b
	}
	var cfg ClinePassUpstreamPinConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("clinepass pin: invalid config: %w", err)
	}
	return &cfg, nil
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

// ClinePassUpstreamPinDisabled reports whether the account explicitly turns the
// pin off ("mode":"off"). Such accounts keep the pre-platform-level behaviour
// even when a platform-level pin is configured.
func ClinePassUpstreamPinDisabled(extra map[string]any) bool {
	if len(extra) == 0 {
		return false
	}
	raw, ok := extra[ClinePassUpstreamPinExtraKey]
	if !ok || raw == nil {
		return false
	}
	cfg, err := decodeClinePassUpstreamPin(raw)
	if err != nil || cfg == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cfg.Mode), "off")
}

// MergeClinePassUpstreamPin layers an account-level pin over the platform-level
// default: non-zero account fields win, platform fields fill the gaps.
func MergeClinePassUpstreamPin(platform, account *ClinePassUpstreamPinConfig) *ClinePassUpstreamPinConfig {
	if platform == nil {
		return account
	}
	if account == nil {
		return platform
	}
	merged := *platform
	if account.Mode != "" {
		merged.Mode = account.Mode
	}
	if account.Upstream != "" {
		merged.Upstream = account.Upstream
	}
	if len(account.Order) > 0 {
		merged.Order = account.Order
	}
	if len(account.Only) > 0 {
		merged.Only = account.Only
	}
	if len(account.Exclude) > 0 {
		merged.Exclude = account.Exclude
	}
	if len(account.Upstreams) > 0 {
		merged.Upstreams = account.Upstreams
	}
	if account.Sort != "" {
		merged.Sort = account.Sort
	}
	if account.Pipelines != "" {
		merged.Pipelines = account.Pipelines
	}
	if len(account.MatchPrefixes) > 0 {
		merged.MatchPrefixes = account.MatchPrefixes
	}
	if len(account.Models) > 0 {
		models := make(map[string]ClinePassUpstreamPinConfig, len(platform.Models)+len(account.Models))
		for key, value := range platform.Models {
			models[key] = value
		}
		for key, value := range account.Models {
			models[key] = value
		}
		merged.Models = models
	}
	return &merged
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

// effectiveForModel merges a per-model override over the top-level fields.
// Model keys may be written bare ("deepseek-v4-pro") or vendor-qualified
// ("cline-pass/deepseek-v4-pro"); both forms match the outbound model id.
func (c *ClinePassUpstreamPinConfig) effectiveForModel(model string) *ClinePassUpstreamPinConfig {
	eff := *c
	eff.Models = nil
	if len(c.Models) == 0 {
		return &eff
	}
	stripped := clinePassStripModelPrefix(model)
	var override *ClinePassUpstreamPinConfig
	for key := range c.Models {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if k == model {
			m := c.Models[key]
			override = &m
			break
		}
	}
	if override == nil {
		for key := range c.Models {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			if clinePassStripModelPrefix(k) == stripped {
				m := c.Models[key]
				override = &m
				break
			}
		}
	}
	if override == nil {
		return &eff
	}
	if override.Mode != "" {
		eff.Mode = override.Mode
	}
	if override.Upstream != "" {
		eff.Upstream = override.Upstream
	}
	if len(override.Order) > 0 {
		eff.Order = override.Order
	}
	if len(override.Only) > 0 {
		eff.Only = override.Only
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
		eff.Pipelines = override.Pipelines
	}
	return &eff
}

// ApplyClinePassUpstreamPin injects the account's pin configuration into the
// outbound body. It is fail-open: when no configuration exists, or injection
// fails, the original body is returned unchanged.
func ApplyClinePassUpstreamPin(account *Account, body []byte) []byte {
	return ApplyClinePassUpstreamPinWithPlatform(account, nil, body)
}

// ApplyClinePassUpstreamPinWithPlatform injects the effective pin configuration
// (account-level overriding the platform-level default) into the outbound body.
// It is fail-open: when no configuration exists, or injection fails, the
// original body is returned unchanged.
func ApplyClinePassUpstreamPinWithPlatform(account *Account, platform *ClinePassUpstreamPinConfig, body []byte) []byte {
	if account == nil || len(body) == 0 {
		return body
	}
	if ClinePassUpstreamPinDisabled(account.Extra) {
		return body
	}
	cfg := MergeClinePassUpstreamPin(platform, ParseClinePassUpstreamPin(account.Extra))
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

	useVercel := eff.Pipelines == "" || eff.Pipelines == "vercel"
	useOpenRouter := eff.Pipelines == "" || eff.Pipelines == "openrouter"
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
