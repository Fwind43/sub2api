package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestH3RejectsNonVideoPayloads(t *testing.T) {
	for _, body := range [][]byte{nil, []byte(`{"status":"completed","url":"https://example.org/video.mp4"}`), []byte("<html>not a video</html>"), make([]byte, 2048)} {
		if verifyH3Video(body) {
			t.Fatal("accepted a non-video payload")
		}
	}
}
func TestH3RequestContract(t *testing.T) {
	good := []byte(`{"model":"MiniMax-H3","prompt":"waves","duration":6}`)
	s, err := ParseH3Request(good)
	if err != nil || s.Seconds != 6 || s.Resolution != "720p" {
		t.Fatalf("invalid defaults: %+v %v", s, err)
	}
	preserved, err := ParseH3Request([]byte(`{"model":"MiniMax-H3","prompt":"waves","duration":6,"resolution":"768p"}`))
	if err != nil || preserved.Resolution != "768p" {
		t.Fatalf("768p was not preserved: %+v %v", preserved, err)
	}
	for _, body := range []string{
		`{"model":"grok-video","prompt":"waves"}`,
		`{"model":"MiniMax-H3","prompt":"waves","duration":16}`,
		`{"model":"MiniMax-H3","prompt":"waves","duration":6,"seconds":8}`,
		`{"model":"MiniMax-H3","prompt":"waves","n":2}`,
	} {
		if _, err := ParseH3Request([]byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
func TestH3UpstreamNormalization(t *testing.T) {
	body := []byte(`{"model":"MiniMax-H3","prompt":"look back and smile","seconds":5,"resolution":"768p","n":1}`)
	before := string(body)
	snap, normalized, err := prepareH3Request(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != before || snap.Seconds != 5 || snap.Resolution != "768p" {
		t.Fatalf("request or snapshot changed: %s %+v", body, snap)
	}
	var payload map[string]json.RawMessage
	if err = json.Unmarshal(normalized, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["n"]; ok {
		t.Fatal("upstream rejects n, including n=1")
	}
	if _, ok := payload["seconds"]; ok {
		t.Fatal("seconds alias was not removed")
	}
	if string(payload["duration"]) != "5" || string(payload["resolution"]) != `"768p"` || string(payload["prompt"]) != `"look back and smile"` {
		t.Fatalf("unexpected normalized payload: %s", normalized)
	}
	if _, _, err = prepareH3Request([]byte(`{"model":"MiniMax-H3","prompt":"waves","n":2}`)); err == nil {
		t.Fatal("normalization must not bypass count validation")
	}
}

func TestH3SubmissionOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, body, id, generation, billing string
		err                                 error
	}{
		{name: "accepted", body: `{"id":"job-1"}`, id: "job-1", generation: "pending", billing: "pending"},
		{name: "request_id", body: `{"request_id":"job-2"}`, id: "job-2", generation: "pending", billing: "pending"},
		{name: "rejected", err: &h3UpstreamHTTPError{400}, generation: "failed", billing: "not_billable"},
		{name: "auth", err: &h3UpstreamHTTPError{401}, generation: "failed", billing: "not_billable"},
		{name: "throttled", err: &h3UpstreamHTTPError{429}, generation: "failed", billing: "not_billable"},
		{name: "timeout", err: context.DeadlineExceeded, generation: "submission_unknown", billing: "pending"},
		{name: "server_error", err: &h3UpstreamHTTPError{502}, generation: "submission_unknown", billing: "pending"},
		{name: "request_timeout", err: &h3UpstreamHTTPError{408}, generation: "submission_unknown", billing: "pending"},
		{name: "conflict", err: &h3UpstreamHTTPError{409}, generation: "submission_unknown", billing: "pending"},
		{name: "malformed", body: `{`, generation: "submission_unknown", billing: "pending"},
		{name: "unsafe_id", body: `{"id":"../job"}`, generation: "submission_unknown", billing: "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, generation, billing := h3SubmissionOutcome([]byte(tc.body), tc.err)
			if id != tc.id || generation != tc.generation || billing != tc.billing {
				t.Fatalf("got %q %q %q", id, generation, billing)
			}
		})
	}
}

func TestH3HTTPRejectionIsTypedAndSanitized(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":"secret-provider-detail"}`))}}}
	s := &H3Service{gateway: &OpenAIGatewayService{httpUpstream: upstream}}
	a := &Account{ID: 1, Credentials: map[string]any{"base_url": "https://example.org", "api_key": "test-secret"}}
	_, err := s.request(context.Background(), a, http.MethodPost, "/videos/generations", []byte(`{}`), false)
	var httpErr *h3UpstreamHTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 400 || err.Error() != "H3 upstream HTTP 400" {
		t.Fatalf("unexpected error: %v", err)
	}
	_, generation, billing := h3SubmissionOutcome(nil, err)
	if generation != "failed" || billing != "not_billable" {
		t.Fatalf("explicit rejection was not terminal: %s %s", generation, billing)
	}
}

func TestH3UpstreamIDBoundary(t *testing.T) {
	for _, id := range []string{"", "../task", "a?token=x", "a/b", "a\n"} {
		if validH3UpstreamID(id) {
			t.Fatalf("accepted %q", id)
		}
	}
	if !validH3UpstreamID("task-123") {
		t.Fatal("rejected safe ID")
	}
}

// Embedding satisfies the unused TLS method; only the dedicated H3 transport
// is exercised, with no external network or paid generation.
type h3BlockingTransport struct {
	HTTPUpstream
	entered chan *http.Request
	finish  chan struct{}
	calls   atomic.Int32
}

func (h *h3BlockingTransport) Do(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	h.calls.Add(1)
	h.entered <- r
	select {
	case <-h.finish:
		return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(`{"request_id":"job-async"}`))}, nil
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
}
func TestH3AsyncSubmissionLifecycle(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		name := "completion"
		if shutdown {
			name = "shutdown_unknown"
		}
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			upstreamID, generation := "job-async", "pending"
			if shutdown {
				upstreamID, generation = "", "submission_unknown"
			}
			query := "UPDATE minimax_h3_tasks SET upstream_id=$2,generation=$3,billing=$4,lease_until=NULL,updated_at=now() WHERE id=$1 AND generation='submitting' AND upstream_id=''"
			mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs("h3-test", upstreamID, generation, "pending").WillReturnResult(sqlmock.NewResult(0, 1))
			transport := &h3BlockingTransport{entered: make(chan *http.Request, 1), finish: make(chan struct{})}
			lifeCtx, cancelLife := context.WithCancel(context.Background())
			s := &H3Service{db: db, gateway: &OpenAIGatewayService{httpUpstream: transport}, ctx: lifeCtx, cancel: cancelLife}
			defer s.Stop()
			requestCtx, cancelRequest := context.WithCancel(context.Background())
			defer cancelRequest()
			var releases atomic.Int32
			account := &Account{ID: 1, Credentials: map[string]any{"base_url": "https://example.org", "api_key": "test-secret"}}
			started := make(chan bool, 1)
			go func() {
				started <- s.startSubmission(requestCtx, "h3-test", account, []byte(`{}`), func() { releases.Add(1) })
			}()
			select {
			case ok := <-started:
				if !ok {
					t.Fatal("worker not started")
				}
			case <-time.After(time.Second):
				t.Fatal("submission blocked HTTP return")
			}
			var req *http.Request
			select {
			case req = <-transport.entered:
			case <-time.After(time.Second):
				t.Fatal("worker did not send")
			}
			cancelRequest()
			if req.Context().Err() != nil {
				t.Fatal("incoming cancellation propagated")
			}
			if HTTPUpstreamProfileFromContext(req.Context()) != HTTPUpstreamProfileLongVideo {
				t.Fatal("missing long-video transport profile")
			}
			deadline, ok := req.Context().Deadline()
			if !ok || time.Until(deadline) < 29*time.Minute {
				t.Fatal("short submission deadline")
			}
			if time.Duration(h3SubmissionLeaseSeconds)*time.Second <= h3SubmissionTimeout+10*time.Second {
				t.Fatal("worker lease can expire before checkpoint")
			}
			if releases.Load() != 0 {
				t.Fatal("account slot released while submitting")
			}
			if shutdown {
				s.Stop()
			} else {
				close(transport.finish)
				s.wg.Wait()
			}
			if releases.Load() != 1 || transport.calls.Load() != 1 {
				t.Fatal("slot leak or repeated POST")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			s.Stop()
			if s.startSubmission(context.Background(), "h3-stopped", account, nil, nil) {
				t.Fatal("accepted worker after Stop")
			}
			if transport.calls.Load() != 1 {
				t.Fatal("shutdown caused another POST")
			}
		})
	}
}
