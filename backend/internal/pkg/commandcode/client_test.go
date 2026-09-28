package commandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestIndependentCredentialsAndGOProtocol(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alpha/generate" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
		}
		mu.Lock()
		seen[r.Header.Get("Authorization")]++
		mu.Unlock()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		params, ok := body["params"].(map[string]any)
		if !ok || params["model"] != "test/model" {
			t.Errorf("unexpected params: %#v", params)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"type":"text-delta","text":"hello"}`)
		fmt.Fprintln(w, `data: {"type":"text-delta","text":" world"}`)
		fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop","usage":{"inputTokens":3,"outputTokens":2}}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	var wg sync.WaitGroup
	for _, key := range []string{"account-a", "account-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			var delta strings.Builder
			out, err := client.Generate(context.Background(), key, "test/model", Request{Messages: []map[string]any{{"role": "user", "content": "hello"}}, MaxTokens: 64}, func(s string) error { delta.WriteString(s); return nil })
			if err != nil {
				t.Error(err)
				return
			}
			if out.Text != "hello world" || delta.String() != out.Text || out.FinishReason != "stop" {
				t.Errorf("unexpected result %#v delta %q", out, delta.String())
			}
		}(key)
	}
	wg.Wait()
	if seen["Bearer account-a"] != 1 || seen["Bearer account-b"] != 1 || len(seen) != 2 {
		t.Fatalf("credential isolation failed: %#v", seen)
	}
}

func TestToolCallsAndUpstreamErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer rejected" {
			http.Error(w, "quota exhausted", 429)
			return
		}
		fmt.Fprintln(w, `data: {"type":"tool-call","toolCallId":"call-1","toolName":"weather","input":{"city":"Paris"}}`)
		fmt.Fprintln(w, `data: {"type":"finish","finishReason":"tool_calls"}`)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	req := Request{Messages: []map[string]any{{"role": "user", "content": "weather?"}}, MaxTokens: 64}
	out, err := client.Generate(context.Background(), "valid", "test/model", req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ToolCalls) != 1 || out.FinishReason != "tool_calls" {
		t.Fatalf("unexpected tool result %#v", out)
	}
	_, err = client.Generate(context.Background(), "rejected", "test/model", req, nil)
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("missing upstream failure: %v", err)
	}
	_, err = client.Generate(context.Background(), "", "test/model", req, nil)
	if err == nil {
		t.Fatal("missing credential accepted")
	}
}
