package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/clinepass"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

// clinePassDefaultAccessTokenTTL mirrors the 1h WorkOS access token lifetime
// when the upstream omits an explicit expiry.
const clinePassDefaultAccessTokenTTL = time.Hour

// ClinePassOAuthService drives the ClinePass (cline.bot) device-code login and
// token lifecycle. The flow is a public-client RFC 8628 device grant against
// WorkOS, followed by an exchange at the Cline API that returns the long-lived
// Cline credential pair used for inference.
type ClinePassOAuthService struct {
	proxyRepo ProxyRepository
	config    *config.Config
}

func NewClinePassOAuthService(proxyRepo ProxyRepository, configs ...*config.Config) *ClinePassOAuthService {
	svc := &ClinePassOAuthService{proxyRepo: proxyRepo}
	if len(configs) > 0 {
		svc.config = configs[0]
	}
	return svc
}

// ClinePassCapabilities reports which login affordances the UI may offer.
type ClinePassCapabilities struct {
	DeviceFlowEnabled bool `json:"device_flow_enabled"`
}

func (s *ClinePassOAuthService) GetCapabilities() ClinePassCapabilities {
	return ClinePassCapabilities{DeviceFlowEnabled: true}
}

// ClinePassDeviceAuth is the challenge handed to the operator, plus the opaque
// device code the server keeps for polling.
type ClinePassDeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// ClinePassTokenInfo is the normalized Cline credential set.
type ClinePassTokenInfo struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresAt    int64
	Email        string
	UserID       string
}

func (s *ClinePassOAuthService) client(ctx context.Context, proxyID *int64) (*clinepass.Client, error) {
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	client := &clinepass.Client{HTTPClient: &http.Client{Timeout: 60 * time.Second}}
	if strings.TrimSpace(proxyURL) != "" {
		parsed, parseErr := url.Parse(proxyURL)
		if parseErr != nil {
			return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_PROXY_INVALID", "configured proxy URL is invalid")
		}
		transport := &http.Transport{}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_PROXY_UNSUPPORTED", "configured proxy is not supported")
		}
		client.HTTPClient = &http.Client{Timeout: 60 * time.Second, Transport: transport}
	}
	return client, nil
}

// StartDeviceAuth creates a fresh device-authorization challenge.
func (s *ClinePassOAuthService) StartDeviceAuth(ctx context.Context, proxyID *int64) (*ClinePassDeviceAuth, error) {
	client, err := s.client(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	auth, err := client.StartDeviceAuth(ctx)
	if err != nil {
		return nil, wrapClinePassOAuthError(err, "CLINEPASS_OAUTH_DEVICE_START_FAILED")
	}
	return &ClinePassDeviceAuth{
		DeviceCode:              auth.DeviceCode,
		UserCode:                auth.UserCode,
		VerificationURI:         auth.VerificationURI,
		VerificationURIComplete: auth.VerificationURIComplete,
		ExpiresIn:               auth.ExpiresIn,
		Interval:                auth.Interval,
	}, nil
}

// ClinePassDeviceAuthPending is returned while the operator has not approved.
var ClinePassDeviceAuthPending = errors.New("clinepass: device authorization pending")

// PollDeviceAuth performs exactly one poll. Callers drive the retry cadence and
// must surface ClinePassDeviceAuthPending as a non-fatal "keep waiting" state.
func (s *ClinePassOAuthService) PollDeviceAuth(ctx context.Context, deviceCode string, proxyID *int64) (*ClinePassTokenInfo, error) {
	if strings.TrimSpace(deviceCode) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_DEVICE_CODE_REQUIRED", "device code is required")
	}
	client, err := s.client(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	tokens, err := client.PollDeviceAuth(ctx, deviceCode)
	if err != nil {
		if errors.Is(err, clinepass.ErrAuthorizationPending) || errors.Is(err, clinepass.ErrSlowDown) {
			return nil, ClinePassDeviceAuthPending
		}
		return nil, wrapClinePassOAuthError(err, "CLINEPASS_OAUTH_DEVICE_POLL_FAILED")
	}
	return s.exchangeWorkOSTokens(ctx, client, tokens.AccessToken, tokens.RefreshToken)
}

// exchangeWorkOSTokens trades WorkOS tokens for the Cline inference credentials.
func (s *ClinePassOAuthService) exchangeWorkOSTokens(ctx context.Context, client *clinepass.Client, accessToken, refreshToken string) (*ClinePassTokenInfo, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "CLINEPASS_OAUTH_NO_WORKOS_REFRESH_TOKEN", "workos token response missing refresh_token")
	}
	registered, err := client.Register(ctx, accessToken, refreshToken)
	if err != nil {
		return nil, wrapClinePassOAuthError(err, "CLINEPASS_OAUTH_REGISTER_FAILED")
	}
	if registered == nil || strings.TrimSpace(registered.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "CLINEPASS_OAUTH_INVALID_TOKEN_RESPONSE", "cline auth response missing accessToken")
	}
	info := &ClinePassTokenInfo{
		AccessToken:  registered.AccessToken,
		RefreshToken: registered.RefreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    parseClinePassExpiry(registered.ExpiresAt),
	}
	if registered.UserInfo != nil {
		info.Email = registered.UserInfo.Email
		info.UserID = registered.UserInfo.ID
	}
	return info, nil
}

