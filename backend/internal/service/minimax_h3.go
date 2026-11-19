package service

// H3 has its own durable task state machine and transport. Shared OpenAI usage
// recording is used only as the pricing/ledger adapter after binary verification.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

const H3Model = "MiniMax-H3"
const OpenAIEndpointCapabilityMiniMaxH3 OpenAIEndpointCapability = "minimax_h3"
const h3MaxVideo = 128 << 20

// The upstream now only needs to accept the payload, not render the video.
const h3SubmissionTimeout = 2 * time.Minute
const h3SubmissionSaveTimeout = 10 * time.Second
const h3SubmissionLeaseSeconds = int((h3SubmissionTimeout + time.Minute) / time.Second)
const h3SubmitPath = "/videos/generations"

var ErrH3NotFound = errors.New("H3 task not found")
var ErrH3Conflict = errors.New("idempotency key belongs to a different request")

// H3CreateError identifies the failed lifecycle stage without exposing request
// bodies, credentials, upstream URLs, or database error messages in logs.
// Error and Unwrap preserve the original error and errors.Is/errors.As behavior.
type H3CreateError struct {
	Stage string
	cause error
}

func (e *H3CreateError) Error() string { return e.cause.Error() }
func (e *H3CreateError) Unwrap() error { return e.cause }

