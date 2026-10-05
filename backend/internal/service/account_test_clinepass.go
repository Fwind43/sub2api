package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ClinePassFallbackTestModel is a free (zero-credit) model on the ClinePass
// catalog, used when the caller did not pin a model. Paid models require a
// positive Cline Credits balance, so a connection test must not assume one.
const ClinePassFallbackTestModel = "apodex/apodex-1.1-mini:free"

// testClinePassAccountConnection probes the ClinePass (cline.bot) upstream with
// the account's own credential.
//
// It mirrors the gateway path (sendClinePassRequest): the OpenAI-compatible
// chat/completions endpoint is called with `Authorization: Bearer
// workos:<access token>`, the Cline relay identity headers, and a
// vendor-qualified model id (`<vendor>/<model>`). Clients send the bare
// subscription id (`deepseek-v4-pro`), so the `cline-pass/` prefix is restored
// before the call.
//
// Before this existed, ClinePass accounts fell through to
// testClaudeAccountConnection, which posts the Cline token to
// https://api.anthropic.com/v1/messages and therefore always answered
// `401 Invalid bearer token`.
func (s *AccountTestService) testClinePassAccountConnection(c *gin.Context, account *Account, modelID, prompt string) error {
	ctx := c.Request.Context()

	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = ClinePassFallbackTestModel
	}
	testModelID = account.GetMappedModel(testModelID)
	// 对外模型名是裸名（无 vendor 前缀），上游要求 "<vendor>/<model>"，回补后转发。
	testModelID = clinePassRestoreModelPrefix(testModelID)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	credential, credentialKind := account.ClinePassCredential()
	if strings.TrimSpace(credential) == "" {
		s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
		if credentialKind == "oauth" {
			return s.sendErrorAndEnd(c, "No ClinePass access token available; re-run the Cline login")
		}
		return s.sendErrorAndEnd(c, "No ClinePass API key available")
	}

	apiURL := account.ClinePassChatCompletionsURL()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	s.sendEvent(c, TestEvent{Type: "status", Text: "Testing via /api/v1/chat/completions"})

	payload := createOpenAIChatCompletionsTestPayload(testModelID, prompt)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create test payload")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create ClinePass test request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+credential)
	// Relay identity headers, identical to the live gateway path.
	applyClinePassHeaders(req.Header)
	// Account-level header overrides keep the highest priority.
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("ClinePass request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		preview := truncateString(string(body), 800)
		errMsg := fmt.Sprintf("API returned %d: %s", resp.StatusCode, preview)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			errMsg = fmt.Sprintf("ClinePass rejected the credential (401): %s", preview)
			if s.accountRepo != nil {
				_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
			}
		case http.StatusPaymentRequired:
			errMsg = fmt.Sprintf(
				"ClinePass model %q is not covered by the current balance (402). Pick a free model or top up Cline Credits: %s",
				testModelID, preview)
		case http.StatusBadRequest:
			errMsg = fmt.Sprintf("ClinePass rejected the request (400): %s", preview)
		}
		return s.sendErrorAndEnd(c, errMsg)
	}

	return s.processOpenAIChatCompletionsStream(c, resp.Body)
}
