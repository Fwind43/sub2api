package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
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
