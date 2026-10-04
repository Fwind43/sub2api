// Package clinepass implements the ClinePass (cline.bot) OAuth device flow and
// token exchange used to obtain WorkOS-backed Cline credentials.
//
// The flow mirrors the official Cline CLI/extension login:
//
//  1. POST https://api.workos.com/user_management/authorize/device
//     -> device_code / user_code / verification_uri / interval / expires_in
//  2. user opens verification_uri and approves
//  3. POST https://api.workos.com/user_management/authenticate
//     grant_type=urn:ietf:params:oauth:grant-type:device_code
//     -> access_token (WorkOS) + refresh_token (WorkOS)
//  4. POST https://api.cline.bot/api/v1/auth/register
//     {accessToken, refreshToken}
//     -> {success, data:{accessToken, refreshToken, expiresAt, userInfo}}
//
// The Cline API expects `Authorization: Bearer workos:<accessToken>`; the
// "workos:" prefix is added on the wire and never stored in credentials.
package clinepass

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the Cline API base (OpenAI-compatible gateway).
	DefaultBaseURL = "https://api.cline.bot/api/v1"
	// DefaultWorkOSBaseURL is the WorkOS user-management API base.
	DefaultWorkOSBaseURL = "https://api.workos.com"

	// ClientID is the public WorkOS client id used by the Cline extension.
	// It is not a secret: device flow is a public-client flow.
	ClientID = "client_01K3A541FN8TA3EPPHTD2325AR"

	// DeviceGrantType is the RFC 8628 device-code grant.
	DeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	// WorkOSPrefix is required on upstream Authorization headers.
	WorkOSPrefix = "workos:"

	// Referer / Title match the official client and are enforced by the
	// upstream edge on some endpoints.
	Referer = "https://cline.bot"
	Title   = "Cline"
)

// HTTPDoer is the minimal HTTP contract (satisfied by *http.Client).
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client talks to WorkOS + the Cline auth API.
type Client struct {
	BaseURL       string
	WorkOSBaseURL string
	ClientID      string
	HTTPClient    HTTPDoer
	UserAgent     string
}

func (c *Client) httpClient() HTTPDoer {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) baseURL() string {
	if c != nil && strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	}
	return DefaultBaseURL
}

func (c *Client) workosBaseURL() string {
	if c != nil && strings.TrimSpace(c.WorkOSBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(c.WorkOSBaseURL), "/")
	}
	return DefaultWorkOSBaseURL
}

func (c *Client) clientID() string {
	if c != nil && strings.TrimSpace(c.ClientID) != "" {
		return strings.TrimSpace(c.ClientID)
	}
	return ClientID
}

func (c *Client) userAgent() string {
	if c != nil && strings.TrimSpace(c.UserAgent) != "" {
		return strings.TrimSpace(c.UserAgent)
	}
	return "sub2api-clinepass/1.0"
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrAuthorizationPending is returned while the user has not approved yet.
var ErrAuthorizationPending = fmt.Errorf("clinepass: authorization_pending")

// ErrSlowDown asks the caller to increase its polling interval.
var ErrSlowDown = fmt.Errorf("clinepass: slow_down")

// APIError is a structured upstream failure.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Body       string
}

func (e *APIError) Error() string {
	if e == nil {
		return "clinepass: api error"
	}
	code := strings.TrimSpace(e.Code)
	if code == "" {
		code = http.StatusText(e.StatusCode)
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = strings.TrimSpace(e.Body)
	}
	if msg == "" {
		return fmt.Sprintf("clinepass: upstream %d (%s)", e.StatusCode, code)
	}
	return fmt.Sprintf("clinepass: upstream %d (%s): %s", e.StatusCode, code, msg)
}

// ---------------------------------------------------------------------------
// Device flow
// ---------------------------------------------------------------------------

// DeviceAuth is the pending device-authorization challenge.
type DeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// StartDeviceAuth creates a new device-authorization challenge.
func (c *Client) StartDeviceAuth(ctx context.Context) (*DeviceAuth, error) {
	payload := map[string]any{"client_id": c.clientID()}
	var out DeviceAuth
	if err := c.doJSON(ctx, http.MethodPost, c.workosBaseURL()+"/user_management/authorize/device", payload, "", &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.DeviceCode) == "" {
		return nil, fmt.Errorf("clinepass: device authorization returned no device_code")
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 300
	}
	return &out, nil
}

// DeviceTokens is the result of a successful WorkOS token exchange.
type DeviceTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// PollDeviceAuth performs one device-code exchange attempt.
//
// It returns ErrAuthorizationPending / ErrSlowDown for the two retryable
// states so callers can drive the polling loop themselves.
func (c *Client) PollDeviceAuth(ctx context.Context, deviceCode string) (*DeviceTokens, error) {
	payload := map[string]any{
		"grant_type":  DeviceGrantType,
		"client_id":   c.clientID(),
		"device_code": strings.TrimSpace(deviceCode),
	}
	return c.exchange(ctx, payload)
}

