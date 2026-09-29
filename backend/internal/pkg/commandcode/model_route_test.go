package commandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectedModelRoute(t *testing.T) {
	for _, tc := range []struct {
		name, input, catalog, detail, target string
		success                              bool
		posts, gets                          int
	}{
		{"wrong_provider", "anthropic:deepseek-v4.1-flash", `{"data":[{"id":"fixture:deepseek-v4.1-flash"}]}`, "Model/provider not recognized:", "fixture:deepseek-v4.1-flash", true, 2, 1},
		{"bare", "deepseek-v4.1-flash", `{"models":[{"id":"fixture/deepseek-v4.1-flash"}]}`, "Model/provider not recognized:", "fixture/deepseek-v4.1-flash", true, 2, 1},
		{"ambiguous", "x:m", `{"data":[{"id":"a:m"},{"id":"b:m"}]}`, "Model/provider not recognized:", "", false, 1, 1},
		{"missing", "x:m", `{"data":[{"id":"a:other"}]}`, "Model/provider not recognized:", "", false, 1, 1},
		{"permission", "x:m", `{"data":[{"id":"a:m"}]}`, "Access forbidden", "", false, 1, 0},
		{"explicit_valid", "a:m", `{}`, "", "a:m", true, 1, 0},
		{"retry_once", "x:m", `{"data":[{"id":"a:m"}]}`, "Model/provider not recognized:", "", false, 2, 1},
		{"exact_not_redirected", "a:m", `{"data":[{"id":"a:m"},{"id":"b:m"}]}`, "Model/provider not recognized:", "", false, 1, 1},
		{"malformed", "x:m", `not json`, "Model/provider not recognized:", "", false, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts, gets := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("credential changed")
				}
				if r.URL.Path == "/provider/v1/models" {
					gets++
					if r.Method != "GET" {
						t.Error("catalog method")
					}
					fmt.Fprint(w, tc.catalog)
					return
				}
				if r.URL.Path != "/alpha/generate" {
					t.Error("unexpected path")
				}
				posts++
				var b struct {
					Params map[string]any `json:"params"`
				}
				json.NewDecoder(r.Body).Decode(&b)
				model, _ := b.Params["model"].(string)
				if b.Params["canonicalID"] != model {
					t.Error("canonicalID mismatch")
				}
				if tc.target != "" && model == tc.target {
					fmt.Fprintln(w, `data: {"type":"text-delta","text":"ok"}`)
					fmt.Fprintln(w, `data: {"type":"finish","finishReason":"stop"}`)
					return
				}
				w.WriteHeader(403)
				fmt.Fprint(w, tc.detail+" "+model)
			}))
			defer server.Close()
			c := Client{BaseURL: server.URL, HTTPClient: server.Client()}
			var output strings.Builder
			result, err := c.Generate(context.Background(), "fixture-key", tc.input, Request{Messages: []map[string]any{{"role": "user", "content": "hi"}}}, func(s string) error { output.WriteString(s); return nil })
			if (err == nil) != tc.success {
				t.Errorf("success=%v error=%v", tc.success, err)
			}
			if tc.success && (result.Text != "ok" || output.String() != "ok") {
				t.Errorf("output=%q callback=%q", result.Text, output.String())
			}
			if posts != tc.posts || gets != tc.gets {
				t.Errorf("posts=%d gets=%d want %d/%d", posts, gets, tc.posts, tc.gets)
			}
		})
	}
}
