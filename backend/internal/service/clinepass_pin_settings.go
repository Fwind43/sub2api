package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GetClinePassUpstreamPin returns the platform-level upstream pin shared by all
// clinepass accounts. A missing/empty setting yields (nil, nil) meaning "no pin".
func (s *SettingService) GetClinePassUpstreamPin(ctx context.Context) (*ClinePassUpstreamPinConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyClinePassUpstreamPinSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get clinepass upstream pin: %w", err)
	}
	cfg, err := ParseClinePassUpstreamPinJSON(value)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// SetClinePassUpstreamPin validates and persists the platform-level pin. Passing
// a nil/empty/"mode=off" configuration clears the setting.
func (s *SettingService) SetClinePassUpstreamPin(ctx context.Context, cfg *ClinePassUpstreamPinConfig) (*ClinePassUpstreamPinConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("setting repository is unavailable")
	}
	if err := validateClinePassUpstreamPin(cfg); err != nil {
		return nil, err
	}
	normalized := cfg.normalize()
	if normalized == nil {
		if err := s.settingRepo.Set(ctx, SettingKeyClinePassUpstreamPinSettings, ""); err != nil {
			return nil, fmt.Errorf("clear clinepass upstream pin: %w", err)
		}
		return nil, nil
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode clinepass upstream pin: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyClinePassUpstreamPinSettings, string(payload)); err != nil {
		return nil, fmt.Errorf("save clinepass upstream pin: %w", err)
	}
	return normalized, nil
}

func validateClinePassUpstreamPin(cfg *ClinePassUpstreamPinConfig) error {
	if cfg == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", "strict", "preferred", "off":
	default:
		return infraerrors.BadRequest("CLINEPASS_PIN_INVALID_MODE", "mode must be one of strict, preferred, off")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Pipelines)) {
	case "", "both", "vercel", "openrouter":
	default:
		return infraerrors.BadRequest("CLINEPASS_PIN_INVALID_PIPELINES", "pipelines must be one of both, vercel, openrouter")
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Mode), "off") {
		return nil
	}
	if strings.TrimSpace(cfg.Upstream) == "" && len(cfg.Only) == 0 && len(cfg.Exclude) == 0 {
		return infraerrors.BadRequest("CLINEPASS_PIN_UPSTREAM_REQUIRED", "upstream is required unless mode is off")
	}
	return nil
}

// clinePassPlatformUpstreamPin loads the platform-level pin for the outbound
// clinepass request path. Fail-open: any error yields nil (no platform pin).
func (s *OpenAIGatewayService) clinePassPlatformUpstreamPin(ctx context.Context) *ClinePassUpstreamPinConfig {
	if s == nil || s.settingService == nil {
		return nil
	}
	cfg, err := s.settingService.GetClinePassUpstreamPin(ctx)
	if err != nil {
		slog.Warn("clinepass_platform_upstream_pin_load_failed", "error", err)
		return nil
	}
	return cfg
}
