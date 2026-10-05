package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clinepass"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// ClinePass (cline.bot) upstream constants.
//
// ClinePass exposes an OpenAI-compatible Chat Completions endpoint that only
// implements streaming generation: a non-stream request is answered with
// "generateText is not implemented". The gateway therefore always forces
// stream=true on the wire and aggregates the SSE stream itself when the client
// asked for a non-streaming response.
const (
	// DefaultClinePassBaseURL is the API root (already versioned).
	DefaultClinePassBaseURL = clinepass.DefaultBaseURL
	// clinePassRelayReferer / clinePassRelayTitle identify the relay to the
	// upstream. ClinePass rejects requests that do not look like a client call.
	clinePassRelayReferer = "https://cline.bot"
	clinePassRelayTitle   = "Cline"
)

// ClinePassBaseURL returns the API root for a ClinePass account, honoring an
// account-level base_url override (credentials.api_base_url / base_url).
func (a *Account) ClinePassBaseURL() string {
	if a == nil {
		return DefaultClinePassBaseURL
	}
	for _, key := range []string{"api_base_url", "base_url"} {
		if raw := strings.TrimSpace(a.GetCredential(key)); raw != "" {
			return strings.TrimRight(raw, "/")
		}
	}
	return DefaultClinePassBaseURL
}

// ClinePassCredential resolves the on-wire bearer credential for a ClinePass
// account. OAuth accounts use the WorkOS access token with the `workos:`
// prefix; API key accounts pass the key through unchanged.
func (a *Account) ClinePassCredential() (string, string) {
	if a == nil {
		return "", ""
	}
	if a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken {
		token := strings.TrimSpace(a.GetOpenAIAccessToken())
		if token == "" {
			token = strings.TrimSpace(a.GetCredential("access_token"))
		}
		if token == "" {
			return "", ""
		}
		return clinepass.NormalizeAccessToken(token), "oauth"
	}
	apiKey := strings.TrimSpace(a.GetCredential("api_key"))
	return apiKey, "apikey"
}

// GetClinePassAccessToken returns the stored OAuth access token for a ClinePass
// account. Credentials written by the OAuth flow use the generic OpenAI keys so
// existing token plumbing (cache invalidation, redaction) keeps working.
func (a *Account) GetClinePassAccessToken() string {
	if a == nil {
		return ""
	}
	if token := strings.TrimSpace(a.GetCredential("access_token")); token != "" {
		return token
	}
	return strings.TrimSpace(a.GetOpenAIAccessToken())
}

// GetClinePassRefreshToken returns the stored OAuth refresh token.
func (a *Account) GetClinePassRefreshToken() string {
	if a == nil {
		return ""
	}
	if token := strings.TrimSpace(a.GetCredential("refresh_token")); token != "" {
		return token
	}
	return strings.TrimSpace(a.GetOpenAIRefreshToken())
}

// ClinePassChatCompletionsURL builds the upstream chat completions endpoint.
func (a *Account) ClinePassChatCompletionsURL() string {
	base := a.ClinePassBaseURL()
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

// prepareClinePassChatBody forces streaming on the upstream request and keeps
// usage reporting enabled so the gateway can bill streamed responses.
func prepareClinePassChatBody(body []byte) ([]byte, error) {
	updated, err := sjson.SetBytes(body, "stream", true)
	if err != nil {
		return body, fmt.Errorf("clinepass: force stream: %w", err)
	}
	updated, err = sjson.SetBytes(updated, "stream_options.include_usage", true)
	if err != nil {
		return updated, fmt.Errorf("clinepass: enable usage: %w", err)
	}
	return updated, nil
}

// applyClinePassHeaders sets the relay identity headers required by the
// ClinePass API. Account-level header overrides are applied by the caller so
// they keep the highest priority.
func applyClinePassHeaders(header http.Header) {
	if header.Get("HTTP-Referer") == "" {
		header.Set("HTTP-Referer", clinePassRelayReferer)
	}
	if header.Get("X-Title") == "" {
		header.Set("X-Title", clinePassRelayTitle)
	}
	if header.Get("Accept") == "" {
		header.Set("Accept", "text/event-stream")
	}
	if header.Get("X-Requested-With") == "" {
		header.Set("X-Requested-With", "XMLHttpRequest")
	}
}

// clinePassModelPrefix is the vendor prefix Cline uses for the models covered
// by the subscription. The relay rejects ids that are not vendor-qualified
// ("invalid model format. Expected format: modelType/model"), so the prefix is
// stripped on the way out to clients and restored on the way in to the relay.
const clinePassModelPrefix = "cline-pass/"

// clinePassStripModelPrefix exposes a subscription-family model id without its
// vendor prefix: "cline-pass/deepseek-v4-pro" -> "deepseek-v4-pro". Ids from
// other families (anthropic/*, cline-free/*, ...) are returned unchanged.
func clinePassStripModelPrefix(model string) string {
	trimmed := strings.TrimSpace(model)
	if !strings.HasPrefix(trimmed, clinePassModelPrefix) {
		return trimmed
	}
	return strings.TrimPrefix(trimmed, clinePassModelPrefix)
}

// ClinePassStripModelPrefix exposes clinePassStripModelPrefix to layers that
// list models to clients (admin model list, catalog sync).
func ClinePassStripModelPrefix(model string) string {
	return clinePassStripModelPrefix(model)
}

// clinePassRestoreModelPrefix re-qualifies a bare model id for the relay.
// Already-qualified ids (any "<vendor>/<model>" shape, including ids sent by
// older clients) are returned unchanged.
func clinePassRestoreModelPrefix(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return trimmed
	}
	return clinePassModelPrefix + trimmed
}

// stripClinePassModelPrefixes applies clinePassStripModelPrefix to a list while
// preserving order and dropping collisions.
func stripClinePassModelPrefixes(models []string) []string {
	if len(models) == 0 {
		return models
	}
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		stripped := clinePassStripModelPrefix(model)
		if _, ok := seen[stripped]; ok {
			continue
		}
		seen[stripped] = struct{}{}
		out = append(out, stripped)
	}
	return out
}

// sendClinePassRequest posts a (streaming) Chat Completions request to the
// ClinePass upstream using the selected account's own credential and proxy.
func (s *OpenAIGatewayService) sendClinePassRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	_ bool,
) (*http.Response, error) {
	credential, _ := account.ClinePassCredential()
	if strings.TrimSpace(credential) == "" {
		return nil, fmt.Errorf("clinepass: credential not found in account %d", account.ID)
	}
	upstreamBody, err := prepareClinePassChatBody(body)
	if err != nil {
		return nil, err
	}
	// 上游钉住（cline-pass-switcher 移植）：把账号配置的路由意图注入出站
	// body。无配置时原样透传，失败时 fail-open 返回未注入的 body。
	upstreamBody = ApplyClinePassUpstreamPin(account, upstreamBody)
	targetURL := account.ClinePassChatCompletionsURL()

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("clinepass: build upstream request: %w", err)
	}
	SetActualOpenAIUpstreamEndpoint(c, "/api/v1/chat/completions")
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+credential)
	upstreamReq.Header.Set("Accept", "text/event-stream")
	applyClinePassHeaders(upstreamReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	account.ApplyHeaderOverrides(upstreamReq.Header)

	resp, err := s.doOpenAIUpstream(upstreamReq, proxyURL, account)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	return resp, nil
}