type H3Snapshot struct {
	Cost                  *CostBreakdown    `json:"cost"`
	Multiplier            float64           `json:"multiplier"`
	AccountRateMultiplier float64           `json:"account_rate_multiplier"`
	Group                 *Group            `json:"group"`
	Subscription          *UserSubscription `json:"subscription,omitempty"`
	Resolution            string            `json:"resolution"`
	Seconds               int               `json:"seconds"`
	PricingAt             time.Time         `json:"pricing_at"`
}
type H3Task struct {
	ID         string     `json:"id"`
	UserID     int64      `json:"-"`
	APIKeyID   int64      `json:"-"`
	GroupID    int64      `json:"-"`
	AccountID  int64      `json:"-"`
	UpstreamID string     `json:"-"`
	Generation string     `json:"status"`
	Delivery   string     `json:"delivery_status"`
	Billing    string     `json:"billing_status"`
	Snapshot   H3Snapshot `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	lease      string
	hash       string
}
type H3Service struct {
	db      *sql.DB
	gateway *OpenAIGatewayService
	keys    *APIKeyService
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex // Serializes worker registration with Stop.
	stopped bool
}

func NewH3Service(db *sql.DB, gateway *OpenAIGatewayService, keys *APIKeyService) *H3Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &H3Service{db: db, gateway: gateway, keys: keys, ctx: ctx, cancel: cancel}
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.run(ctx) }()
	return s
}
func (s *H3Service) Stop() {
	s.mu.Lock()
	s.stopped = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// ParseH3Request validates both the frozen billing inputs and GeToken's native
// reference contract before account selection or any upstream submission.
func ParseH3Request(body []byte) (H3Snapshot, error) {
	snapshot, _, err := parseH3Request(body)
	return snapshot, err
}

func parseH3Request(body []byte) (H3Snapshot, string, error) {
	if len(body) > H3MaxRequestBytes {
		return H3Snapshot{}, "", errors.New("H3 JSON must be at most 12 MiB")
	}
	snapshot, err := parseH3Snapshot(body)
	if err != nil {
		return H3Snapshot{}, "", err
	}
	task, err := validateH3References(body)
	return snapshot, task, err
}

// Only the exact video model is accepted; images have an independent endpoint.
func parseH3Snapshot(body []byte) (H3Snapshot, error) {
	var v struct {
		Model      string `json:"model"`
		Prompt     string `json:"prompt"`
		Duration   int    `json:"duration"`
		Seconds    int    `json:"seconds"`
		Resolution string `json:"resolution"`
		N          int    `json:"n"`
	}
	if json.Unmarshal(body, &v) != nil || v.Model != H3Model || strings.TrimSpace(v.Prompt) == "" {
		return H3Snapshot{}, errors.New("model must be MiniMax-H3 and prompt must be non-empty JSON")
	}
	if v.N > 1 || v.N < 0 {
		return H3Snapshot{}, errors.New("H3 supports one video per task")
	}
	if v.Duration == 0 {
		v.Duration = v.Seconds
	}
	if v.Duration == 0 {
		v.Duration = 6
	}
	if v.Duration < 4 || v.Duration > 15 {
		return H3Snapshot{}, errors.New("duration must be 4-15 seconds")
	}
	if v.Seconds != 0 && v.Seconds != v.Duration {
		return H3Snapshot{}, errors.New("conflicting duration and seconds")
	}
	if v.Resolution == "" {
		v.Resolution = "720p"
	}
	if v.Resolution != "720p" && v.Resolution != "1080p" && v.Resolution != "480p" && v.Resolution != "768p" {
		return H3Snapshot{}, errors.New("unsupported resolution")
	}
	return H3Snapshot{Resolution: v.Resolution, Seconds: v.Duration, PricingAt: time.Now().UTC()}, nil
}

// prepareH3Request translates gateway compatibility fields without changing the
// original body used for idempotency. H3 always creates one video and rejects n.
func prepareH3Request(body []byte) (H3Snapshot, []byte, error) {
	snap, task, err := parseH3Request(body)
	if err != nil {
		return H3Snapshot{}, nil, err
	}
	var payload map[string]json.RawMessage
	if err = json.Unmarshal(body, &payload); err != nil {
		return H3Snapshot{}, nil, err
	}
	payload["duration"], _ = json.Marshal(snap.Seconds)
	payload["resolution"], _ = json.Marshal(snap.Resolution)
	payload["task"], _ = json.Marshal(task)
	delete(payload, "seconds")
	delete(payload, "n")
	normalized, err := json.Marshal(payload)
	return snap, normalized, err
}

const h3Columns = "id,user_id,api_key_id,group_id,account_id,upstream_id,generation,delivery,billing,snapshot,created_at,lease_token,payload_hash"

type h3Scanner interface{ Scan(...any) error }

func scanH3(row h3Scanner) (*H3Task, error) {
	t := &H3Task{}
	var raw []byte
	err := row.Scan(&t.ID, &t.UserID, &t.APIKeyID, &t.GroupID, &t.AccountID, &t.UpstreamID, &t.Generation, &t.Delivery, &t.Billing, &raw, &t.CreatedAt, &t.lease, &t.hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrH3NotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &t.Snapshot); err != nil {
		return nil, err
	}
	return t, nil
}
func (s *H3Service) Get(ctx context.Context, id string, key *APIKey) (*H3Task, error) {
	if key == nil || key.GroupID == nil {
		return nil, ErrH3NotFound
	}
	return scanH3(s.db.QueryRowContext(ctx, "SELECT "+h3Columns+" FROM minimax_h3_tasks WHERE id=$1 AND user_id=$2 AND api_key_id=$3 AND group_id=$4", id, key.UserID, key.ID, *key.GroupID))
}
func (s *H3Service) Create(ctx context.Context, key *APIKey, sub *UserSubscription, body []byte, idem string) (task *H3Task, createErr error) {
	stage := "dependencies"
	defer func() {
		if createErr != nil {
			createErr = &H3CreateError{Stage: stage, cause: createErr}
		}
	}()
	if s.db == nil || s.gateway == nil || s.gateway.usageBillingRepo == nil || s.keys == nil ||
		s.gateway.cfg == nil || s.gateway.cfg.RunMode == config.RunModeSimple {
		return nil, errors.New("H3 requires durable billing and storage before submission")
	}
	stage = "request_validation"
	snap, normalized, err := prepareH3Request(body)
	if err != nil {
		return nil, err
	}
	stage = "group_validation"
	if key == nil || key.GroupID == nil || key.Group == nil {
		return nil, errors.New("a MiniMax group is required")
	}
	platform := key.Group.Platform
	if platform == PlatformComposite {
		platform, _ = ResolvedTargetPlatformFromContext(ctx)
	}
	if platform != PlatformMiniMax {
		return nil, errors.New("a MiniMax target group is required")
	}
	stage = "idempotency_validation"
	if len(idem) > 200 {
		return nil, errors.New("idempotency key too long")
	}
	if idem == "" {
		idem = uuid.NewString()
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	stage = "idempotency_lookup"
	old, err := scanH3(s.db.QueryRowContext(ctx, "SELECT "+h3Columns+" FROM minimax_h3_tasks WHERE api_key_id=$1 AND idempotency_key=$2", key.ID, idem))
	if err == nil {
		if old.hash != hash || old.UserID != key.UserID || old.GroupID != *key.GroupID {
			return nil, ErrH3Conflict
		}
		return old, nil
	}
	if !errors.Is(err, ErrH3NotFound) {
		return nil, err
	}
	stage = "account_selection"
	selected, _, err := s.gateway.SelectAccountWithSchedulerForCapability(ctx, key.GroupID, "", "", H3Model, nil, OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityMiniMaxH3, false, false, false, PlatformMiniMax)
	if err != nil {
		return nil, err
	}
	stage = "account_availability"
	if selected == nil || selected.Account == nil {
		return nil, errors.New("no H3 account available")
	}
	release := selected.ReleaseFunc
	defer func() {
		if release != nil {
			release()
		}
	}()
	stage = "account_acquisition"
	if !selected.Acquired {
		return nil, errors.New("H3 account busy")
	}
	account := selected.Account
	stage = "account_capability"
	if !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityMiniMaxH3) {
		return nil, errors.New("H3 scheduler capability mismatch")
	}
	stage = "endpoint_validation"
	if _, err = s.request(ctx, account, http.MethodGet, "", nil, true); err != nil {
		return nil, err
	}
	stage = "pricing"
	baseMultiplier := s.gateway.ResolveUserGroupRateMultiplier(ctx, key.UserID, *key.GroupID, key.Group.RateMultiplier)
	snap.Multiplier = resolveVideoRateMultiplier(key, baseMultiplier)
	snap.AccountRateMultiplier = account.BillingRateMultiplier()
	snap.Cost = s.gateway.calculateOpenAIVideoCost(ctx, H3Model, key, &OpenAIForwardResult{VideoCount: 1, VideoResolution: snap.Resolution, VideoDurationSeconds: snap.Seconds}, snap.Multiplier)
	if snap.Cost == nil || snap.Cost.ActualCost <= 0 {
		return nil, errors.New("H3 requires a positive configured video price")
	}
	snap.Group = key.Group
	snap.Subscription = sub
	stage = "snapshot_serialization"
	raw, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	id := "h3-" + uuid.NewString()
	stage = "task_insert"
	res, err := s.db.ExecContext(ctx, `INSERT INTO minimax_h3_tasks(id,user_id,api_key_id,group_id,account_id,snapshot,idempotency_key,payload_hash,lease_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now()+$9*interval '1 second') ON CONFLICT(api_key_id,idempotency_key) DO NOTHING`, id, key.UserID, key.ID, *key.GroupID, account.ID, string(raw), idem, hash, h3SubmissionLeaseSeconds)
	if err != nil {
		return nil, err
	}
	stage = "task_insert_result"
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		stage = "idempotency_reload"
		old, err = scanH3(s.db.QueryRowContext(ctx, "SELECT "+h3Columns+" FROM minimax_h3_tasks WHERE api_key_id=$1 AND idempotency_key=$2", key.ID, idem))
		if err == nil && (old.hash != hash || old.GroupID != *key.GroupID || old.UserID != key.UserID) {
			return nil, ErrH3Conflict
		}
		return old, err
	}
	// Transfer the account slot to the worker only after durable insertion.
	// The HTTP request returns without waiting for GPU startup/rendering.
	stage = "submission_start"
	if !s.startSubmission(ctx, id, account, normalized, release) {
		saveCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = s.db.ExecContext(saveCtx, "UPDATE minimax_h3_tasks SET generation='failed',billing='not_billable',lease_until=NULL,updated_at=now() WHERE id=$1 AND generation='submitting' AND upstream_id=''", id)
		return nil, errors.New("H3 service is stopping; submission not sent")
	}
	release = nil
	stage = "task_reload"
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return s.Get(readCtx, id, key)
}

// startSubmission registers before Stop can start waiting. Incoming request
// cancellation must not lose a paid submission, but service shutdown cancels it.
func (s *H3Service) startSubmission(ctx context.Context, id string, account *Account, body []byte, release func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return false
	}
	submitCtx, cancel := context.WithTimeout(WithHTTPUpstreamProfile(context.WithoutCancel(ctx), HTTPUpstreamProfileLongVideo), h3SubmissionTimeout)
	stopCancel := func() bool { return false }
	if s.ctx != nil {
		stopCancel = context.AfterFunc(s.ctx, cancel)
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer stopCancel()
		if release != nil {
			defer release()
		}
		s.submit(submitCtx, id, account, body)
	}()
	return true
}

func (s *H3Service) submit(ctx context.Context, id string, account *Account, body []byte) {
	// Never retry this POST automatically. A lost response is still ambiguous.
	start := time.Now()
	var diag h3RequestDiagnostics
	response, submitErr := s.requestWithDiagnostics(ctx, account, http.MethodPost, h3SubmitPath, body, false, &diag, h3UpstreamIdempotencyKey(id))
	upstream, state, billing := h3SubmissionOutcome(response, submitErr)
	if submitErr == nil && state == "submission_unknown" {
		diag.failure = "invalid_receipt"
	}
	// The receipt ID is distinct from HTTP tracing IDs. Keep it available for
	// reconciliation even when the database is temporarily unavailable.
	log.Printf("H3 submission task=%s state=%s account_id=%d upstream_http_status=%d failure=%q elapsed_ms=%d upstream_task_id=%q upstream_request_id=%q upstream_cf_ray=%q upstream_error=%q",
		id, state, account.ID, diag.status, diag.failure, time.Since(start).Milliseconds(), upstream, diag.requestID, diag.cfRay, diag.upstreamError)
	s.saveSubmissionOutcome(id, upstream, state, billing)
}

func h3UpstreamIdempotencyKey(id string) string {
	return "sub2api-h3-" + id
}

func (s *H3Service) saveSubmissionOutcome(id, upstream, state, billing string) {
	// The original submit context may have timed out or been canceled at shutdown.
	// A valid receipt can recover a task after its submission lease expired.
	// Clear the lease token so that a concurrent poll cannot overwrite it.
	// Retry only this conditional update, never the upstream generation request.
	const query = `UPDATE minimax_h3_tasks SET upstream_id=$2,generation=$3,billing=$4,lease_until=NULL,lease_token='',updated_at=now()
WHERE id=$1 AND upstream_id='' AND billing='pending' AND (generation='submitting' OR (generation='submission_unknown' AND $2<>''))`
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), h3SubmissionSaveTimeout)
		result, err := s.db.ExecContext(ctx, query, id, upstream, state, billing)
		cancel()
		if err == nil {
			if n, countErr := result.RowsAffected(); countErr == nil {
				if n == 1 || s.submissionOutcomeSaved(id, upstream, state, billing) {
					return
				}
				log.Printf("H3 submission task=%s checkpoint_conflict=true", id)
				return
			}
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 250 * time.Millisecond)
		}
	}
	log.Printf("H3 submission task=%s checkpoint_failed=true", id)
}

// An UPDATE can commit even when its acknowledgement is lost. Do not report a
// conflict if the result is already stored or a poll has advanced that receipt.
func (s *H3Service) submissionOutcomeSaved(id, upstream, state, billing string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), h3SubmissionSaveTimeout)
	defer cancel()
	var storedUpstream, storedState, storedBilling string
	if err := s.db.QueryRowContext(ctx, "SELECT upstream_id,generation,billing FROM minimax_h3_tasks WHERE id=$1", id).Scan(&storedUpstream, &storedState, &storedBilling); err != nil {
		return false
	}
	return storedUpstream == upstream && (upstream != "" || (storedState == state && storedBilling == billing))
}

type h3UpstreamHTTPError struct {
	StatusCode int
}

func (e *h3UpstreamHTTPError) Error() string {
	return fmt.Sprintf("H3 upstream HTTP %d", e.StatusCode)
}

// Explicit client rejections cannot be billed. Timeouts, server errors, and
// malformed successful replies remain ambiguous and must never be resubmitted.
func h3SubmissionOutcome(response []byte, err error) (upstream, generation, billing string) {
	if err != nil {
		var httpErr *h3UpstreamHTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.StatusCode {
			case 400, 401, 403, 404, 405, 413, 415, 422, 429:
				return "", "failed", "not_billable"
			}
		}
		return "", "submission_unknown", "pending"
	}
	var v struct {
		ID        string `json:"id"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(response, &v) == nil {
		upstream = v.RequestID
		if upstream == "" {
			upstream = v.ID
		}
		if validH3UpstreamID(upstream) {
			return upstream, "pending", "pending"
		}
	}
	return "", "submission_unknown", "pending"
}

