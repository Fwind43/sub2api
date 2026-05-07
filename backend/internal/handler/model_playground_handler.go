package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelPlaygroundHandler provides authenticated playground proxy endpoints.
// Requests are intentionally sent through the existing gateway routes so normal
// API-key auth, channel selection, usage accounting, billing, and error logging
// still apply.
type ModelPlaygroundHandler struct {
	apiKeyService *service.APIKeyService
	taskRepo      *repository.ModelPlaygroundTaskRepository
	cfg           *config.Config
	client        *http.Client
	runningTasks  sync.Map
}

func NewModelPlaygroundHandler(apiKeyService *service.APIKeyService, taskRepo *repository.ModelPlaygroundTaskRepository, cfg *config.Config) *ModelPlaygroundHandler {
	return &ModelPlaygroundHandler{
		apiKeyService: apiKeyService,
		taskRepo:      taskRepo,
		cfg:           cfg,
		client:        &http.Client{Timeout: 10 * time.Minute},
	}
}

type playgroundKeyRef struct {
	APIKeyID *int64 `json:"api_key_id"`
	// Optional manual key for ad-hoc tests. Prefer api_key_id so the browser never
	// needs to hold a raw key beyond what the existing key-management UI exposes.
	APIKey string `json:"api_key"`
}

type playgroundImageParams struct {
	Size              string `json:"size"`
	Quality           string `json:"quality"`
	OutputFormat      string `json:"output_format"`
	OutputCompression *int   `json:"output_compression"`
	Moderation        string `json:"moderation"`
	N                 int    `json:"n"`
}

type playgroundImageRequest struct {
	playgroundKeyRef
	APIMode        string                `json:"api_mode"`
	Model          string                `json:"model"`
	Prompt         string                `json:"prompt"`
	Params         playgroundImageParams `json:"params"`
	InputImages    []string              `json:"input_images"`
	CodexCLI       bool                  `json:"codex_cli"`
	TimeoutSeconds int                   `json:"timeout_seconds"`
}

type playgroundTaskListResponse struct {
	Items []repository.ModelPlaygroundTask `json:"items"`
}

func (h *ModelPlaygroundHandler) Images(c *gin.Context) {
	var req playgroundImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Model == "" {
		response.BadRequest(c, "model is required")
		return
	}
	if req.Prompt == "" {
		response.BadRequest(c, "prompt is required")
		return
	}
	if len(req.InputImages) > 16 {
		response.BadRequest(c, "input_images supports at most 16 images")
		return
	}

	apiKey, ok := h.resolveAPIKey(c, req.playgroundKeyRef)
	if !ok {
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.APIMode))
	if mode == "" {
		mode = "images"
	}
	var payload map[string]any
	var success bool
	ctx, cancel := playgroundTimeoutContext(c.Request.Context(), req.TimeoutSeconds)
	defer cancel()
	switch mode {
	case "responses":
		payload, success = h.proxyResponsesImage(ctx, apiKey, req)
	case "images":
		payload, success = h.proxyImagesAPI(ctx, apiKey, req)
	default:
		response.BadRequest(c, "api_mode must be images or responses")
		return
	}
	if !success {
		response.BadRequest(c, gatewayErrorMessage(payload, "playground request failed"))
		return
	}
	response.Success(c, payload)
}

func (h *ModelPlaygroundHandler) CreateImageTask(c *gin.Context) {
	var req playgroundImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Model == "" {
		response.BadRequest(c, "model is required")
		return
	}
	if req.Prompt == "" {
		response.BadRequest(c, "prompt is required")
		return
	}
	if len(req.InputImages) > 16 {
		response.BadRequest(c, "input_images supports at most 16 images")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	apiKey, apiKeyID, ok := h.resolveAPIKeyForUser(c, subject.UserID, req.playgroundKeyRef)
	if !ok {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.APIMode))
	if mode == "" {
		mode = "images"
	}
	if mode != "images" && mode != "responses" {
		response.BadRequest(c, "api_mode must be images or responses")
		return
	}
	params := normalizedImageParams(req.Params)
	task := &repository.ModelPlaygroundTask{
		ID:          newPlaygroundTaskID(),
		UserID:      subject.UserID,
		APIKeyID:    apiKeyID,
		APIMode:     mode,
		Model:       req.Model,
		Prompt:      req.Prompt,
		Params:      imageParamsToMap(params),
		InputImages: req.InputImages,
		CodexCLI:    req.CodexCLI,
		Status:      "queued",
		CreatedAt:   time.Now(),
	}
	if err := h.taskRepo.Create(c.Request.Context(), task); err != nil {
		response.InternalError(c, "failed to create image task")
		return
	}
	go h.runImageTask(task.ID, apiKey, req.TimeoutSeconds)
	response.Accepted(c, task)
}

