package commandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatCompletionAdapter(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/alpha/generate" || r.Header.Get("Authorization") != "Bearer account-a" {
					t.Errorf("wrong endpoint or authentication")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintln(w, `data: {"type":"text-delta","text":" hello "}`)
				fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop","totalUsage":{"inputTokens":3,"outputTokens":2}}`)
			}))
			defer server.Close()
			client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			resp, err := client.ChatCompletion(ctx, "account-a", []byte(`{"model":"test/model","messages":[{"role":"user","content":"hi"}]}`), stream)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status=%d body=%s", resp.StatusCode, body)
			}
			if stream {
				if !strings.Contains(string(body), `"content":" hello "`) || !strings.Contains(string(body), `"total_tokens":5`) || !strings.Contains(string(body), "data: [DONE]") {
					t.Fatalf("bad SSE: %s", body)
				}
			} else {
				var result struct {
					Choices []struct {
						Message      struct{ Content string }
						FinishReason string `json:"finish_reason"`
					}
					Usage struct {
						Total int `json:"total_tokens"`
					}
				}
				if err = json.Unmarshal(body, &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Choices) != 1 || result.Choices[0].Message.Content != " hello " || result.Choices[0].FinishReason != "stop" || result.Usage.Total != 5 {
					t.Fatalf("bad JSON: %s", body)
				}
			}
		})
	}
}

func TestChatCompletionPreservesUpstreamErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":"quota exhausted"}`)
	}))
	defer server.Close()
	for _, stream := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
		resp, err := client.ChatCompletion(ctx, "account-a", []byte(`{"model":"test/model","messages":[{"role":"user","content":"hi"}]}`), stream)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		b, e := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if e != nil || resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "17" || string(b) != `{"error":"quota exhausted"}` {
			t.Fatalf("status/header/body lost: %v %s", e, b)
		}
	}
}

func TestChatCompletionToolContinuation(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				if strings.Contains(string(b), `"type":"tool-result"`) {
					if !strings.Contains(string(b), `"toolCallId":"call-1"`) || !strings.Contains(string(b), `"toolName":"lookup"`) || !strings.Contains(string(b), "verified-result") {
						t.Error("tool result mapping lost")
					}
					fmt.Fprintln(w, `data: {"type":"text-delta","text":"continued"}`)
					fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop"}`)
				} else {
					fmt.Fprintln(w, `data: {"type":"tool-call","toolCallId":"call-1","toolName":"lookup","input":{"q":"x"}}`)
					fmt.Fprintln(w, `data: {"type":"finish","finishReason":"tool-calls"}`)
				}
			}))
			defer server.Close()
			client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
			inputs := []string{
				`{"model":"test/model","messages":[{"role":"user","content":"lookup"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`,
				`{"model":"test/model","messages":[{"role":"user","content":"lookup"},{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},{"role":"tool","tool_call_id":"call-1","content":"verified-result"}]}`,
			}
			for i, input := range inputs {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				resp, err := client.ChatCompletion(ctx, "account-a", []byte(input), stream)
				if err != nil {
					cancel()
					t.Fatal(err)
				}
				b, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 && (!strings.Contains(string(b), `"finish_reason":"tool_calls"`) || !strings.Contains(string(b), `"id":"call-1"`)) {
					t.Fatalf("lost tool call: %s", b)
				}
				if i == 1 && !strings.Contains(string(b), "continued") {
					t.Fatalf("failed continuation: %s", b)
				}
			}
		})
	}
}

func TestChatCompletionRejectsTruncatedStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `data: {"type":"text-delta","text":"partial"}`)
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := client.ChatCompletion(ctx, "account-a", []byte(`{"model":"test/model","messages":[{"role":"user","content":"hi"}]}`), true)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err == nil || strings.Contains(string(b), "data: [DONE]") {
		t.Fatalf("truncated stream reported success: err=%v body=%s", err, b)
	}
}
