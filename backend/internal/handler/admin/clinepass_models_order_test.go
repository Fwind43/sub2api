package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortClinePassModelIDsPrefersSubscriptionFamily(t *testing.T) {
	ids := []string{
		"anthropic/claude-opus-5.5",
		"anthropic/claude-sonnet-5.5",
		"cline-free/deepseek-v4.1-flash",
		"cline-free/mimo-v2.6-flash",
		"cline-pass/deepseek-v4-pro",
		"cline-pass/glm-5.3",
	}

	sortClinePassModelIDs(ids)

	require.Equal(t, []string{
		"cline-pass/deepseek-v4-pro",
		"cline-pass/glm-5.3",
		"anthropic/claude-opus-5.5",
		"anthropic/claude-sonnet-5.5",
		"cline-free/deepseek-v4.1-flash",
		"cline-free/mimo-v2.6-flash",
	}, ids)
}

func TestSortClinePassModelIDsIsIdempotent(t *testing.T) {
	ids := []string{"cline-free/x", "anthropic/y", "cline-pass/z"}

	sortClinePassModelIDs(ids)
	first := append([]string(nil), ids...)
	sortClinePassModelIDs(ids)

	require.Equal(t, first, ids)
}