// RefreshWorkOSToken exchanges a WorkOS refresh token for new WorkOS tokens.
func (c *Client) RefreshWorkOSToken(ctx context.Context, refreshToken string) (*DeviceTokens, error) {
	payload := map[string]any{
		"grant_type":    "refresh_token",
		"client_id":     c.clientID(),
		"refresh_token": strings.TrimSpace(refreshToken),
	}
	return c.exchange(ctx, payload)
}

func (c *Client) exchange(ctx context.Context, payload map[string]any) (*DeviceTokens, error) {
	raw, status, err := c.doRawJSON(ctx, http.MethodPost, c.workosBaseURL()+"/user_management/authenticate", payload, "")
	if err != nil {
		return nil, err
	}
	var out DeviceTokens
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("clinepass: decode authenticate response: %w", err)
	}
	if status >= 400 || strings.TrimSpace(out.AccessToken) == "" {
		code, msg := parseOAuthError(raw)
		switch code {
		case "authorization_pending":
			return nil, ErrAuthorizationPending
		case "slow_down":
			return nil, ErrSlowDown
		case "expired_token":
			return nil, &APIError{StatusCode: status, Code: code, Message: "device code expired, restart login"}
		case "access_denied":
			return nil, &APIError{StatusCode: status, Code: code, Message: "authorization denied by user"}
		}
		return nil, &APIError{StatusCode: status, Code: code, Message: msg, Body: string(raw)}
	}
	return &out, nil
}

func parseOAuthError(raw []byte) (code, message string) {
	var envelope struct {
		Error            any    `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", ""
	}
	switch v := envelope.Error.(type) {
	case string:
		code = v
	case map[string]any:
		code, _ = v["code"].(string)
		message, _ = v["message"].(string)
	}
	if message == "" {
		message = envelope.ErrorDescription
	}
	return code, message
}

// ---------------------------------------------------------------------------
// Cline auth API
// ---------------------------------------------------------------------------

// Credentials is the gateway-side credential payload persisted on an account.
type Credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	Email        string `json:"email,omitempty"`
	UserID       string `json:"cline_user_id,omitempty"`
}

// UserInfo mirrors data.userInfo from /auth/register.
type UserInfo struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// RegisterResponse is the payload inside data of /auth/register.
type RegisterResponse struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    string    `json:"expiresAt"`
	UserInfo     *UserInfo `json:"userInfo"`
}

// Register exchanges WorkOS tokens for Cline API tokens.
//
// Both tokens are mandatory upstream: omitting refreshToken yields HTTP 400
// {"error":"refreshtoken required"}.
func (c *Client) Register(ctx context.Context, accessToken, refreshToken string) (*RegisterResponse, error) {
	accessToken = strings.TrimSpace(accessToken)
	refreshToken = strings.TrimSpace(refreshToken)
	if accessToken == "" || refreshToken == "" {
		return nil, fmt.Errorf("clinepass: register requires both accessToken and refreshToken")
	}
	payload := map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
	}
	raw, status, err := c.doRawJSON(ctx, http.MethodPost, c.baseURL()+"/auth/register", payload, "")
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Success bool              `json:"success"`
		Data    *RegisterResponse `json:"data"`
		Error   json.RawMessage   `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("clinepass: decode register response: %w", err)
	}
	if status >= 400 || envelope.Data == nil || strings.TrimSpace(envelope.Data.AccessToken) == "" {
		return nil, &APIError{
			StatusCode: status,
			Code:       "register_failed",
			Message:    strings.TrimSpace(string(envelope.Error)),
			Body:       string(raw),
		}
	}
	return envelope.Data, nil
}

// Refresh exchanges a Cline refresh token for a fresh Cline access token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*RegisterResponse, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("clinepass: refresh requires refreshToken")
	}
	payload := map[string]any{
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
	}
	raw, status, err := c.doRawJSON(ctx, http.MethodPost, c.baseURL()+"/auth/refresh", payload, "")
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Success bool              `json:"success"`
		Data    *RegisterResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("clinepass: decode refresh response: %w", err)
	}
	if status >= 400 || envelope.Data == nil || strings.TrimSpace(envelope.Data.AccessToken) == "" {
		return nil, &APIError{StatusCode: status, Code: "refresh_failed", Body: string(raw)}
	}
	return envelope.Data, nil
}

// UserProfile is GET /users/me.
type UserProfile struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// FetchProfile validates a Cline access token and returns the account identity.
func (c *Client) FetchProfile(ctx context.Context, accessToken string) (*UserProfile, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("clinepass: missing access token")
	}
	raw, status, err := c.doRawJSON(ctx, http.MethodGet, c.baseURL()+"/users/me", nil, accessToken)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, &APIError{StatusCode: status, Code: "profile_failed", Body: string(raw)}
	}
	var profile UserProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return nil, fmt.Errorf("clinepass: decode profile: %w", err)
	}
	return &profile, nil
}

// ---------------------------------------------------------------------------
// Recommended models
// ---------------------------------------------------------------------------

