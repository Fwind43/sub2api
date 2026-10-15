package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Anthropic stop_sequences must survive the direct Messages -> Chat
// Completions bridge as the OpenAI `stop` field (string or []string); it used
// to be dropped silently, so client-side stop sequences never reached the
// upstream.
func TestAnthropicToChatCompletionsRequest_StopSequences(t *testing.T) {
	cases := []struct {
		name string
		seqs []string
		want []string
	}{
		{name: "absent", seqs: nil, want: nil},
		{name: "empty", seqs: []string{}, want: nil},
		{name: "blank only", seqs: []string{"", "  "}, want: nil},
		{name: "single", seqs: []string{"END"}, want: []string{"END"}},
		{name: "multiple", seqs: []string{"END", "\n\nHuman:"}, want: []string{"END", "\n\nHuman:"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &AnthropicRequest{
				Model:     "claude-sonnet-4-5",
				MaxTokens: 16,
				Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
				StopSeqs:  tc.seqs,
			}
			out, err := AnthropicToChatCompletionsRequest(req)
			require.NoError(t, err)

			if tc.want == nil {
				require.Empty(t, out.Stop)
			} else {
				var got []string
				require.NoError(t, json.Unmarshal(out.Stop, &got))
				require.Equal(t, tc.want, got)
			}

			encoded, err := json.Marshal(out)
			require.NoError(t, err)
			if tc.want == nil {
				require.NotContains(t, string(encoded), `"stop"`)
			} else if len(tc.want) == 1 {
				require.Contains(t, string(encoded), `"stop":["END"]`)
			} else {
				require.Contains(t, string(encoded), `"stop":["END","\n\nHuman:"]`)
			}
		})
	}
}