func (h *ModelPlaygroundHandler) ListImageTasks(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.taskRepo.ListByUser(c.Request.Context(), subject.UserID, c.Query("status"), c.Query("search"), limit, offset)
	if err != nil {
		response.InternalError(c, "failed to list image tasks")
		return
	}
	for i := range items {
		h.resumeImageTaskIfNeeded(&items[i])
	}
	response.Success(c, playgroundTaskListResponse{Items: items})
}

func (h *ModelPlaygroundHandler) GetImageTask(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	task, err := h.taskRepo.GetByIDForUser(c.Request.Context(), subject.UserID, c.Param("id"))
	if err == sql.ErrNoRows {
		response.NotFound(c, "task not found")
		return
	}
	if err != nil {
		response.InternalError(c, "failed to get image task")
		return
	}
	h.resumeImageTaskIfNeeded(task)
	response.Success(c, task)
}

func (h *ModelPlaygroundHandler) DeleteImageTask(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if err := h.taskRepo.DeleteForUser(c.Request.Context(), subject.UserID, c.Param("id")); err != nil {
		response.InternalError(c, "failed to delete image task")
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *ModelPlaygroundHandler) resolveAPIKey(c *gin.Context, ref playgroundKeyRef) (string, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return "", false
	}
	apiKey, _, ok := h.resolveAPIKeyForUser(c, subject.UserID, ref)
	return apiKey, ok
}

func (h *ModelPlaygroundHandler) resolveAPIKeyForUser(c *gin.Context, userID int64, ref playgroundKeyRef) (string, *int64, bool) {
	if ref.APIKeyID != nil && *ref.APIKeyID > 0 {
		key, err := h.apiKeyService.GetByID(c.Request.Context(), *ref.APIKeyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return "", nil, false
		}
		if key.UserID != userID {
			response.Forbidden(c, "Not authorized to use this key")
			return "", nil, false
		}
		if key.Status != service.StatusAPIKeyActive && key.Status != service.StatusActive {
			response.BadRequest(c, "API key is not active")
			return "", nil, false
		}
		id := *ref.APIKeyID
		return key.Key, &id, true
	}

	manual := strings.TrimSpace(ref.APIKey)
	if manual == "" {
		response.BadRequest(c, "api_key_id or api_key is required")
		return "", nil, false
	}
	return manual, nil, true
}

func (h *ModelPlaygroundHandler) runImageTask(taskID, immediateAPIKey string, timeoutSeconds int) {
	if _, loaded := h.runningTasks.LoadOrStore(taskID, struct{}{}); loaded {
		return
	}
	defer h.runningTasks.Delete(taskID)
	ctx := context.Background()
	task, err := h.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return
	}
	started := time.Now()
	if err := h.taskRepo.MarkRunning(ctx, taskID); err != nil {
		return
	}
	apiKey := strings.TrimSpace(immediateAPIKey)
	if apiKey == "" && task.APIKeyID != nil {
		key, err := h.apiKeyService.GetByID(ctx, *task.APIKeyID)
		if err != nil || key.UserID != task.UserID || (key.Status != service.StatusAPIKeyActive && key.Status != service.StatusActive) {
			_ = h.taskRepo.Fail(ctx, taskID, "API key is unavailable", int(time.Since(started).Milliseconds()))
			return
		}
		apiKey = key.Key
	}
	if apiKey == "" {
		_ = h.taskRepo.Fail(ctx, taskID, "API key is unavailable", int(time.Since(started).Milliseconds()))
		return
	}

	req := taskToImageRequest(task, timeoutSeconds)
	callCtx, cancel := playgroundTimeoutContext(ctx, timeoutSeconds)
	defer cancel()
	outputs, revised, actual, raw, err := h.generateTaskImages(callCtx, apiKey, req)
	elapsed := int(time.Since(started).Milliseconds())
	if err != nil {
		_ = h.taskRepo.Fail(ctx, taskID, err.Error(), elapsed)
		return
	}
	if len(outputs) == 0 {
		_ = h.taskRepo.Fail(ctx, taskID, "接口已返回，但未解析到图片", elapsed)
		return
	}
	_ = h.taskRepo.Complete(ctx, taskID, outputs, revised, actual, raw, elapsed)
}