// Model is one entry of the Cline recommended-model catalog.
type Model struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Description     string `json:"description"`
	ContextWindow   int    `json:"contextWindow"`
	MaxOutputTokens int    `json:"maxTokens"`
}

// FetchRecommendedModels reads the Cline model catalog.
//
// The response shape has drifted across releases, so several spellings of the
// grouping keys are accepted. When provider information is only present in the
// model id (e.g. "anthropic/claude-opus-5.5") it is derived from the id.
func (c *Client) FetchRecommendedModels(ctx context.Context, accessToken string) ([]Model, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("clinepass: missing access token")
	}
	raw, status, err := c.doRawJSON(ctx, http.MethodGet, c.baseURL()+"/ai/cline/recommended-models", nil, accessToken)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, &APIError{StatusCode: status, Code: "models_failed", Body: string(raw)}
	}

	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("clinepass: decode models: %w", err)
	}
	models := collectModels(probe)
	seen := map[string]bool{}
	out := make([]Model, 0, len(models))
	for _, m := range models {
		m.ID = strings.TrimSpace(m.ID)
		if m.ID == "" || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		if strings.TrimSpace(m.Name) == "" {
			m.Name = m.ID
		}
		if m.Provider == "" {
			m.Provider = ProviderFromModelID(m.ID)
		}
		out = append(out, m)
	}
	return out, nil
}

// ProviderFromModelID returns the provider prefix of a Cline model id.
func ProviderFromModelID(id string) string {
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	if i := strings.Index(id, ":"); i > 0 {
		return id[:i]
	}
	return ""
}

func collectModels(node any) []Model {
	var out []Model
	switch v := node.(type) {
	case []any:
		for _, item := range v {
			out = append(out, collectModels(item)...)
		}
	case map[string]any:
		if id, ok := v["id"].(string); ok && strings.TrimSpace(id) != "" {
			m := Model{ID: id}
			m.Name, _ = v["name"].(string)
			m.Provider, _ = v["provider"].(string)
			m.Description, _ = v["description"].(string)
			m.ContextWindow = firstInt(v, "contextWindow", "context_window", "maxContextTokens")
			m.MaxOutputTokens = firstInt(v, "maxTokens", "max_tokens", "maxOutputTokens")
			out = append(out, m)
			return out
		}
		if groups, ok := v["providers"].(map[string]any); ok {
			for provider, item := range groups {
				derived := collectModels(item)
				for i := range derived {
					if derived[i].Provider == "" {
						derived[i].Provider = provider
					}
				}
				out = append(out, derived...)
			}
		}
		for key, item := range v {
			switch key {
			case "data", "models", "recommendedModels", "recommended_models":
				out = append(out, collectModels(item)...)
			}
		}
	}
	return out
}

func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (c *Client) doJSON(ctx context.Context, method, target string, payload any, accessToken string, out any) error {
	raw, status, err := c.doRawJSON(ctx, method, target, payload, accessToken)
	if err != nil {
		return err
	}
	if status >= 400 {
		code, msg := parseOAuthError(raw)
		return &APIError{StatusCode: status, Code: code, Message: msg, Body: string(raw)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("clinepass: decode response from %s: %w", target, err)
	}
	return nil
}

func (c *Client) doRawJSON(ctx context.Context, method, target string, payload any, accessToken string) ([]byte, int, error) {
	var bodyReader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("clinepass: encode request: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("clinepass: build request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent())
	req.Header.Set("HTTP-Referer", Referer)
	req.Header.Set("X-Title", Title)
	if strings.TrimSpace(accessToken) != "" {
		req.Header.Set("Authorization", "Bearer "+NormalizeAccessToken(accessToken))
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("clinepass: request %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("clinepass: read response from %s: %w", target, err)
	}
	return raw, resp.StatusCode, nil
}

// NormalizeAccessToken adds the mandatory "workos:" prefix exactly once.
func NormalizeAccessToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, WorkOSPrefix) {
		return token
	}
	return WorkOSPrefix + token
}

// ProviderRequiredError reports a model id that carries no provider prefix; the
// Cline gateway rejects such ids with "Model/provider not recognized".
func ProviderRequiredError(model string) error {
	return fmt.Errorf("clinepass: model %q is not provider-qualified (expected e.g. \"anthropic/claude-opus-5.5\")", strings.TrimSpace(model))
}

// ResolveURL joins base and path safely.
func ResolveURL(base, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	path = strings.TrimSpace(path)
	if path == "" {
		return base
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

// EncodeQuery builds a URL with query parameters.
func EncodeQuery(base string, params map[string]string) string {
	if len(params) == 0 {
		return base
	}
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + values.Encode()
}

// BackoffDelay returns the polling delay mandated by the device flow.
func BackoffDelay(intervalSeconds int, slowdown bool) time.Duration {
	if intervalSeconds <= 0 {
		intervalSeconds = 5
	}
	if slowdown {
		intervalSeconds += 5
	}
	return time.Duration(intervalSeconds) * time.Second
}