// RefreshToken exchanges a Cline refresh token for a new credential pair.
func (s *ClinePassOAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyID *int64) (*ClinePassTokenInfo, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}
	client, err := s.client(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	refreshed, err := client.Refresh(ctx, refreshToken)
	if err != nil {
		return nil, wrapClinePassOAuthError(err, "CLINEPASS_OAUTH_REFRESH_FAILED")
	}
	if refreshed == nil || strings.TrimSpace(refreshed.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "CLINEPASS_OAUTH_INVALID_TOKEN_RESPONSE", "cline refresh response missing accessToken")
	}
	info := &ClinePassTokenInfo{
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    parseClinePassExpiry(refreshed.ExpiresAt),
	}
	if info.RefreshToken == "" {
		info.RefreshToken = refreshToken
	}
	if refreshed.UserInfo != nil {
		info.Email = refreshed.UserInfo.Email
		info.UserID = refreshed.UserInfo.ID
	}
	return info, nil
}

// RefreshAccountToken refreshes an existing account in place.
func (s *ClinePassOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*ClinePassTokenInfo, error) {
	if account == nil || account.Platform != PlatformClinePass {
		return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_INVALID_ACCOUNT", "account is not a ClinePass account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}
	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}
	info, err := s.RefreshToken(ctx, refreshToken, account.ProxyID)
	if err != nil {
		return nil, err
	}
	if info.Email == "" {
		info.Email = strings.TrimSpace(account.GetCredential("email"))
	}
	if info.UserID == "" {
		info.UserID = strings.TrimSpace(account.GetCredential("cline_user_id"))
	}
	return info, nil
}

// BuildAccountCredentials renders the persisted credential document.
func (s *ClinePassOAuthService) BuildAccountCredentials(info *ClinePassTokenInfo) map[string]any {
	if info == nil {
		return nil
	}
	expiresAt := info.ExpiresAt
	if expiresAt <= 0 {
		expiresAt = time.Now().Add(clinePassDefaultAccessTokenTTL).Unix()
	}
	creds := map[string]any{
		"access_token": info.AccessToken,
		"expires_at":   time.Unix(expiresAt, 0).UTC().Format(time.RFC3339),
	}
	if info.RefreshToken != "" {
		creds["refresh_token"] = info.RefreshToken
	}
	if info.TokenType != "" {
		creds["token_type"] = info.TokenType
	}
	if info.Email != "" {
		creds["email"] = info.Email
	}
	if info.UserID != "" {
		creds["cline_user_id"] = info.UserID
	}
	creds["base_url"] = DefaultClinePassBaseURL
	return creds
}

// FetchProfile validates a credential against the upstream user endpoint.
func (s *ClinePassOAuthService) FetchProfile(ctx context.Context, accessToken string, proxyID *int64) (*clinepass.UserProfile, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_NO_ACCESS_TOKEN", "no access token available")
	}
	client, err := s.client(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	profile, err := client.FetchProfile(ctx, accessToken)
	if err != nil {
		return nil, wrapClinePassOAuthError(err, "CLINEPASS_OAUTH_PROFILE_FAILED")
	}
	return profile, nil
}

// FetchRecommendedModels proxies the Cline model catalog.
func (s *ClinePassOAuthService) FetchRecommendedModels(ctx context.Context, accessToken string, proxyID *int64) ([]clinepass.Model, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_NO_ACCESS_TOKEN", "no access token available")
	}
	client, err := s.client(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	models, err := client.FetchRecommendedModels(ctx, accessToken)
	if err != nil {
		return nil, wrapClinePassOAuthError(err, "CLINEPASS_OAUTH_MODELS_FAILED")
	}
	return models, nil
}

func (s *ClinePassOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		if errors.Is(err, ErrProxyNotFound) {
			return "", infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_PROXY_NOT_FOUND", "configured proxy was not found")
		}
		return "", infraerrors.New(http.StatusServiceUnavailable, "CLINEPASS_OAUTH_PROXY_LOOKUP_FAILED", "proxy lookup is temporarily unavailable")
	}
	if proxy == nil {
		return "", infraerrors.New(http.StatusBadRequest, "CLINEPASS_OAUTH_PROXY_NOT_FOUND", "configured proxy was not found")
	}
	return proxy.URL(), nil
}

func parseClinePassExpiry(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().Add(clinePassDefaultAccessTokenTTL).Unix()
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts.Unix()
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts.Unix()
	}
	return time.Now().Add(clinePassDefaultAccessTokenTTL).Unix()
}

// wrapClinePassOAuthError maps upstream failures onto API errors without
// leaking credentials.
func wrapClinePassOAuthError(err error, code string) error {
	if err == nil {
		return nil
	}
	var apiErr *clinepass.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		message := strings.TrimSpace(apiErr.Message)
		if message == "" {
			message = strings.TrimSpace(apiErr.Code)
		}
		if message == "" {
			message = "clinepass upstream request failed"
		}
		return infraerrors.New(status, code, message)
	}
	return infraerrors.New(http.StatusBadGateway, code, "clinepass upstream request failed")
}
