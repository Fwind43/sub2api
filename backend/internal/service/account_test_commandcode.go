package service

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/commandcode"
	"github.com/gin-gonic/gin"
)

// testCommandCodeAccountConnection uses the same native GO protocol as the gateway.
func (s *AccountTestService) testCommandCodeAccountConnection(c *gin.Context, account *Account, modelID, prompt string) error {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return s.sendErrorAndEnd(c, "Select a CommandCode model before testing")
	}
	modelID = account.GetMappedModel(modelID)
	key := strings.TrimSpace(account.GetCredential("api_key"))
	if key == "" {
		return s.sendErrorAndEnd(c, "No CommandCode API key available")
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "Reply with OK."
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client := commandcode.Client{
		BaseURL:   "https://api.commandcode.ai",
		UserAgent: "command-code-cli/1.54.1", Version: "1.54.1",
		HTTPClient: commandCodeHTTPDoer(func(req *http.Request) (*http.Response, error) {
			req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
			account.ApplyHeaderOverrides(req.Header)
			return s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
		}),
	}
	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})
	_, err := client.Generate(c.Request.Context(), key, modelID, commandcode.Request{
		Model: modelID, Messages: []map[string]any{{"role": "user", "content": prompt}}, MaxTokens: 256, Stream: true,
	}, func(text string) error {
		s.sendEvent(c, TestEvent{Type: "content", Text: text})
		return c.Request.Context().Err()
	})
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Model: modelID, Success: true})
	return nil
}
