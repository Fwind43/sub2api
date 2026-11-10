package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func grokAsyncTestAccount() *Account {
	return &Account{
		ID:          1767,
		Name:        "minimax-h3-getoken",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 10,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://relay.example",
		},
	}
}

func grokAsyncTestContext(method, rawURL string, headers map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, rawURL, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c, rec
}

func grokAsyncJSONResponse(body string) *http.Response {
	return grokAsyncStatusJSONResponse(http.StatusOK, body)
}

func grokAsyncStatusJSONResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type asyncVideoTestGatewayCache struct {
	schedulerTestGatewayCache
	mu      sync.Mutex
	pending map[string][]byte
	claimed map[string]bool
}

func (c *asyncVideoTestGatewayCache) SetGrokVideoPendingBilling(_ context.Context, key string, payload []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		c.pending = make(map[string][]byte)
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	c.pending[key] = cp
	return nil
}

func (c *asyncVideoTestGatewayCache) GetGrokVideoPendingBilling(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, ok := c.pending[key]
	if !ok {
		return nil, nil
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return cp, nil
}

func (c *asyncVideoTestGatewayCache) ClaimGrokVideoBilled(_ context.Context, key string, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.claimed == nil {
		c.claimed = make(map[string]bool)
	}
	if c.claimed[key] {
		return false, nil
	}
	c.claimed[key] = true
	return true, nil
}

type blockingAsyncVideoUpstreamStub struct {
	mu       sync.Mutex
	requests []*http.Request
	handler  func(req *http.Request) (*http.Response, error)
}

func (s *blockingAsyncVideoUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	h := s.handler
	s.mu.Unlock()
	return h(req)
}

func (s *blockingAsyncVideoUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestStartAsyncGrokVideoGeneration_CompletesInBackgroundAfterClientCancelAndServesStatusAndContent(t *testing.T) {
	postStarted := make(chan struct{})
	releasePost := make(chan struct{})
	completedCh := make(chan *OpenAIForwardResult, 1)

	upstream := &blockingAsyncVideoUpstreamStub{
		handler: func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/v1/videos/generations"):
				close(postStarted)
				select {
				case <-releasePost:
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
				return grokAsyncJSONResponse(`{
					"request_id":"up-real-123",
					"status":"done",
					"model":"MiniMax-H3",
					"video":{"url":"https://relay.example/v1/videos/up-real-123/content","duration":5,"respect_moderation":true}
				}`), nil
			case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/v1/videos/up-real-123"):
				return grokAsyncJSONResponse(`{"status":"done"}`), nil
			case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/v1/videos/up-real-123/content"):
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":   []string{"video/mp4"},
						"Content-Length": []string{"9"},
					},
					Body: io.NopCloser(strings.NewReader("mp4-bytes")),
				}, nil
			default:
				t.Fatalf("unexpected upstream request: %s %s", req.Method, req.URL.String())
				return nil, nil
			}
		},
	}

	cache := &asyncVideoTestGatewayCache{}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		cache:        cache,
		httpUpstream: upstream,
	}

	clientCtx, cancelClient := context.WithCancel(context.Background())
	createGinCtx, createRecorder := grokAsyncTestContext(
		http.MethodPost,
		"https://code.fwind.work/v1/videos/generations",
		map[string]string{"Content-Type": "application/json"},
	)

	reqBody := []byte(`{"model":"MiniMax-H3","prompt":"cat on skateboard","resolution":"768p","duration":5}`)
	createRes, err := svc.ForwardGrokMedia(
		clientCtx,
		createGinCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideosGenerations,
		"",
		reqBody,
		"application/json",
	)
	require.NoError(t, err)
	require.NotNil(t, createRes)
	require.NotNil(t, createRes.AsyncVideoStart)
	syntheticID := createRes.ResponseID
	require.True(t, strings.HasPrefix(syntheticID, "mmh3-"))
	require.Equal(t, http.StatusOK, createRecorder.Code)
	require.Equal(t, syntheticID, gjson.GetBytes(createRecorder.Body.Bytes(), "request_id").String())
	require.Equal(t, "pending", gjson.GetBytes(createRecorder.Body.Bytes(), "status").String())

	createRes.AsyncVideoStart(func(billed *OpenAIForwardResult) {
		completedCh <- billed
	})

	// Simulate Cloudflare / client disconnect immediately after receiving synthetic request_id.
	cancelClient()

	select {
	case <-postStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("background upstream POST did not start")
	}

	// Polling status while upstream POST is still in-flight must return pending.
	pendingCtx, pendingRecorder := grokAsyncTestContext(
		http.MethodGet,
		"https://code.fwind.work/v1/videos/"+syntheticID,
		nil,
	)
	pendingRes, err := svc.ForwardGrokMedia(
		context.Background(),
		pendingCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideoStatus,
		syntheticID,
		nil,
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, pendingRes)
	require.Equal(t, http.StatusOK, pendingRecorder.Code)
	require.Equal(t, "pending", gjson.GetBytes(pendingRecorder.Body.Bytes(), "status").String())
	require.Zero(t, pendingRes.VideoCount)

	// Unblock upstream POST completion.
	close(releasePost)

	var bgBilled *OpenAIForwardResult
	select {
	case bgBilled = <-completedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("background completion callback did not fire")
	}
	require.NotNil(t, bgBilled)
	require.Equal(t, 1, bgBilled.VideoCount)
	require.Equal(t, 5, bgBilled.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution480P, bgBilled.VideoResolution)

	// Subsequent status poll returns done with rewritten content URL using synthetic ID.
	doneCtx, doneRecorder := grokAsyncTestContext(
		http.MethodGet,
		"https://code.fwind.work/v1/videos/"+syntheticID,
		nil,
	)
	doneRes, err := svc.ForwardGrokMedia(
		context.Background(),
		doneCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideoStatus,
		syntheticID,
		nil,
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, doneRes)
	require.Equal(t, http.StatusOK, doneRecorder.Code)
	require.Equal(t, "done", gjson.GetBytes(doneRecorder.Body.Bytes(), "status").String())
	require.Equal(t, "/v1/videos/"+syntheticID+"/content", gjson.GetBytes(doneRecorder.Body.Bytes(), "video.url").String())
	require.Equal(t, 1, doneRes.VideoCount)

	// Content download resolves synthetic ID -> upstream real ID and streams MP4.
	contentCtx, contentRecorder := grokAsyncTestContext(
		http.MethodGet,
		"https://code.fwind.work/v1/videos/"+syntheticID+"/content",
		nil,
	)
	contentRes, err := svc.ForwardGrokMedia(
		context.Background(),
		contentCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideoContent,
		syntheticID,
		nil,
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, contentRes)
	require.Equal(t, http.StatusOK, contentRecorder.Code)
	require.Equal(t, "mp4-bytes", contentRecorder.Body.String())
}