func (h *ModelPlaygroundHandler) resumeImageTaskIfNeeded(task *repository.ModelPlaygroundTask) {
	if task == nil || task.APIKeyID == nil {
		return
	}
	if task.Status != "queued" && task.Status != "running" {
		return
	}
	go h.runImageTask(task.ID, "", 300)
}

func (h *ModelPlaygroundHandler) generateTaskImages(ctx context.Context, apiKey string, req playgroundImageRequest) ([]string, map[string]string, map[string]any, map[string]any, error) {
	params := normalizedImageParams(req.Params)
	count := params.N
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}
	shouldSplit := count > 1 && (strings.EqualFold(req.APIMode, "responses") || req.CodexCLI)
	var raws []map[string]any
	var outputs []string
	revised := map[string]string{}
	actual := map[string]any{}
	runs := 1
	if shouldSplit {
		runs = count
	}
	for i := 0; i < runs; i++ {
		single := req
		if shouldSplit {
			single.Params.N = 1
		}
		var payload map[string]any
		var ok bool
		if strings.EqualFold(single.APIMode, "responses") {
			payload, ok = h.proxyResponsesImage(ctx, apiKey, single)
		} else {
			payload, ok = h.proxyImagesAPI(ctx, apiKey, single)
		}
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("%s", gatewayErrorMessage(payload, "playground request failed"))
		}
		raws = append(raws, payload)
		images := parseGeneratedImages(payload, single.APIMode, single.Params.OutputFormat)
		for _, image := range images {
			outputs = append(outputs, image.Src)
			if image.RevisedPrompt != "" {
				revised[image.Src] = image.RevisedPrompt
			}
			for k, v := range image.ActualParams {
				actual[k] = v
			}
		}
	}
	raw := mergeTaskRawPayloads(raws, req.APIMode)
	if len(outputs) > 0 {
		actual["n"] = len(outputs)
	}
	return outputs, revised, actual, raw, nil
}

func (h *ModelPlaygroundHandler) proxyImagesAPI(ctx context.Context, apiKey string, req playgroundImageRequest) (map[string]any, bool) {
	if len(req.InputImages) > 0 {
		return h.proxyImagesEdit(ctx, apiKey, req)
	}

	body := h.imagesGenerationBody(req, false)
	return h.proxyJSON(ctx, apiKey, "/v1/images/generations", body)
}