func validH3UpstreamID(id string) bool {
	return len(id) > 0 && len(id) < 256 && !strings.ContainsAny(id, "/\\?#\x00\r\n")
}

type h3RequestDiagnostics struct {
	status                    int
	failure, requestID, cfRay string
	// upstreamError is a short, sanitized excerpt of a non-2xx response body
	// (for example the reason behind a 422) so rejections can be diagnosed.
	upstreamError string
}

const (
	h3UpstreamErrorReadLimit = 4 << 10
	h3UpstreamErrorMaxRunes  = 300
)

// h3UpstreamErrorSummary extracts a readable reason from an error body.
// Common JSON shapes ({"error":"..."}, {"error":{"message":"..."}},
// {"message":"..."}, {"detail":"..."}) are reduced to their message; other
// bodies are kept as text. Control characters are flattened and the result is
// truncated so a large or hostile body cannot flood the log.
func h3UpstreamErrorSummary(body []byte) string {
	text := strings.TrimSpace(string(body))
	var v map[string]any
	if json.Unmarshal(body, &v) == nil {
		pick := func(m map[string]any, keys ...string) string {
			for _, k := range keys {
				if str, ok := m[k].(string); ok && strings.TrimSpace(str) != "" {
					return str
				}
			}
			return ""
		}
		if msg := pick(v, "error", "message", "detail", "error_description"); msg != "" {
			text = msg
		} else if inner, ok := v["error"].(map[string]any); ok {
			if msg := pick(inner, "message", "detail", "code", "type"); msg != "" {
				text = msg
			}
		}
	}
	text = strings.Map(func(r rune) rune {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) > h3UpstreamErrorMaxRunes {
		runes := []rune(text)
		text = string(runes[:h3UpstreamErrorMaxRunes]) + "..."
	}
	return text
}

// Accept only short, opaque trace tokens. A URL, free-form text, or a list of
// header values is not a trace ID and should not appear in the submission log.
func h3TraceID(value string) string {
	if len(value) > 128 {
		return ""
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return ""
		}
	}
	return value
}

