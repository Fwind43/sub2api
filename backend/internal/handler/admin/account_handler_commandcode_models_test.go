package admin

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCommandCodeAvailableModelsMapping(t *testing.T) {
	for _, typ := range []string{service.AccountTypeOAuth, service.AccountTypeAPIKey} {
		t.Run(typ, func(t *testing.T) {
			mapping := map[string]any{"glm-5.3-flash": "z-ai/glm-5.3-flash", "gpt-5": "openai/gpt-5"}
			svc := &availableModelsAdminService{stubAdminService: newStubAdminService(), account: service.Account{
				ID: 44, Platform: service.PlatformCommandCode, Type: typ,
				Credentials: map[string]any{"model_mapping": mapping},
			}}
			rec := httptest.NewRecorder()
			setupAvailableModelsRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/44/models", nil))
			require.Equal(t, http.StatusOK, rec.Code)
			var resp struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			ids := make([]string, 0, len(resp.Data))
			for _, m := range resp.Data {
				ids = append(ids, m.ID)
			}
			t.Logf("platform=%s type=%s models=%v", svc.account.Platform, typ, ids)
			require.Equal(t, []string{"glm-5.3-flash", "gpt-5"}, ids)
			require.Equal(t, "z-ai/glm-5.3-flash", mapping["glm-5.3-flash"])
		})
	}
}

func TestCommandCodeAvailableModelsDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   int
	}{
		{"catalog", 200, `{"data":[{"id":"openai/gpt-5"},{"id":"z-ai/glm-5.3-flash"}]}`, 200},
		{"upstream_error", 500, `PRIVATE_UPSTREAM_BODY`, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &availableModelsAdminService{stubAdminService: newStubAdminService(), account: service.Account{ID: 44, Platform: service.PlatformCommandCode, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-not-a-real-key"}}}
			upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}}
			testSvc := service.NewAccountTestService(nil, nil, nil, nil, nil, upstream, &config.Config{}, nil)
			h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, testSvc, nil, nil, nil, nil, nil)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/models", h.GetAvailableModels)
			router = gin.New()
			router.GET("/api/v1/admin/accounts/:id/models", h.GetAvailableModels)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/44/models", nil))
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM_BODY")
			require.NotContains(t, rec.Body.String(), "claude-")
			if tc.want == 200 {
				require.Contains(t, rec.Body.String(), "openai/gpt-5")
				require.Contains(t, rec.Body.String(), "z-ai/glm-5.3-flash")
			}
		})
	}
}
func TestCommandCodeAvailableModelsMissingService(t *testing.T) {
	svc := &availableModelsAdminService{stubAdminService: newStubAdminService(), account: service.Account{ID: 44, Platform: service.PlatformCommandCode, Type: service.AccountTypeOAuth}}
	rec := httptest.NewRecorder()
	setupAvailableModelsRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/44/models", nil))
	require.Equal(t, 500, rec.Code)
	require.NotContains(t, rec.Body.String(), "claude-")
}
