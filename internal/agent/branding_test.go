package agent

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrandingUsesCodeplane(t *testing.T) {
	t.Parallel()

	require.Contains(t, userAgent, "Codeplane")
	require.NotContains(t, strings.ToLower(userAgent), "crush")

	for _, path := range []string{
		"templates/task.md.tpl",
		"templates/agentic_fetch_prompt.md.tpl",
		"templates/smithers.md.tpl",
	} {
		bts, err := os.ReadFile(path)
		require.NoError(t, err)

		text := string(bts)
		require.Contains(t, text, "Codeplane")
		require.NotContains(t, text, " for Crush")
	}
}
