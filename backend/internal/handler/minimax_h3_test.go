package handler

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type h3BrokenBody struct{}

func (h3BrokenBody) Read([]byte) (int, error) { return 0, errors.New("private reader failure") }
func (h3BrokenBody) Close() error             { return nil }

func TestMiniMaxH3CreateBodyBoundaries(t *testing.T) {
	// Reaching the group guard proves the upload was accepted, without billing,
	// account selection, network requests or a configured database.
	media := base64.StdEncoding.EncodeToString(make([]byte, 2<<20))
	largeValid := `{"model":"MiniMax-H3","prompt":"waves","task":"fl2va","conditions":[{"type":"image","role":"keyframe","frame_index":0,"data":"data:image/png;base64,` + media + `"}]}`
	for _, tc := range []struct {
		name       string
		body       string
		outerLimit int64
		broken     bool
		status     int
		message    string
	}{
		{"above_old_2MiB", largeValid, 0, false, http.StatusBadRequest, "MiniMax target group required"},
		{"over_12MiB", strings.Repeat(" ", service.H3MaxRequestBytes+1), 0, false, http.StatusRequestEntityTooLarge, "request body limit"},
		{"smaller_operator_cap", largeValid, 1024, false, http.StatusRequestEntityTooLarge, "1024 bytes"},
		{"malformed_json", "{", 0, false, http.StatusBadRequest, "non-empty JSON"},
		{"read_failure", "", 0, true, http.StatusBadRequest, "Unable to read H3 JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/minimax/videos", strings.NewReader(tc.body))
			if tc.broken {
				c.Request.Body = h3BrokenBody{}
			}
			if tc.outerLimit > 0 {
				c.Request.Body = http.MaxBytesReader(rec, c.Request.Body, tc.outerLimit)
			}
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{})
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})
			h := &OpenAIGatewayHandler{h3: &service.H3Service{}}
			h.MiniMaxH3Create(c)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.message) {
				t.Fatalf("status=%d response=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "private reader failure") {
				t.Fatal("reader detail leaked")
			}
			_ = c.Request.Body.Close()
		})
	}
}

var _ io.ReadCloser = h3BrokenBody{}