func TestStartAsyncGrokVideoGeneration_PropagatesUpstreamErrorToStatus(t *testing.T) {
	postDone := make(chan struct{})
	upstream := &blockingAsyncVideoUpstreamStub{
		handler: func(req *http.Request) (*http.Response, error) {
			defer close(postDone)
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"MiniMax-H3 GPU node unavailable"}}`)),
			}, nil
		},
	}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		cache:        &asyncVideoTestGatewayCache{},
		httpUpstream: upstream,
	}

	createGinCtx, _ := grokAsyncTestContext(
		http.MethodPost,
		"https://code.fwind.work/v1/videos/generations",
		map[string]string{"Content-Type": "application/json"},
	)
	createRes, err := svc.ForwardGrokMedia(
		context.Background(),
		createGinCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideosGenerations,
		"",
		[]byte(`{"model":"MiniMax-H3","prompt":"test","resolution":"768p","duration":5}`),
		"application/json",
	)
	require.NoError(t, err)
	require.NotNil(t, createRes)
	require.NotNil(t, createRes.AsyncVideoStart)
	syntheticID := createRes.ResponseID
	createRes.AsyncVideoStart(nil)

	select {
	case <-postDone:
	case <-time.After(3 * time.Second):
		t.Fatal("background upstream POST did not complete")
	}
	// Wait briefly for state write after response read.
	time.Sleep(50 * time.Millisecond)

	statusCtx, statusRecorder := grokAsyncTestContext(
		http.MethodGet,
		"https://code.fwind.work/v1/videos/"+syntheticID,
		nil,
	)
	_, err = svc.ForwardGrokMedia(
		context.Background(),
		statusCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideoStatus,
		syntheticID,
		nil,
		"",
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, statusRecorder.Code)
	require.Contains(t, statusRecorder.Body.String(), "MiniMax-H3 GPU node unavailable")
}

func TestValidateGrokMediaVideoRequest_MiniMaxH3DurationAndConditions(t *testing.T) {
	t.Parallel()

	for _, validDuration := range []int{4, 6, 8, 12, 15} {
		body := []byte(fmt.Sprintf(`{"model":"MiniMax-H3","prompt":"test","duration":%d}`, validDuration))
		require.NoError(t, ValidateGrokMediaVideoRequest(GrokMediaEndpointVideosGenerations, "application/json", body))
	}

	for _, invalidBody := range []string{
		`{"model":"MiniMax-H3","prompt":"test","duration":3}`,
		`{"model":"MiniMax-H3","prompt":"test","duration":16}`,
		`{"model":"MiniMax-H3","prompt":"test","duration":6.5}`,
	} {
		err := ValidateGrokMediaVideoRequest(GrokMediaEndpointVideosGenerations, "application/json", []byte(invalidBody))
		require.Error(t, err)
		require.Contains(t, err.Error(), "MiniMax-H3 duration")
	}

	reqInfo := ParseGrokMediaRequest("application/json", []byte(`{"model":"MiniMax-H3","prompt":"test","duration":12}`))
	require.Equal(t, 12, reqInfo.DurationSeconds)
	defaultInfo := ParseGrokMediaRequest("application/json", []byte(`{"model":"MiniMax-H3","prompt":"test"}`))
	require.Equal(t, 6, defaultInfo.DurationSeconds)
}

func TestStartAsyncGrokVideoGeneration_GeTokenNativeAsyncRelativeContentURLAndVideoJobNotFound(t *testing.T) {
	t.Parallel()

	upstreamHits := make(chan string, 8)
	upstreamClient := &blockingAsyncVideoUpstreamStub{
		handler: func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/v1/videos/generations":
				require.Equal(t, "respond-async", r.Header.Get("Prefer"))
				require.Equal(t, "client-idem-123", r.Header.Get("Idempotency-Key"))
				upstreamHits <- "POST /v1/videos/generations"
				return grokAsyncStatusJSONResponse(
					http.StatusAccepted,
					`{"id":"mmh3-native-1","request_id":"mmh3-native-1","object":"video.generation","model":"MiniMax-H3","status":"pending","status_url":"/v1/videos/mmh3-native-1","content_url":"/v1/videos/mmh3-native-1/content","poll_after_ms":5000}`,
				), nil
			case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/mmh3-native-1":
				upstreamHits <- "GET /v1/videos/mmh3-native-1"
				return grokAsyncStatusJSONResponse(
					http.StatusOK,
					`{"id":"mmh3-native-1","request_id":"mmh3-native-1","object":"video.generation","model":"MiniMax-H3","status":"done","resolution":"768p","duration":12,"status_url":"/v1/videos/mmh3-native-1","content_url":"/v1/videos/mmh3-native-1/content"}`,
				), nil
			case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/mmh3-missing-1":
				upstreamHits <- "GET /v1/videos/mmh3-missing-1"
				return grokAsyncStatusJSONResponse(
					http.StatusNotFound,
					`{"error":{"message":"video job not found","type":"invalid_request_error","code":"video_job_not_found"}}`,
				), nil
			default:
				return grokAsyncStatusJSONResponse(http.StatusNotFound, `{"error":"unexpected path"}`), nil
			}
		},
	}

	cache := &asyncVideoTestGatewayCache{}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		cache:        cache,
		httpUpstream: upstreamClient,
	}

	createGinCtx, createRecorder := grokAsyncTestContext(
		http.MethodPost,
		"https://code.fwind.work/v1/videos/generations",
		map[string]string{
			"Content-Type":    "application/json",
			"Prefer":          "respond-async",
			"Idempotency-Key": "client-idem-123",
		},
	)
	createRes, err := svc.ForwardGrokMedia(
		context.Background(),
		createGinCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideosGenerations,
		"",
		[]byte(`{"model":"MiniMax-H3","prompt":"native async","resolution":"768p","duration":12}`),
		"application/json",
	)
	require.NoError(t, err)
	require.NotNil(t, createRes)
	require.Equal(t, http.StatusAccepted, createRecorder.Code)
	require.Contains(t, createRecorder.Header().Get("Location"), "/v1/videos/"+createRes.ResponseID)
	require.Equal(t, "5", createRecorder.Header().Get("Retry-After"))
	syntheticID := createRes.ResponseID
	createRes.AsyncVideoStart(nil)

	select {
	case <-upstreamHits:
	case <-time.After(3 * time.Second):
		t.Fatal("background upstream POST did not complete")
	}
	time.Sleep(50 * time.Millisecond)

	statusCtx, statusRecorder := grokAsyncTestContext(
		http.MethodGet,
		"https://code.fwind.work/v1/videos/"+syntheticID,
		nil,
	)
	statusRes, err := svc.ForwardGrokMedia(
		context.Background(),
		statusCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideoStatus,
		syntheticID,
		nil,
		"",
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusRecorder.Code)
	require.Equal(t, 1, statusRes.VideoCount)
	require.Equal(t, 12, statusRes.VideoDurationSeconds)
	require.Equal(t, "/v1/videos/"+syntheticID+"/content", gjson.GetBytes(statusRecorder.Body.Bytes(), "content_url").String())

	// Also verify direct upstream status lookup on 404 video_job_not_found does NOT fall back to /v1/videos/generations/:id
	missingCtx, missingRecorder := grokAsyncTestContext(
		http.MethodGet,
		"https://code.fwind.work/v1/videos/mmh3-missing-1",
		nil,
	)
	_, err = svc.ForwardGrokMedia(
		context.Background(),
		missingCtx,
		grokAsyncTestAccount(),
		GrokMediaEndpointVideoStatus,
		"mmh3-missing-1",
		nil,
		"",
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, missingRecorder.Code)
	require.Equal(t, "video_job_not_found", gjson.GetBytes(missingRecorder.Body.Bytes(), "error.code").String())
}