func (h *ModelPlaygroundHandler) proxyImagesEdit(ctx context.Context, apiKey string, req playgroundImageRequest) (map[string]any, bool) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range h.imagesStringFields(req) {
		if err := writer.WriteField(key, value); err != nil {
			return h.errorMap("failed to encode multipart field")
		}
	}
	for i, dataURL := range req.InputImages {
		mediaType, data, err := decodeDataURL(dataURL)
		if err != nil {
			return h.errorMap(fmt.Sprintf("invalid input image %d: %v", i+1, err))
		}
		ext := extensionForMediaType(mediaType)
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="input-%d.%s"`, i+1, ext))
		header.Set("Content-Type", mediaType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return h.errorMap("failed to create multipart image")
		}
		if _, err := part.Write(data); err != nil {
			return h.errorMap("failed to write multipart image")
		}
	}
	if err := writer.Close(); err != nil {
		return h.errorMap("failed to finalize multipart request")
	}
	return h.proxyBody(ctx, apiKey, "/v1/images/edits", writer.FormDataContentType(), &buf)
}

func (h *ModelPlaygroundHandler) proxyResponsesImage(ctx context.Context, apiKey string, req playgroundImageRequest) (map[string]any, bool) {
	body := h.responsesImageBody(req)
	return h.proxyJSON(ctx, apiKey, "/v1/responses", body)
}

func (h *ModelPlaygroundHandler) imagesGenerationBody(req playgroundImageRequest, forceSingle bool) map[string]any {
	body := map[string]any{
		"model":  req.Model,
		"prompt": codexPrompt(req.Prompt, req.CodexCLI),
	}
	params := normalizedImageParams(req.Params)
	if params.Size != "" {
		body["size"] = params.Size
	}
	if params.Quality != "" && params.Quality != "auto" && !req.CodexCLI {
		body["quality"] = params.Quality
	}
	if params.OutputFormat != "" {
		body["output_format"] = params.OutputFormat
	}
	if params.OutputCompression != nil && params.OutputFormat != "png" {
		body["output_compression"] = *params.OutputCompression
	}
	if params.Moderation != "" {
		body["moderation"] = params.Moderation
	}
	n := params.N
	if forceSingle || n < 1 {
		n = 1
	}
	body["n"] = n
	return body
}

func (h *ModelPlaygroundHandler) imagesStringFields(req playgroundImageRequest) map[string]string {
	params := normalizedImageParams(req.Params)
	fields := map[string]string{
		"model":  req.Model,
		"prompt": codexPrompt(req.Prompt, req.CodexCLI),
	}
	if params.Size != "" {
		fields["size"] = params.Size
	}
	if params.Quality != "" && params.Quality != "auto" && !req.CodexCLI {
		fields["quality"] = params.Quality
	}
	if params.OutputFormat != "" {
		fields["output_format"] = params.OutputFormat
	}
	if params.OutputCompression != nil && params.OutputFormat != "png" {
		fields["output_compression"] = strconv.Itoa(*params.OutputCompression)
	}
	if params.Moderation != "" {
		fields["moderation"] = params.Moderation
	}
	return fields
}

func (h *ModelPlaygroundHandler) responsesImageBody(req playgroundImageRequest) map[string]any {
	params := normalizedImageParams(req.Params)
	action := "generate"
	if len(req.InputImages) > 0 {
		action = "edit"
	}
	tool := map[string]any{
		"type":          "image_generation",
		"action":        action,
		"size":          params.Size,
		"output_format": params.OutputFormat,
	}
	if params.Quality != "" && params.Quality != "auto" && !req.CodexCLI {
		tool["quality"] = params.Quality
	}
	if params.OutputCompression != nil && params.OutputFormat != "png" {
		tool["output_compression"] = *params.OutputCompression
	}

	body := map[string]any{
		"model":       req.Model,
		"tools":       []any{tool},
		"tool_choice": "required",
	}

	prompt := codexPrompt(req.Prompt, req.CodexCLI)
	if len(req.InputImages) == 0 {
		body["input"] = prompt
		return body
	}
	content := make([]map[string]any, 0, len(req.InputImages)+1)
	content = append(content, map[string]any{"type": "input_text", "text": prompt})
	for _, image := range req.InputImages {
		content = append(content, map[string]any{"type": "input_image", "image_url": image})
	}
	body["input"] = []map[string]any{{"role": "user", "content": content}}
	return body
}

func normalizedImageParams(params playgroundImageParams) playgroundImageParams {
	params.Size = strings.TrimSpace(params.Size)
	if params.Size == "" {
		params.Size = "auto"
	}
	params.Quality = strings.ToLower(strings.TrimSpace(params.Quality))
	if params.Quality == "" {
		params.Quality = "auto"
	}
	params.OutputFormat = strings.ToLower(strings.TrimSpace(params.OutputFormat))
	if params.OutputFormat == "" {
		params.OutputFormat = "png"
	}
	params.Moderation = strings.ToLower(strings.TrimSpace(params.Moderation))
	if params.Moderation == "" {
		params.Moderation = "auto"
	}
	if params.N < 1 {
		params.N = 1
	}
	if params.N > 10 {
		params.N = 10
	}
	return params
}

func codexPrompt(prompt string, enabled bool) string {
	if !enabled {
		return prompt
	}
	return "Use the following text as the complete prompt. Do not rewrite it:\n" + prompt
}

func playgroundTimeoutContext(ctx context.Context, seconds int) (context.Context, context.CancelFunc) {
	if seconds <= 0 {
		seconds = 300
	}
	if seconds > 900 {
		seconds = 900
	}
	return context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
}

func (h *ModelPlaygroundHandler) proxyJSON(ctx context.Context, apiKey, path string, body map[string]any) (map[string]any, bool) {
	data, err := json.Marshal(body)
	if err != nil {
		return h.errorMap("failed to encode request")
	}
	return h.proxyBody(ctx, apiKey, path, "application/json", bytes.NewReader(data))
}

func (h *ModelPlaygroundHandler) proxyBody(ctx context.Context, apiKey, path, contentType string, body io.Reader) (map[string]any, bool) {
	url := h.gatewayURL(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return h.errorMap(err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Pragma", "no-cache")

	resp, err := h.client.Do(req)
	if err != nil {
		return h.errorMap("gateway request failed: " + err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return h.errorMap("failed to read gateway response")
	}
	var payload map[string]any
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &payload); err != nil {
			payload = map[string]any{"raw": string(respBody)}
		}
	} else {
		payload = map[string]any{}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := gatewayErrorMessage(payload, resp.Status)
		return map[string]any{"error": message}, false
	}
	return payload, true
}

func (h *ModelPlaygroundHandler) errorMap(message string) (map[string]any, bool) {
	return map[string]any{"error": message}, false
}

func gatewayErrorMessage(payload map[string]any, fallback string) string {
	if payload == nil {
		return fallback
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok && msg != "" {
			return msg
		}
	}
	if msg, ok := payload["error"].(string); ok && msg != "" {
		return msg
	}
	if msg, ok := payload["message"].(string); ok && msg != "" {
		return msg
	}
	if raw, ok := payload["raw"].(string); ok && raw != "" {
		return raw
	}
	return fallback
}

func (h *ModelPlaygroundHandler) gatewayURL(path string) string {
	host := strings.TrimSpace(h.cfg.Server.Host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(h.cfg.Server.Port))
	if strings.HasPrefix(host, "[") {
		addr = host + ":" + strconv.Itoa(h.cfg.Server.Port)
	}
	return "http://" + addr + path
}

func decodeDataURL(value string) (string, []byte, error) {
	if !strings.HasPrefix(value, "data:") {
		return "", nil, fmt.Errorf("must be a data URL")
	}
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return "", nil, fmt.Errorf("missing data URL payload")
	}
	header := value[5:comma]
	payload := value[comma+1:]
	parts := strings.Split(header, ";")
	mediaType := strings.TrimSpace(parts[0])
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
			break
		}
	}
	if !isBase64 {
		return "", nil, fmt.Errorf("only base64 data URLs are supported")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, err
	}
	return mediaType, data, nil
}

func extensionForMediaType(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return "png"
	}
}

func newPlaygroundTaskID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("img-%d", time.Now().UnixNano())
	}
	return "img-" + hex.EncodeToString(buf[:])
}

func imageParamsToMap(params playgroundImageParams) map[string]any {
	out := map[string]any{
		"size":          params.Size,
		"quality":       params.Quality,
		"output_format": params.OutputFormat,
		"moderation":    params.Moderation,
		"n":             params.N,
	}
	if params.OutputCompression != nil {
		out["output_compression"] = *params.OutputCompression
	} else {
		out["output_compression"] = nil
	}
	return out
}

func taskToImageRequest(task *repository.ModelPlaygroundTask, timeoutSeconds int) playgroundImageRequest {
	params := playgroundImageParams{
		Size:         stringFromMap(task.Params, "size", "auto"),
		Quality:      stringFromMap(task.Params, "quality", "auto"),
		OutputFormat: stringFromMap(task.Params, "output_format", "png"),
		Moderation:   stringFromMap(task.Params, "moderation", "auto"),
		N:            intFromMap(task.Params, "n", 1),
	}
	if value, ok := intPtrFromMap(task.Params, "output_compression"); ok {
		params.OutputCompression = value
	}
	return playgroundImageRequest{
		APIMode:        task.APIMode,
		Model:          task.Model,
		Prompt:         task.Prompt,
		Params:         params,
		InputImages:    task.InputImages,
		CodexCLI:       task.CodexCLI,
		TimeoutSeconds: timeoutSeconds,
	}
}

func stringFromMap(values map[string]any, key, fallback string) string {
	if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func intFromMap(values map[string]any, key string, fallback int) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		if i, err := value.Int64(); err == nil {
			return int(i)
		}
	}
	return fallback
}

func intPtrFromMap(values map[string]any, key string) (*int, bool) {
	if values[key] == nil {
		return nil, false
	}
	v := intFromMap(values, key, 0)
	return &v, true
}

type parsedGeneratedImage struct {
	Src           string
	RevisedPrompt string
	ActualParams  map[string]any
}

func parseGeneratedImages(raw map[string]any, apiMode, fallbackFormat string) []parsedGeneratedImage {
	if strings.EqualFold(apiMode, "responses") {
		return parseResponsesGeneratedImages(raw, fallbackFormat)
	}
	return parseImagesGeneratedImages(raw, fallbackFormat)
}

func parseImagesGeneratedImages(raw map[string]any, fallbackFormat string) []parsedGeneratedImage {
	data, _ := raw["data"].([]any)
	images := make([]parsedGeneratedImage, 0, len(data))
	for _, entry := range data {
		item, _ := entry.(map[string]any)
		if item == nil {
			continue
		}
		format := firstNonEmptyString(
			stringAny(item["output_format"]),
			stringAny(nestedAny(item, "actual_params", "output_format")),
			stringAny(raw["output_format"]),
			fallbackFormat,
			"png",
		)
		src := stringAny(item["url"])
		if b64 := stringAny(item["b64_json"]); b64 != "" {
			src = "data:" + mimeForOutputFormat(format) + ";base64," + b64
		}
		if src == "" {
			continue
		}
		images = append(images, parsedGeneratedImage{
			Src:           src,
			RevisedPrompt: stringAny(item["revised_prompt"]),
			ActualParams:  pickActualParams(raw, item),
		})
	}
	return images
}

func parseResponsesGeneratedImages(raw map[string]any, fallbackFormat string) []parsedGeneratedImage {
	output, _ := raw["output"].([]any)
	images := make([]parsedGeneratedImage, 0, len(output))
	for _, entry := range output {
		item, _ := entry.(map[string]any)
		if item == nil || stringAny(item["type"]) != "image_generation_call" {
			continue
		}
		result := item["result"]
		b64 := stringAny(result)
		if b64 == "" {
			if m, ok := result.(map[string]any); ok {
				b64 = firstNonEmptyString(stringAny(m["b64_json"]), stringAny(m["image"]), stringAny(m["data"]))
			}
		}
		if b64 == "" {
			b64 = firstNonEmptyString(stringAny(item["image"]), stringAny(item["b64_json"]))
		}
		if b64 == "" {
			continue
		}
		format := firstNonEmptyString(stringAny(item["output_format"]), stringAny(nestedAny(item, "actual_params", "output_format")), fallbackFormat, "png")
		src := b64
		if !strings.HasPrefix(src, "data:") && !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
			src = "data:" + mimeForOutputFormat(format) + ";base64," + src
		}
		images = append(images, parsedGeneratedImage{
			Src:           src,
			RevisedPrompt: stringAny(item["revised_prompt"]),
			ActualParams:  pickActualParams(item),
		})
	}
	return images
}

func pickActualParams(maps ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range maps {
		for _, key := range []string{"size", "quality", "output_format", "output_compression", "moderation", "n"} {
			if v, ok := m[key]; ok && v != nil {
				out[key] = v
			}
		}
		if nested, ok := m["actual_params"].(map[string]any); ok {
			for k, v := range nested {
				out[k] = v
			}
		}
	}
	return out
}

func nestedAny(m map[string]any, keys ...string) any {
	var cur any = m
	for _, key := range keys {
		next, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = next[key]
	}
	return cur
}

func stringAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mimeForOutputFormat(format string) string {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func mergeTaskRawPayloads(raws []map[string]any, apiMode string) map[string]any {
	if len(raws) == 0 {
		return nil
	}
	if len(raws) == 1 {
		return raws[0]
	}
	first := map[string]any{}
	for k, v := range raws[0] {
		first[k] = v
	}
	if strings.EqualFold(apiMode, "responses") {
		var output []any
		for _, raw := range raws {
			if items, ok := raw["output"].([]any); ok {
				output = append(output, items...)
			}
		}
		first["output"] = output
	} else {
		var data []any
		for _, raw := range raws {
			if items, ok := raw["data"].([]any); ok {
				data = append(data, items...)
			}
		}
		first["data"] = data
	}
	first["playground_parallel_raw"] = raws
	return first
}
