package swagger

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSwaggerInfoUsesCodeplaneBranding(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Codeplane API", SwaggerInfo.Title)
	require.Contains(t, SwaggerInfo.Description, "Codeplane")
	require.NotContains(t, SwaggerInfo.Description, "Crush")
	require.NotContains(t, strings.ToLower(docTemplate), "title\": \"crush api")
}