func h3RequestFailure(phase string, err error) string {
	var timeout net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return phase + "_canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout():
		return phase + "_timeout"
	default:
		return phase + "_error"
	}
}

func (s *H3Service) request(ctx context.Context, a *Account, method, path string, body []byte, validateOnly bool) ([]byte, error) {
	return s.requestWithDiagnostics(ctx, a, method, path, body, validateOnly, nil)
}

// Only operator-configured endpoints are contacted. Provider-supplied URLs are
// never followed and authorization is never sent to a video URL from JSON.
func (s *H3Service) requestWithDiagnostics(ctx context.Context, a *Account, method, path string, body []byte, validateOnly bool, diag *h3RequestDiagnostics, idempotencyKey ...string) ([]byte, error) {
	if diag != nil {
		*diag = h3RequestDiagnostics{failure: "request_setup_error"}
	}
	base := strings.TrimRight(strings.TrimSpace(a.GetCredential("base_url")), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("H3 requires an explicit HTTPS base_url")
	}
	token := a.GetCredential("api_key")
	if token == "" {
		return nil, errors.New("H3 requires API key credentials")
	}
	if validateOnly {
		if diag != nil {
			diag.failure = ""
		}
		return nil, nil
	}
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), method, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if len(idempotencyKey) > 0 && idempotencyKey[0] != "" {
		req.Header.Set("Prefer", "respond-async")
		req.Header.Set("Idempotency-Key", idempotencyKey[0])
		// net/http otherwise treats the keyed POST with a rewindable body as
		// replayable. Keep ambiguous submissions available for reconciliation.
		req.GetBody = nil
	}
	proxy := ""
	if a.Proxy != nil {
		proxy = a.Proxy.URL()
	}
	resp, err := s.gateway.httpUpstream.Do(req, proxy, a.ID, a.Concurrency)
	if err != nil {
		if diag != nil {
			diag.failure = h3RequestFailure("transport", err)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if diag != nil {
		diag.status = resp.StatusCode
		diag.requestID = h3TraceID(resp.Header.Get("X-Request-ID"))
		diag.cfRay = h3TraceID(resp.Header.Get("CF-Ray"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if diag != nil {
			diag.failure = "upstream_http"
			if errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, h3UpstreamErrorReadLimit)); readErr == nil || len(errBody) > 0 {
				diag.upstreamError = h3UpstreamErrorSummary(errBody)
			}
		}
		return nil, &h3UpstreamHTTPError{StatusCode: resp.StatusCode}
	}
	limit := int64(2 << 20)
	if strings.HasSuffix(path, "/content") {
		limit = h3MaxVideo
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		if diag != nil {
			diag.failure = h3RequestFailure("response_body", err)
		}
		return nil, err
	}
	if int64(len(data)) > limit {
		if diag != nil {
			diag.failure = "response_too_large"
		}
		return nil, errors.New("H3 response exceeds limit")
	}
	if diag != nil {
		diag.failure = ""
	}
	return data, nil
}

// Require MP4 structure and a media payload, not just MIME or completed JSON.
func verifyH3Video(data []byte) bool {
	if len(data) < 1024 {
		return false
	}
	ftyp, moov, mdat := false, false, false
	for off := 0; off+8 <= len(data); {
		size := uint64(data[off])<<24 | uint64(data[off+1])<<16 | uint64(data[off+2])<<8 | uint64(data[off+3])
		header := uint64(8)
		if size == 1 {
			if off+16 > len(data) {
				return false
			}
			size = 0
			for _, b := range data[off+8 : off+16] {
				size = size<<8 | uint64(b)
			}
			header = 16
		}
		if size == 0 {
			size = uint64(len(data) - off)
		}
		if size < header || size > uint64(len(data)-off) {
			return false
		}
		switch string(data[off+4 : off+8]) {
		case "ftyp":
			ftyp = true
		case "moov":
			moov = size > header+8
		case "mdat":
			mdat = size > header+512
		}
		off += int(size)
		if off == len(data) {
			return ftyp && moov && mdat
		}
	}
	return false
}
func (s *H3Service) Content(ctx context.Context, t *H3Task) ([]byte, error) {
	if t.Generation != "completed" || t.Delivery != "verified" || t.Billing != "settled" {
		return nil, errors.New("H3 video is not ready")
	}
	a, err := s.gateway.accountRepo.GetByID(ctx, t.AccountID)
	if err != nil {
		return nil, err
	}
	data, err := s.request(ctx, a, http.MethodGet, "/videos/"+url.PathEscape(t.UpstreamID)+"/content", nil, false)
	if err != nil {
		return nil, err
	}
	if !verifyH3Video(data) {
		return nil, errors.New("H3 returned invalid video")
	}
	return data, nil
}
func (s *H3Service) run(ctx context.Context) {
	timer := time.NewTicker(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.poll(ctx)
		}
	}
}
func (s *H3Service) poll(ctx context.Context) {
	token := uuid.NewString()
	t, err := scanH3(s.db.QueryRowContext(ctx, `UPDATE minimax_h3_tasks SET lease_until=now()+interval '3 minutes',lease_token=$1 WHERE id=(SELECT id FROM minimax_h3_tasks WHERE billing='pending' AND generation IN ('submitting','pending','completed') AND next_poll_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY next_poll_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING `+h3Columns, token))
	if errors.Is(err, ErrH3NotFound) {
		return
	}
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("H3 claim failed: %v", err)
		}
		return
	}
	s.processClaimed(ctx, t, token)
}

