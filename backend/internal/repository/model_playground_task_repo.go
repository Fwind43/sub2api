package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	modelPlaygroundTaskTTL       = 7 * 24 * time.Hour
	modelPlaygroundTaskKeyPrefix = "model_playground:task:"
	modelPlaygroundUserKeyPrefix = "model_playground:user:"
	modelPlaygroundListChunkSize = 200
)

type ModelPlaygroundTask struct {
	ID                   string            `json:"id"`
	UserID               int64             `json:"user_id"`
	APIKeyID             *int64            `json:"api_key_id,omitempty"`
	APIMode              string            `json:"api_mode"`
	Model                string            `json:"model"`
	Prompt               string            `json:"prompt"`
	Params               map[string]any    `json:"params"`
	InputImages          []string          `json:"input_images"`
	OutputImages         []string          `json:"output_images"`
	InputImageCount      int               `json:"input_image_count,omitempty"`
	OutputImageCount     int               `json:"output_image_count,omitempty"`
	RevisedPromptByImage map[string]string `json:"revised_prompt_by_image,omitempty"`
	ActualParams         map[string]any    `json:"actual_params,omitempty"`
	RawResponse          map[string]any    `json:"raw_response,omitempty"`
	CodexCLI             bool              `json:"codex_cli"`
	Status               string            `json:"status"`
	Error                *string           `json:"error,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	StartedAt            *time.Time        `json:"started_at,omitempty"`
	FinishedAt           *time.Time        `json:"finished_at,omitempty"`
	ElapsedMS            *int              `json:"elapsed_ms,omitempty"`
}

// ModelPlaygroundTaskRepository keeps image playground tasks in Redis only.
//
// Image payloads can be very large Base64 data URLs. Keeping them in the SQL
// database makes the task-list endpoint slow and leaves long-lived blobs behind.
// Redis keys are therefore the source of truth for playground tasks, with a hard
// 7-day TTL from task creation. Once Redis expires the key, the task is gone.
type ModelPlaygroundTaskRepository struct {
	rdb *redis.Client
}

func NewModelPlaygroundTaskRepository(rdb *redis.Client) *ModelPlaygroundTaskRepository {
	return &ModelPlaygroundTaskRepository{rdb: rdb}
}

func (r *ModelPlaygroundTaskRepository) Create(ctx context.Context, task *ModelPlaygroundTask) error {
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	normalizeModelPlaygroundTask(task)
	return r.save(ctx, task)
}

func (r *ModelPlaygroundTaskRepository) GetByID(ctx context.Context, id string) (*ModelPlaygroundTask, error) {
	if err := r.ensureRedis(); err != nil {
		return nil, err
	}
	val, err := r.rdb.Get(ctx, modelPlaygroundTaskKey(id)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	task, err := decodeModelPlaygroundTask(val)
	if err != nil {
		return nil, err
	}
	if taskExpired(task) {
		_ = r.deleteTask(ctx, task.UserID, task.ID)
		return nil, sql.ErrNoRows
	}
	return task, nil
}

func (r *ModelPlaygroundTaskRepository) GetByIDForUser(ctx context.Context, userID int64, id string) (*ModelPlaygroundTask, error) {
	task, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.UserID != userID {
		return nil, sql.ErrNoRows
	}
	return task, nil
}

func (r *ModelPlaygroundTaskRepository) ListByUser(ctx context.Context, userID int64, status, search string, limit, offset int) ([]ModelPlaygroundTask, error) {
	if err := r.ensureRedis(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	ids, err := r.rdb.ZRevRange(ctx, modelPlaygroundUserKey(userID), 0, -1).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []ModelPlaygroundTask{}, nil
		}
		return nil, err
	}

	status = strings.TrimSpace(status)
	search = strings.ToLower(strings.TrimSpace(search))
	items := make([]ModelPlaygroundTask, 0, limit)
	skipped := 0
	staleIDs := make([]any, 0)

	for start := 0; start < len(ids) && len(items) < limit; start += modelPlaygroundListChunkSize {
		end := start + modelPlaygroundListChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		keys := make([]string, 0, end-start)
		for _, id := range ids[start:end] {
			keys = append(keys, modelPlaygroundTaskKey(id))
		}
		values, err := r.rdb.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, err
		}
		for i, raw := range values {
			id := ids[start+i]
			if raw == nil {
				staleIDs = append(staleIDs, id)
				continue
			}
			task, err := decodeModelPlaygroundTaskValue(raw)
			if err != nil || taskExpired(task) || task.UserID != userID {
				staleIDs = append(staleIDs, id)
				continue
			}
			if status != "" && status != "all" && task.Status != status {
				continue
			}
			if search != "" && !strings.Contains(strings.ToLower(task.Prompt), search) && !strings.Contains(strings.ToLower(task.Model), search) {
				continue
			}
			if skipped < offset {
				skipped++
				continue
			}
			items = append(items, summarizeModelPlaygroundTask(*task))
			if len(items) >= limit {
				break
			}
		}
	}

	if len(staleIDs) > 0 {
		_ = r.rdb.ZRem(ctx, modelPlaygroundUserKey(userID), staleIDs...).Err()
	}
	return items, nil
}

func (r *ModelPlaygroundTaskRepository) MarkRunning(ctx context.Context, id string) error {
	task, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now()
	task.Status = "running"
	task.StartedAt = &now
	task.Error = nil
	return r.save(ctx, task)
}

func (r *ModelPlaygroundTaskRepository) Complete(ctx context.Context, id string, outputImages []string, revised map[string]string, actualParams map[string]any, raw map[string]any, elapsedMS int) error {
	task, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now()
	task.Status = "done"
	task.OutputImages = defaultSlice(outputImages)
	task.OutputImageCount = len(task.OutputImages)
	task.RevisedPromptByImage = defaultStringMap(revised)
	task.ActualParams = defaultMap(actualParams)
	task.RawResponse = defaultMap(raw)
	task.FinishedAt = &now
	task.ElapsedMS = &elapsedMS
	task.Error = nil
	return r.save(ctx, task)
}

func (r *ModelPlaygroundTaskRepository) Fail(ctx context.Context, id, message string, elapsedMS int) error {
	task, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now()
	task.Status = "error"
	task.Error = &message
	task.FinishedAt = &now
	task.ElapsedMS = &elapsedMS
	return r.save(ctx, task)
}

func (r *ModelPlaygroundTaskRepository) DeleteForUser(ctx context.Context, userID int64, id string) error {
	task, err := r.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if task.UserID != userID {
		return nil
	}
	return r.deleteTask(ctx, userID, id)
}

func (r *ModelPlaygroundTaskRepository) save(ctx context.Context, task *ModelPlaygroundTask) error {
	if err := r.ensureRedis(); err != nil {
		return err
	}
	normalizeModelPlaygroundTask(task)
	ttl := time.Until(task.CreatedAt.Add(modelPlaygroundTaskTTL))
	if ttl <= 0 {
		_ = r.deleteTask(ctx, task.UserID, task.ID)
		return sql.ErrNoRows
	}
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal model playground task: %w", err)
	}
	userKey := modelPlaygroundUserKey(task.UserID)
	pipe := r.rdb.Pipeline()
	pipe.Set(ctx, modelPlaygroundTaskKey(task.ID), data, ttl)
	pipe.ZAdd(ctx, userKey, redis.Z{Score: float64(task.CreatedAt.UnixMilli()), Member: task.ID})
	pipe.Expire(ctx, userKey, modelPlaygroundTaskTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *ModelPlaygroundTaskRepository) deleteTask(ctx context.Context, userID int64, id string) error {
	if err := r.ensureRedis(); err != nil {
		return err
	}
	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, modelPlaygroundTaskKey(id))
	pipe.ZRem(ctx, modelPlaygroundUserKey(userID), id)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *ModelPlaygroundTaskRepository) ensureRedis() error {
	if r == nil || r.rdb == nil {
		return errors.New("model playground task cache is unavailable")
	}
	return nil
}

func modelPlaygroundTaskKey(id string) string {
	return modelPlaygroundTaskKeyPrefix + id
}

func modelPlaygroundUserKey(userID int64) string {
	return fmt.Sprintf("%s%d:tasks", modelPlaygroundUserKeyPrefix, userID)
}

func decodeModelPlaygroundTaskValue(raw any) (*ModelPlaygroundTask, error) {
	switch value := raw.(type) {
	case string:
		return decodeModelPlaygroundTask([]byte(value))
	case []byte:
		return decodeModelPlaygroundTask(value)
	default:
		return nil, fmt.Errorf("unexpected model playground task cache value %T", raw)
	}
}

func decodeModelPlaygroundTask(data []byte) (*ModelPlaygroundTask, error) {
	var task ModelPlaygroundTask
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("unmarshal model playground task: %w", err)
	}
	normalizeModelPlaygroundTask(&task)
	return &task, nil
}

func normalizeModelPlaygroundTask(task *ModelPlaygroundTask) {
	if task == nil {
		return
	}
	task.Params = defaultMap(task.Params)
	task.InputImages = defaultSlice(task.InputImages)
	task.OutputImages = defaultSlice(task.OutputImages)
	task.InputImageCount = len(task.InputImages)
	task.OutputImageCount = len(task.OutputImages)
	task.RevisedPromptByImage = defaultStringMap(task.RevisedPromptByImage)
	if task.Status == "" {
		task.Status = "queued"
	}
}

func summarizeModelPlaygroundTask(task ModelPlaygroundTask) ModelPlaygroundTask {
	normalizeModelPlaygroundTask(&task)
	inputCount := task.InputImageCount
	outputCount := task.OutputImageCount

	task.InputImages = []string{}
	if len(task.OutputImages) > 1 {
		first := task.OutputImages[0]
		task.OutputImages = []string{first}
		if revised := task.RevisedPromptByImage[first]; revised != "" {
			task.RevisedPromptByImage = map[string]string{first: revised}
		} else {
			task.RevisedPromptByImage = map[string]string{}
		}
	}
	task.InputImageCount = inputCount
	task.OutputImageCount = outputCount
	task.RawResponse = nil
	return task
}

func taskExpired(task *ModelPlaygroundTask) bool {
	if task == nil || task.CreatedAt.IsZero() {
		return false
	}
	return time.Since(task.CreatedAt) >= modelPlaygroundTaskTTL
}

func defaultMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func defaultSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func defaultStringMap(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	return value
}
