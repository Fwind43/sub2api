package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const (
	grokAsyncVideoIDPrefix     = "mmh3-"
	grokAsyncVideoCacheKeyPref = "async_job:"
	grokAsyncVideoUpstreamWait = 30 * time.Minute
	grokAsyncVideoMaxBodyBytes = 16 << 20
)

type grokAsyncVideoJobState struct {
	SyntheticID  string `json:"synthetic_id"`
	UpstreamID   string `json:"upstream_id,omitempty"`
	Status       string `json:"status"` // "pending" | "done" | "failed"
	Model        string `json:"model,omitempty"`
	UpstreamBody string `json:"upstream_body,omitempty"`
	ErrorCode    int    `json:"error_code,omitempty"`
	ErrorBody    string `json:"error_body,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

func shouldWrapGrokVideoAsync(endpoint GrokMediaEndpoint, requestModel, upstreamModel string) bool {
	switch endpoint {
	case GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
	default:
		return false
	}
	for _, candidate := range []string{upstreamModel, requestModel} {
		lower := strings.ToLower(strings.TrimSpace(candidate))
		if strings.HasPrefix(lower, "minimax") {
			return true
		}
	}
	return false
}

func isAsyncGrokVideoRequestID(requestID string) bool {
	return strings.HasPrefix(strings.TrimSpace(requestID), grokAsyncVideoIDPrefix)
}

func (s *OpenAIGatewayService) storeGrokAsyncVideoJobState(ctx context.Context, state grokAsyncVideoJobState) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("gateway cache is not configured")
	}
	id := strings.TrimSpace(state.SyntheticID)
	if id == "" {
		return fmt.Errorf("synthetic video request_id is required")
	}
	state.UpdatedAt = time.Now().UnixMilli()
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return s.cache.SetGrokVideoPendingBilling(storeCtx, grokAsyncVideoCacheKeyPref+id, raw, grokVideoPendingBillingTTL(s.cfg))
}

func (s *OpenAIGatewayService) getGrokAsyncVideoJobState(ctx context.Context, syntheticID string) (*grokAsyncVideoJobState, error) {
	if s == nil || s.cache == nil {
		return nil, fmt.Errorf("gateway cache is not configured")
	}
	id := strings.TrimSpace(syntheticID)
	if id == "" {
		return nil, nil
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	raw, err := s.cache.GetGrokVideoPendingBilling(readCtx, grokAsyncVideoCacheKeyPref+id)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil, err
	}
	var state grokAsyncVideoJobState
	if uerr := json.Unmarshal(raw, &state); uerr != nil {
		return nil, uerr
	}
	return &state, nil
}

func rewriteAsyncGrokVideoStatusBody(body []byte, upstreamID, syntheticID, contentProxyURL string) []byte {
	if len(body) == 0 {
		return body
	}
	out := body
	if strings.TrimSpace(upstreamID) != "" {
		out = rewriteGrokMediaVideoContentURLs(out, upstreamID, contentProxyURL)
	}
	out = rewriteGrokMediaVideoContentURLs(out, syntheticID, contentProxyURL)
	if !gjson.ValidBytes(out) {
		return out
	}
	if gjson.GetBytes(out, "request_id").Exists() {
		if updated, err := sjson.SetBytes(out, "request_id", syntheticID); err == nil {
			out = updated
		}
	}
	if gjson.GetBytes(out, "id").Exists() {
		if updated, err := sjson.SetBytes(out, "id", syntheticID); err == nil {
			out = updated
		}
	}
	if !gjson.GetBytes(out, "request_id").Exists() && !gjson.GetBytes(out, "id").Exists() {
		if updated, err := sjson.SetBytes(out, "request_id", syntheticID); err == nil {
			out = updated
		}
	}
	return out
}

func (s *OpenAIGatewayService) startAsyncGrokMediaVideoCreate(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	endpoint GrokMediaEndpoint,
	targetURL string,
	body []byte,
	contentType string,
	requestInfo GrokMediaRequestInfo,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	syntheticID := grokAsyncVideoIDPrefix + uuid.NewString()
	requestModel := requestInfo.Model
	initialState := grokAsyncVideoJobState{
		SyntheticID: syntheticID,
		Status:      "pending",
		Model:       firstNonEmpty(requestModel, upstreamModel),
		CreatedAt:   startTime.UnixMilli(),
	}
	if err := s.storeGrokAsyncVideoJobState(ctx, initialState); err != nil {
		return nil, fmt.Errorf("store async video job state: %w", err)
	}

	contentProxyURL := grokMediaContentProxyURL(c, syntheticID)
	bodyCopy := append([]byte(nil), body...)
	initResp, _ := json.Marshal(map[string]any{
		"request_id": syntheticID,
		"status":     "pending",
	})
	if c != nil {
		c.Data(http.StatusOK, "application/json", initResp)
	}

	result := &OpenAIForwardResult{
		RequestID:            syntheticID,
		ResponseID:           syntheticID,
		Model:                requestModel,
		BillingModel:         requestModel,
		UpstreamModel:        upstreamModel,
		Duration:             time.Since(startTime),
		VideoCount:           1,
		VideoResolution:      requestInfo.Resolution,
		VideoDurationSeconds: requestInfo.DurationSeconds,
	}
	result.AsyncVideoStart = func(onComplete func(*OpenAIForwardResult)) {
		go s.runAsyncGrokMediaVideoCreate(
			context.WithoutCancel(ctx),
			account,
			token,
			endpoint,
			targetURL,
			bodyCopy,
			contentType,
			requestInfo,
			upstreamModel,
			syntheticID,
			contentProxyURL,
			startTime,
			onComplete,
		)
	}
	return result, nil
}

func (s *OpenAIGatewayService) runAsyncGrokMediaVideoCreate(
	baseCtx context.Context,
	account *Account,
	token string,
	endpoint GrokMediaEndpoint,
	targetURL string,
	body []byte,
	contentType string,
	requestInfo GrokMediaRequestInfo,
	upstreamModel string,
	syntheticID string,
	contentProxyURL string,
	startTime time.Time,
	onComplete func(*OpenAIForwardResult),
) {
	bgLog := logger.L().With(
		zap.String("component", "service.grok_media.async_video"),
		zap.String("synthetic_id", syntheticID),
		zap.Int64("account_id", account.ID),
		zap.String("upstream_model", upstreamModel),
	)
	longCtx := WithHTTPUpstreamProfile(context.WithoutCancel(baseCtx), HTTPUpstreamProfileLongVideo)
	upstreamCtx, cancel := context.WithTimeout(longCtx, grokAsyncVideoUpstreamWait)
	defer cancel()

	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		bgLog.Warn("grok_media.async_video_build_request_failed", zap.Error(err))
		_ = s.storeGrokAsyncVideoJobState(context.Background(), grokAsyncVideoJobState{
			SyntheticID: syntheticID,
			Status:      "failed",
			Model:       firstNonEmpty(requestInfo.Model, upstreamModel),
			ErrorCode:   http.StatusBadGateway,
			ErrorBody:   `{"error":{"message":"failed to build upstream video request","type":"upstream_error"}}`,
			CreatedAt:   startTime.UnixMilli(),
		})
		return
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(targetURL) {
		applyGrokCLIHeaders(upstreamReq.Header)
	}
	if endpoint.RequiresRequestBody() {
		ct := strings.TrimSpace(contentType)
		if ct == "" {
			ct = "application/json"
		}
		upstreamReq.Header.Set("Content-Type", ct)
	}
	account.ApplyHeaderOverrides(upstreamReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		bgLog.Warn("grok_media.async_video_upstream_transport_failed", zap.Error(err))
		errJSON, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": "upstream video generation request failed: " + err.Error(),
				"type":    "upstream_error",
			},
		})
		_ = s.storeGrokAsyncVideoJobState(context.Background(), grokAsyncVideoJobState{
			SyntheticID: syntheticID,
			Status:      "failed",
			Model:       firstNonEmpty(requestInfo.Model, upstreamModel),
			ErrorCode:   http.StatusBadGateway,
			ErrorBody:   string(errJSON),
			CreatedAt:   startTime.UnixMilli(),
		})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	requestModel := requestInfo.Model
	s.updateGrokUsageFromResponse(withGrokTeamRateLimitModel(upstreamCtx, requestModel), account, resp.Header, resp.StatusCode)
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, grokAsyncVideoMaxBodyBytes))
	if readErr != nil {
		bgLog.Warn("grok_media.async_video_read_response_failed", zap.Error(readErr))
		_ = s.storeGrokAsyncVideoJobState(context.Background(), grokAsyncVideoJobState{
			SyntheticID: syntheticID,
			Status:      "failed",
			Model:       firstNonEmpty(requestModel, upstreamModel),
			ErrorCode:   http.StatusBadGateway,
			ErrorBody:   `{"error":{"message":"failed to read upstream video response","type":"upstream_error"}}`,
			CreatedAt:   startTime.UnixMilli(),
		})
		return
	}

	if resp.StatusCode >= 400 {
		bgLog.Warn("grok_media.async_video_upstream_http_error",
			zap.Int("status_code", resp.StatusCode),
			zap.String("body", truncateString(string(respBody), 512)),
		)
		_ = s.storeGrokAsyncVideoJobState(context.Background(), grokAsyncVideoJobState{
			SyntheticID: syntheticID,
			Status:      "failed",
			Model:       firstNonEmpty(requestModel, upstreamModel),
			ErrorCode:   resp.StatusCode,
			ErrorBody:   string(respBody),
			CreatedAt:   startTime.UnixMilli(),
		})
		return
	}

	requestIDHeader := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	upstreamID := strings.TrimSpace(firstNonEmpty(extractGrokMediaVideoRequestID(respBody), requestIDHeader))
	statusBody := respBody

	if upstreamID != "" && strings.TrimSpace(gjson.GetBytes(statusBody, "video.url").String()) == "" {
		if fetched, fetchErr := s.fetchAsyncUpstreamVideoStatusBody(upstreamCtx, account, token, proxyURL, upstreamID); fetchErr == nil && len(fetched) > 0 {
			statusBody = fetched
		} else if fetchErr != nil {
			bgLog.Warn("grok_media.async_video_initial_status_fetch_failed",
				zap.String("upstream_id", upstreamID),
				zap.Error(fetchErr),
			)
		}
	}

	rewrittenBody := rewriteAsyncGrokVideoStatusBody(statusBody, upstreamID, syntheticID, contentProxyURL)
	billed := ExtractGrokVideoBillingFromStatusBody(rewrittenBody, nil, syntheticID)
	jobStatus := "pending"
	if billed != nil {
		jobStatus = "done"
	} else if st := strings.ToLower(strings.TrimSpace(gjson.GetBytes(rewrittenBody, "status").String())); st == "failed" || st == "error" || st == "expired" {
		jobStatus = "failed"
	}

	state := grokAsyncVideoJobState{
		SyntheticID:  syntheticID,
		UpstreamID:   upstreamID,
		Status:       jobStatus,
		Model:        firstNonEmpty(requestModel, upstreamModel),
		UpstreamBody: string(rewrittenBody),
		CreatedAt:    startTime.UnixMilli(),
	}
	if jobStatus == "failed" {
		state.ErrorCode = http.StatusBadGateway
		state.ErrorBody = string(rewrittenBody)
	}
	if err := s.storeGrokAsyncVideoJobState(context.Background(), state); err != nil {
		bgLog.Error("grok_media.async_video_store_final_state_failed",
			zap.String("upstream_id", upstreamID),
			zap.String("status", jobStatus),
			zap.Error(err),
		)
	}

	if billed != nil && onComplete != nil {
		if strings.TrimSpace(billed.Model) == "" {
			billed.Model = requestModel
		}
		if strings.TrimSpace(billed.BillingModel) == "" {
			billed.BillingModel = firstNonEmpty(requestModel, billed.Model)
		}
		if strings.TrimSpace(billed.UpstreamModel) == "" {
			billed.UpstreamModel = upstreamModel
		}
		if strings.TrimSpace(billed.VideoResolution) == "" {
			billed.VideoResolution = requestInfo.Resolution
		}
		if billed.VideoDurationSeconds <= 0 {
			billed.VideoDurationSeconds = requestInfo.DurationSeconds
		}
		billed.RequestID = syntheticID
		billed.ResponseID = syntheticID
		billed.Duration = time.Since(startTime)
		onComplete(billed)
	}
}

func (s *OpenAIGatewayService) fetchAsyncUpstreamVideoStatusBody(
	ctx context.Context,
	account *Account,
	token string,
	proxyURL string,
	upstreamID string,
) ([]byte, error) {
	statusURL, err := buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoStatus, upstreamID)
	if err != nil {
		return nil, err
	}
	statusCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(statusCtx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(statusURL) {
		applyGrokCLIHeaders(req.Header)
	}
	account.ApplyHeaderOverrides(req.Header)

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, grokAsyncVideoMaxBodyBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("upstream status http %d: %s", resp.StatusCode, truncateString(string(body), 256))
	}
	return body, nil
}

func (s *OpenAIGatewayService) forwardAsyncGrokMediaVideoStatus(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	syntheticID string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	state, err := s.getGrokAsyncVideoJobState(ctx, syntheticID)
	if err != nil {
		return nil, fmt.Errorf("read async video job state: %w", err)
	}
	if state == nil {
		if c != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "Video request not found or expired",
					"type":    "invalid_request_error",
				},
			})
		}
		return &OpenAIForwardResult{
			RequestID:  syntheticID,
			ResponseID: syntheticID,
			Model:      "MiniMax-H3",
			Duration:   time.Since(startTime),
		}, nil
	}

	contentProxyURL := grokMediaContentProxyURL(c, syntheticID)
	if state.Status == "failed" {
		statusCode := state.ErrorCode
		if statusCode < 400 {
			statusCode = http.StatusBadGateway
		}
		errBody := strings.TrimSpace(state.ErrorBody)
		if errBody == "" {
			errBody = `{"error":{"message":"upstream video generation failed","type":"upstream_error"}}`
		}
		if c != nil {
			c.Data(statusCode, "application/json", []byte(errBody))
		}
		return &OpenAIForwardResult{
			RequestID:  syntheticID,
			ResponseID: syntheticID,
			Model:      state.Model,
			Duration:   time.Since(startTime),
		}, nil
	}

	if state.Status == "done" && strings.TrimSpace(state.UpstreamBody) != "" {
		respBody := rewriteAsyncGrokVideoStatusBody([]byte(state.UpstreamBody), state.UpstreamID, syntheticID, contentProxyURL)
		if c != nil {
			c.Data(http.StatusOK, "application/json", respBody)
		}
		if billed := ExtractGrokVideoBillingFromStatusBody(respBody, nil, syntheticID); billed != nil {
			billed.RequestID = syntheticID
			billed.ResponseID = syntheticID
			if strings.TrimSpace(billed.Model) == "" {
				billed.Model = state.Model
			}
			if strings.TrimSpace(billed.BillingModel) == "" {
				billed.BillingModel = billed.Model
			}
			billed.Duration = time.Since(startTime)
			return billed, nil
		}
	}

	// If upstream POST has returned an upstream request_id while still pending, poll upstream status live.
	if strings.TrimSpace(state.UpstreamID) != "" {
		proxyURL := ""
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}
		if liveBody, fetchErr := s.fetchAsyncUpstreamVideoStatusBody(ctx, account, token, proxyURL, state.UpstreamID); fetchErr == nil && len(liveBody) > 0 {
			rewritten := rewriteAsyncGrokVideoStatusBody(liveBody, state.UpstreamID, syntheticID, contentProxyURL)
			if billed := ExtractGrokVideoBillingFromStatusBody(rewritten, nil, syntheticID); billed != nil {
				state.Status = "done"
				state.UpstreamBody = string(rewritten)
				_ = s.storeGrokAsyncVideoJobState(ctx, *state)
				if c != nil {
					c.Data(http.StatusOK, "application/json", rewritten)
				}
				billed.RequestID = syntheticID
				billed.ResponseID = syntheticID
				if strings.TrimSpace(billed.Model) == "" {
					billed.Model = state.Model
				}
				if strings.TrimSpace(billed.BillingModel) == "" {
					billed.BillingModel = billed.Model
				}
				billed.Duration = time.Since(startTime)
				return billed, nil
			}
			if c != nil {
				c.Data(http.StatusOK, "application/json", rewritten)
			}
			return &OpenAIForwardResult{
				RequestID:  syntheticID,
				ResponseID: syntheticID,
				Model:      state.Model,
				Duration:   time.Since(startTime),
			}, nil
		}
	}

	pendingBody, _ := json.Marshal(map[string]any{
		"request_id": syntheticID,
		"status":     "pending",
	})
	if c != nil {
		c.Data(http.StatusOK, "application/json", pendingBody)
	}
	return &OpenAIForwardResult{
		RequestID:  syntheticID,
		ResponseID: syntheticID,
		Model:      state.Model,
		Duration:   time.Since(startTime),
	}, nil
}

func (s *OpenAIGatewayService) forwardAsyncGrokMediaVideoContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	syntheticID string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	state, err := s.getGrokAsyncVideoJobState(ctx, syntheticID)
	if err != nil {
		return nil, fmt.Errorf("read async video job state: %w", err)
	}
	if state == nil || strings.TrimSpace(state.UpstreamID) == "" {
		statusCode := http.StatusNotFound
		msg := "Video content is not ready or request not found"
		if state != nil && state.Status == "pending" {
			statusCode = http.StatusConflict
			msg = "Video generation is still in progress"
		}
		if c != nil {
			c.JSON(statusCode, gin.H{
				"error": gin.H{
					"message": msg,
					"type":    "invalid_request_error",
				},
			})
		}
		return &OpenAIForwardResult{
			RequestID:  syntheticID,
			ResponseID: syntheticID,
			Model:      "MiniMax-H3",
			Duration:   time.Since(startTime),
		}, nil
	}
	res, err := s.forwardGrokMediaVideoContent(ctx, c, account, token, state.UpstreamID, startTime)
	if err != nil {
		return nil, err
	}
	if res != nil {
		res.RequestID = syntheticID
		res.ResponseID = syntheticID
		if strings.TrimSpace(res.Model) == "" {
			res.Model = state.Model
		}
		if strings.TrimSpace(res.BillingModel) == "" {
			res.BillingModel = res.Model
		}
	}
	return res, nil
}
