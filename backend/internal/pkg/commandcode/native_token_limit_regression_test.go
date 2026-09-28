package commandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNativeTokenLimitRegression(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		tokens, complete, want int
	}{
		{"max_tokens", 37, 0, 37},
		{"max_completion_tokens", 0, 43, 43},
		{"completion_precedence", 37, 43, 43},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observed := make(chan int, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Params struct {
						MaxTokens int `json:"max_tokens"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					http.Error(w, "invalid request", 400)
					return
				}
				observed <- body.Params.MaxTokens
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintln(w, `data: {"type":"text-delta","text":"pong"}`)
				fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop"}`)
			}))
			defer server.Close()
			client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
			_, err := client.Generate(context.Background(), "fixture-key", "test/model", Request{Messages: []map[string]any{{"role": "user", "content": "ping"}}, MaxTokens: tc.tokens, MaxComplete: tc.complete}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := <-observed; got != tc.want {
				t.Fatalf("params.max_tokens=%d; want %d", got, tc.want)
			}
		})
	}
}
