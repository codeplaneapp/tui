package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchParseLiteSearchResults_NormalizesDuckDuckGoRedirectURL(t *testing.T) {
	t.Parallel()

	htmlContent := `<html><body><table>
		<tr><td><a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fdocs%3Fa%3D1%26b%3D2&rut=ignored">Example Title</a></td></tr>
		<tr><td class="result-snippet">Example snippet</td></tr>
	</table></body></html>`

	results, err := parseLiteSearchResults(htmlContent, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "Example Title", results[0].Title)
	assert.Equal(t, "https://example.com/docs?a=1&b=2", results[0].Link)
	assert.Equal(t, "Example snippet", results[0].Snippet)
	assert.Equal(t, 1, results[0].Position)
}
