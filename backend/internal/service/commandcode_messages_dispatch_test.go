//go:build unit

package service

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The group-level Messages dispatch mapping defaults (gpt-5.4 / gpt-5.3-codex)
// are openai-only; a Command Code group must never rewrite a claude-* model.
// Its dispatch config is sanitized away anyway, this pins the decision itself.
func TestResolveMessagesDispatchModel_CommandCodeSkipsOpenAIDefaults(t *testing.T) {
	group := &Group{Platform: PlatformCommandCode}
	require.Empty(t, group.ResolveMessagesDispatchModel("claude-sonnet-4-5"))
	require.Empty(t, group.ResolveMessagesDispatchModel("claude-opus-4-1"))
}

func commandCodeMessagesIngress(t *testing.T) routingMatrixIngress {
	t.Helper()
	for _, ingress := range routingMatrixIngresses() {
		if ingress.name == "messages" {
			return ingress
		}
	}
	t.Fatal("messages ingress not found in the routing matrix")
	return routingMatrixIngress{}
}

// An inbound /v1/messages request on a Command Code account (the group switch
// allow_messages_dispatch is sanitized to false for this platform, see
// TestAllowOpenAICompatibleMessagesDispatch_CommandCodeExempt) must still reach
// the Command Code native upstream, with the requested model name intact.
func TestForwardAsAnthropic_CommandCodeReachesNativeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ingress := commandCodeMessagesIngress(t)

	for _, model := range []string{"claude-sonnet-4-5-20250929", "claude-opus-4-1"} {
		t.Run(model, func(t *testing.T) {
			account := commandCodeTestAccount(901)
			body := (routingMatrixCase{ingress: ingress, model: model}).body()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, ingress.path, bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			err := ingress.forward(svc, c, account, body)

			require.Error(t, err)
			require.NotEmpty(t, upstream.requests, "入站 /v1/messages 必须产生一次 Command Code 上游请求")
			require.True(t, strings.HasPrefix(upstream.lastReq.URL.String(), "https://api.commandcode.ai/"),
				"入站 /v1/messages 必须打到 Command Code 原生上游: %s", upstream.lastReq.URL.String())
			require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String(),
				"模型名不得被调度级 gpt-5.x 默认映射改写")
		})
	}
}
