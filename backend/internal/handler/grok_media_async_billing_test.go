//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the handler's actual AsyncVideoStart callback, not just the Redis
// claim helper: the old image-only gate claimed billing but never wrote usage.
func TestGrokAsyncVideoCompletionRecordsAndBillsOnce(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		name := "connected"
		if disconnect {
			name = "client_disconnected"
		}
		t.Run(name, func(t *testing.T) {
			h, slots, bindings, upstream := newGrokMediaSlotHandler(t, false, false)
			usage := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 4)}
			billing := &compatStreamUsageBillingRepo{}
			account := service.Account{ID: 1, Platform: service.PlatformGrok, Type: service.AccountTypeAPIKey,
				Status: service.StatusActive, Schedulable: true, Concurrency: 50,
				GroupIDs: []int64{24}, Credentials: map[string]any{"api_key": "test-key", "base_url": "https://video.example.test",
					"model_mapping": map[string]any{"MiniMax-H3": "MiniMax-H3"}}}
			repo := grokMediaSlotRepo{openAIImagesFailoverAccountRepo: openAIImagesFailoverAccountRepo{accounts: []service.Account{account}}}
			cfg := &config.Config{}
			// Keep the handler's simple admission fixture but use normal billing in
			// the gateway so Apply must receive a non-zero monetary command.
			h.gatewayService = service.NewOpenAIGatewayService(repo, usage, billing, nil, nil, nil, bindings, cfg, nil,
				service.NewConcurrencyService(slots), service.NewBillingService(cfg, nil), nil, nil, upstream,
				&service.DeferredService{}, nil, service.NewGrokTokenProvider(repo, nil), nil, nil, nil, nil, nil)
			started, release := make(chan struct{}), make(chan struct{})
			upstream.call = func(req *http.Request, _ int64) (*http.Response, error) {
				if req.Method == http.MethodPost {
					close(started)
					<-release
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"request_id":"upstream-task","status":"done","model":"MiniMax-H3","video":{"url":"https://video.example.test/result.mp4","duration":5,"resolution":"768p"}}`))}, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, w := grokMediaSlotContext(ctx, true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations",
				strings.NewReader(`{"model":"MiniMax-H3","prompt":"test","duration":5,"resolution":"768p"}`)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			key, ok := middleware.GetAPIKeyFromContext(c)
			require.True(t, ok)
			key.User.Balance = 100
			key.Group.Hydrated = true
			key.Group.RateMultiplier = 0.4
			key.Group.VideoRateIndependent = true
			key.Group.VideoRateMultiplier = 0.3
			price := 0.5
			key.Group.VideoPrice480P = &price
			h.GrokVideoGeneration(c)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var pending struct {
				RequestID string `json:"request_id"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pending))
			require.True(t, strings.HasPrefix(pending.RequestID, "mmh3-"), w.Body.String())
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("background POST did not start")
			}
			require.Empty(t, usage.created, "pending jobs must not bill")
			if disconnect {
				cancel()
			}
			close(release)
			var log *service.UsageLog
			select {
			case log = <-usage.created:
			case <-time.After(5 * time.Second):
				t.Fatal("completed video claimed billing without recording usage")
			}
			require.Equal(t, service.StableGrokVideoBillingRequestID(pending.RequestID), log.RequestID)
			require.InDelta(t, 0.75, log.ActualCost, 0.000001)
			billing.mu.Lock()
			require.Len(t, billing.commands, 1)
			require.InDelta(t, 0.75, billing.commands[0].BalanceCost, 0.000001)
			billing.mu.Unlock()
			// Repeated status observations use the same claim and cannot bill again.
			for range 3 {
				poll, out := grokMediaSlotContext(context.Background(), false)
				poll.Params = gin.Params{{Key: "request_id", Value: pending.RequestID}}
				poll.Set(string(middleware.ContextKeyAPIKey), key)
				h.GrokVideoStatus(poll)
				require.Equal(t, http.StatusOK, out.Code, out.Body.String())
			}
			require.Empty(t, usage.created)
			billing.mu.Lock()
			require.Len(t, billing.commands, 1)
			billing.mu.Unlock()
		})
	}
}
