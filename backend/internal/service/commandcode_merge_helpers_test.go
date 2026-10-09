//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// Merge helper shims.
//
// Upstream v0.2.15 shipped its own Command Code implementation together with test
// fixtures (backend/internal/service/command_code_test.go and
// command_code_usage_test.go). This fork keeps its own Command Code / ClinePass
// implementation under the fork platform ids (commandcode / clinepass), so those
// upstream implementation + fixture files were dropped during the merge.
//
// The fixtures below are the pieces that upstream test files which we DID keep
// (model_protocol_catalog_test.go, opencode_unsupported_models_routing_test.go)
// still depend on. They are copied verbatim from upstream v0.2.15, with the only
// change being that the account uses the fork platform constant value.

// commandCodeTestAccount builds a Command Code account carrying only an api_key:
// endpoints and protocol routing come entirely from the provider profile.
func commandCodeTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "cc",
		Platform:    PlatformCommandCode,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "user_test_key"},
	}
}

// commandCodeAlphaUpstream serves canned responses keyed by request path and
// records every request it received.
type commandCodeAlphaUpstream struct {
	mu        sync.Mutex
	responses map[string]commandCodeAlphaResponse
	requests  []*http.Request
}

type commandCodeAlphaResponse struct {
	status int
	body   string
}

func (u *commandCodeAlphaUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.requests = append(u.requests, req)
	u.mu.Unlock()
	resp, ok := u.responses[req.URL.Path]
	if !ok {
		resp = commandCodeAlphaResponse{status: http.StatusNotFound, body: `{"error":"not found"}`}
	}
	return &http.Response{
		StatusCode: resp.status,
		Body:       io.NopCloser(strings.NewReader(resp.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (u *commandCodeAlphaUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}
