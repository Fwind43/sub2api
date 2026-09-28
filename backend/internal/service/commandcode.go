package service

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/commandcode"
	"github.com/gin-gonic/gin"
)

type commandCodeHTTPDoer func(*http.Request) (*http.Response, error)

func (fn commandCodeHTTPDoer) Do(req *http.Request) (*http.Response, error) { return fn(req) }

// sendCommandCodeRequest uses the selected account's own key and proxy.
// No CPA address or credential is involved in this path.
func (s *OpenAIGatewayService) sendCommandCodeRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, stream bool) (*http.Response, error) {
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
			return s.doOpenAIUpstream(req, proxyURL, account)
		}),
	}
	SetActualOpenAIUpstreamEndpoint(c, "/alpha/generate")
	resp, err := client.ChatCompletion(ctx, account.GetCredential("api_key"), body, stream)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	return resp, nil
}
