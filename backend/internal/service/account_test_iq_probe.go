package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
)

// RunQuestionBackground sends a single custom prompt through the account's
// native test path (no real HTTP client on the inbound side) and returns the
// concatenated assistant text plus the wall-clock latency in milliseconds.
//
// It reuses TestAccountConnection, so it inherits per-platform routing,
// credential handling and SSE parsing; the prompt and reasoning effort are
// threaded through every payload builder (Claude / OpenAI Responses / Chat
// Completions / Gemini).
func (s *AccountTestService) RunQuestionBackground(ctx context.Context, accountID int64, modelID string, prompt string, reasoningEffort string) (string, int64, error) {
	startedAt := time.Now()

	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = (&http.Request{}).WithContext(ctx)

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID, prompt, AccountTestModeDefault, AccountTestOptions{ReasoningEffort: reasoningEffort})

	latencyMs := time.Since(startedAt).Milliseconds()
	responseText, errMsg := parseTestSSEOutput(w.Body.String())

	if testErr != nil {
		if errMsg == "" {
			errMsg = testErr.Error()
		}
		return responseText, latencyMs, errors.New(errMsg)
	}
	if errMsg != "" {
		return responseText, latencyMs, errors.New(errMsg)
	}
	return responseText, latencyMs, nil
}