// processClaimed advances a leased task once and releases its lease.
func (s *H3Service) processClaimed(ctx context.Context, t *H3Task, token string) {
	work, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	var err error
	if t.Generation == "submitting" {
		t.Generation = "submission_unknown"
	} else {
		err = s.advance(work, t)
	}
	if err != nil {
		log.Printf("H3 task %s retry: %v", t.ID, err)
	}
	save, cancelSave := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSave()
	_, saveErr := s.db.ExecContext(save, `UPDATE minimax_h3_tasks SET generation=$2,delivery=$3,billing=$4,lease_until=NULL,lease_token='',next_poll_at=now()+interval '20 seconds',updated_at=now() WHERE id=$1 AND lease_token=$5`, t.ID, t.Generation, t.Delivery, t.Billing, token)
	if saveErr != nil {
		log.Printf("H3 task %s checkpoint failed: %v", t.ID, saveErr)
	}
}
func (s *H3Service) advance(ctx context.Context, t *H3Task) error {
	a, err := s.gateway.accountRepo.GetByID(ctx, t.AccountID)
	if err != nil {
		return err
	}
	if a.Platform != "minimax" {
		return errors.New("pinned H3 account platform changed")
	}
	if t.Generation == "pending" {
		data, err := s.request(ctx, a, http.MethodGet, "/videos/"+url.PathEscape(t.UpstreamID), nil, false)
		if err != nil {
			return err
		}
		var v struct {
			Status string `json:"status"`
		}
		if err = json.Unmarshal(data, &v); err != nil {
			return err
		}
		switch strings.ToLower(v.Status) {
		case "failed", "cancelled", "canceled", "expired":
			t.Generation = "failed"
			t.Billing = "not_billable"
			return nil
		case "completed", "succeeded", "done":
			t.Generation = "completed"
		default:
			return nil
		}
	}
	data, err := s.request(ctx, a, http.MethodGet, "/videos/"+url.PathEscape(t.UpstreamID)+"/content", nil, false)
	if err != nil {
		return err
	}
	if !verifyH3Video(data) {
		return errors.New("completed H3 task has no verified MP4")
	}
	t.Delivery = "verified"
	key, err := s.keys.GetByID(ctx, t.APIKeyID)
	if err != nil {
		return err
	}
	if key == nil || key.UserID != t.UserID || key.GroupID == nil || *key.GroupID != t.GroupID ||
		t.Snapshot.Group == nil || t.Snapshot.Group.ID != t.GroupID {
		return errors.New("H3 billing identity changed; operator reconciliation required")
	}
	keyCopy := *key
	keyCopy.Group = t.Snapshot.Group
	keyCopy.GroupID = &t.GroupID
	user, err := s.gateway.userRepo.GetByID(ctx, t.UserID)
	if err != nil {
		return err
	}
	if t.Snapshot.Cost == nil {
		return errors.New("H3 frozen cost missing; operator reconciliation required")
	}
	err = s.gateway.RecordUsage(ctx, &OpenAIRecordUsageInput{h3Cost: t.Snapshot.Cost, h3Multiplier: t.Snapshot.Multiplier, h3AccountRateMultiplier: t.Snapshot.AccountRateMultiplier, APIKey: &keyCopy, User: user, Account: a, Subscription: t.Snapshot.Subscription, PricingAt: t.Snapshot.PricingAt, APIKeyService: s.keys, QuotaPlatform: "minimax", RequestPayloadHash: t.hash, InboundEndpoint: "/v1/minimax/videos", UpstreamEndpoint: "/v1/videos/generations", Result: &OpenAIForwardResult{RequestID: "minimax-h3:" + t.ID, ResponseID: t.ID, Model: H3Model, BillingModel: H3Model, VideoCount: 1, VideoResolution: t.Snapshot.Resolution, VideoDurationSeconds: t.Snapshot.Seconds}})
	if err != nil {
		return err
	}
	t.Billing = "settled"
	return nil
}
