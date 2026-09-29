//go:build unit

package service

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountTestService_CommandCodeNativeRoute(t *testing.T) {
	for _, kind := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		t.Run(kind, func(t *testing.T) {
			account := &Account{ID: 701, Platform: PlatformCommandCode, Type: kind, Concurrency: 1, Credentials: map[string]any{
				"api_key": "native-test-key", "access_token": "not-the-api-key", "base_url": "https://api.commandcode.ai",
				"model_mapping": map[string]any{"short-model": "provider/native-model"},
			}}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"text-delta\",\"text\":\"native pong\"}\n\ndata: {\"type\":\"finish\",\"finishReason\":\"stop\",\"usage\":{\"inputTokens\":3,\"outputTokens\":2}}\n\ndata: [DONE]\n\n"))}
			svc, upstream := adaptiveCNAccountTestService(account, resp)
			c, recorder := newTestContext()
			err := svc.TestAccountConnection(c, account.ID, "short-model", "ping", AccountTestModeDefault)
			require.Len(t, upstream.requests, 1)
			request := upstream.requests[0]
			fmt.Printf("COMMANDCODE_ROUTE type=%s method=%s url=%s body=%s\n", kind, request.Method, request.URL.String(), upstream.bodies[0])
			require.Equal(t, "https://api.commandcode.ai/alpha/generate", request.URL.String())
			require.Equal(t, "Bearer native-test-key", request.Header.Get("Authorization"))
			require.Contains(t, string(upstream.bodies[0]), "provider/native-model")
			require.Contains(t, string(upstream.bodies[0]), "ping")
			require.NoError(t, err)
			require.Contains(t, recorder.Body.String(), "native pong")
			require.Contains(t, recorder.Body.String(), `"success":true`)
		})
	}
}
