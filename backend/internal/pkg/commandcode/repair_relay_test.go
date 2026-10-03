package commandcode

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
)

// TestChatCompletionRepairsRejectedModelRoute is the regression guard for the
// production 403 leak: when the first upstream call rejects a bare model id
// ("anthropic:<model>"), generate resolves the catalog alias and retries. The
// intermediate 403 must never be relayed to ChatCompletion, and the caller must
// observe the repaired 200 stream instead.
func TestChatCompletionRepairsRejectedModelRoute(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/provider/v1/models" {
			mu.Lock()
			seen = append(seen, "CATALOG")
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"deepseek/deepseek-v4.1-flash"},{"id":"deepseek/deepseek-v4.1-flash-fast"}]}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		if strings.Contains(string(b), `"model":"deepseek/deepseek-v4.1-flash"`) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintln(w, `data: {"type":"text-delta","text":"repaired"}`)
			fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop"}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"success":false,"error":{"code":"FORBIDDEN","status":403,"message":"Model/provider not recognized: anthropic:deepseek-v4.1-flash"}}`)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.ChatCompletion(ctx, "account-a", []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hi"}]}`), true)
	if err != nil {
		t.Fatalf("ChatCompletion returned error instead of repaired stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected repaired 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading repaired stream: %v", err)
	}
	if !strings.Contains(string(body), "repaired") || !strings.Contains(string(body), "data: [DONE]") {
		t.Fatalf("repaired stream missing content: %s", body)
	}
	mu.Lock()
	defer mu.Unlock()
	var posts int
	for _, s := range seen {
		if s == "CATALOG" {
			return
		}
		if s == "/alpha/generate" {
			posts++
		}
	}
	if posts < 2 {
		t.Fatalf("expected catalog repair then retry, saw %v", seen)
	}
}

// TestChatCompletionRelaysTerminalError pins the error contract: a
// non-repairable upstream failure must reach the caller with its status,
// headers and body preserved.
func TestChatCompletionRelaysTerminalError(t *testing.T) {
	const upstreamBody = `{"error":{"message":"slow down"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "17")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, upstreamBody)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.ChatCompletion(ctx, "account-a", []byte(`{"model":"test/model","messages":[{"role":"user","content":"hi"}]}`), false)
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got != "17" {
		t.Fatalf("Retry-After = %q, want 17", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != upstreamBody {
		t.Fatalf("body = %q, want %q", body, upstreamBody)
	}
}
