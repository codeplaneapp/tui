package config

import (
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/env"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterSlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []string
		mask     []string
		include  bool
		expected []string
	}{
		{
			name:     "exclude mode removes masked items",
			data:     []string{"a", "b", "c", "d"},
			mask:     []string{"b", "d"},
			include:  false,
			expected: []string{"a", "c"},
		},
		{
			name:     "include mode keeps only masked items",
			data:     []string{"a", "b", "c", "d"},
			mask:     []string{"b", "d"},
			include:  true,
			expected: []string{"b", "d"},
		},
		{
			name:     "empty mask with exclude keeps all",
			data:     []string{"a", "b", "c"},
			mask:     []string{},
			include:  false,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty mask with include returns nil",
			data:     []string{"a", "b", "c"},
			mask:     []string{},
			include:  true,
			expected: nil,
		},
		{
			name:     "empty data returns nil",
			data:     []string{},
			mask:     []string{"a"},
			include:  false,
			expected: nil,
		},
		{
			name:     "mask with no overlap in exclude keeps all",
			data:     []string{"a", "b"},
			mask:     []string{"x", "y"},
			include:  false,
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := filterSlice(tt.data, tt.mask, tt.include)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveAllowedTools(t *testing.T) {
	t.Parallel()

	allTools := []string{"bash", "edit", "view", "grep", "glob", "sourcegraph"}

	t.Run("nil disabled returns all tools", func(t *testing.T) {
		t.Parallel()
		result := resolveAllowedTools(allTools, nil)
		assert.Equal(t, allTools, result)
	})

	t.Run("empty disabled returns all tools", func(t *testing.T) {
		t.Parallel()
		result := resolveAllowedTools(allTools, []string{})
		assert.Equal(t, allTools, result)
	})

	t.Run("disabled tools are removed", func(t *testing.T) {
		t.Parallel()
		result := resolveAllowedTools(allTools, []string{"bash", "sourcegraph"})
		assert.Equal(t, []string{"edit", "view", "grep", "glob"}, result)
		assert.NotContains(t, result, "bash")
		assert.NotContains(t, result, "sourcegraph")
	})
}

func TestResolveReadOnlyTools(t *testing.T) {
	t.Parallel()

	t.Run("filters to read-only subset", func(t *testing.T) {
		t.Parallel()
		tools := []string{"bash", "edit", "view", "grep", "glob", "sourcegraph", "write", "ls"}
		result := resolveReadOnlyTools(tools)
		assert.Equal(t, []string{"view", "grep", "glob", "sourcegraph", "ls"}, result)
	})

	t.Run("only keeps tools that exist in input", func(t *testing.T) {
		t.Parallel()
		tools := []string{"bash", "edit", "view", "grep"}
		result := resolveReadOnlyTools(tools)
		assert.Equal(t, []string{"view", "grep"}, result)
		assert.NotContains(t, result, "bash")
		assert.NotContains(t, result, "edit")
	})
}

func TestParseKeyValueEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		expected map[string]string
	}{
		{
			name:     "single key-value pair",
			raw:      "key=value",
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "multiple key-value pairs",
			raw:      "Authorization=Bearer token,X-Scope=test",
			expected: map[string]string{"Authorization": "Bearer token", "X-Scope": "test"},
		},
		{
			name:     "empty string returns empty map",
			raw:      "",
			expected: map[string]string{},
		},
		{
			name:     "whitespace is trimmed",
			raw:      " key = value , other = stuff ",
			expected: map[string]string{"key": "value", "other": "stuff"},
		},
		{
			name:     "entries without equals are skipped",
			raw:      "good=pair,badentry,another=one",
			expected: map[string]string{"good": "pair", "another": "one"},
		},
		{
			name:     "empty key is skipped",
			raw:      "=value,key=val",
			expected: map[string]string{"key": "val"},
		},
		{
			name:     "value containing equals sign",
			raw:      "auth=Bearer=token",
			expected: map[string]string{"auth": "Bearer=token"},
		},
		{
			name:     "trailing comma handled",
			raw:      "key=value,",
			expected: map[string]string{"key": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseKeyValueEnv(tt.raw)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasAWSCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "bearer token present",
			envVars:  map[string]string{"AWS_BEARER_TOKEN_BEDROCK": "tok"},
			expected: true,
		},
		{
			name:     "access key and secret present",
			envVars:  map[string]string{"AWS_ACCESS_KEY_ID": "AKIA...", "AWS_SECRET_ACCESS_KEY": "secret"},
			expected: true,
		},
		{
			name:     "access key without secret",
			envVars:  map[string]string{"AWS_ACCESS_KEY_ID": "AKIA..."},
			expected: false,
		},
		{
			name:     "profile present",
			envVars:  map[string]string{"AWS_PROFILE": "default"},
			expected: true,
		},
		{
			name:     "default profile present",
			envVars:  map[string]string{"AWS_DEFAULT_PROFILE": "default"},
			expected: true,
		},
		{
			name:     "region present",
			envVars:  map[string]string{"AWS_REGION": "us-east-1"},
			expected: true,
		},
		{
			name:     "default region present",
			envVars:  map[string]string{"AWS_DEFAULT_REGION": "us-west-2"},
			expected: true,
		},
		{
			name:     "container credentials relative URI",
			envVars:  map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds"},
			expected: true,
		},
		{
			name:     "container credentials full URI",
			envVars:  map[string]string{"AWS_CONTAINER_CREDENTIALS_FULL_URI": "http://169.254.170.2/creds"},
			expected: true,
		},
		{
			name:     "empty env returns false",
			envVars:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := env.NewFromMap(tt.envVars)
			result := hasAWSCredentials(e)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvWithFallback(t *testing.T) {
	t.Run("primary env var is preferred", func(t *testing.T) {
		t.Setenv("CODEPLANE_TEST_VAR", "primary")
		t.Setenv("CRUSH_TEST_VAR", "legacy")
		result := envWithFallback("CODEPLANE_TEST_VAR", "CRUSH_TEST_VAR")
		assert.Equal(t, "primary", result)
	})

	t.Run("falls back to legacy when primary unset", func(t *testing.T) {
		t.Setenv("CRUSH_TEST_FALLBACK", "legacy_val")
		result := envWithFallback("CODEPLANE_TEST_UNSET_UNIQUE_42", "CRUSH_TEST_FALLBACK")
		assert.Equal(t, "legacy_val", result)
	})

	t.Run("returns empty when nothing set", func(t *testing.T) {
		result := envWithFallback("CODEPLANE_NONEXISTENT_UNIQUE_42", "CRUSH_NONEXISTENT_UNIQUE_42")
		assert.Empty(t, result)
	})
}

func TestPtrValOr(t *testing.T) {
	t.Parallel()

	t.Run("nil pointer returns default", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 42, ptrValOr((*int)(nil), 42))
		assert.Equal(t, "fallback", ptrValOr((*string)(nil), "fallback"))
	})

	t.Run("non-nil pointer returns pointed value", func(t *testing.T) {
		t.Parallel()
		v := 99
		assert.Equal(t, 99, ptrValOr(&v, 42))

		s := "actual"
		assert.Equal(t, "actual", ptrValOr(&s, "fallback"))
	})

	t.Run("zero value pointer returns zero not default", func(t *testing.T) {
		t.Parallel()
		v := 0
		assert.Equal(t, 0, ptrValOr(&v, 42))
	})
}

func TestAssignIfNil(t *testing.T) {
	t.Parallel()

	t.Run("assigns value when pointer is nil", func(t *testing.T) {
		t.Parallel()
		var p *int
		assignIfNil(&p, 7)
		require.NotNil(t, p)
		assert.Equal(t, 7, *p)
	})

	t.Run("does not overwrite existing value", func(t *testing.T) {
		t.Parallel()
		v := 3
		p := &v
		assignIfNil(&p, 7)
		assert.Equal(t, 3, *p)
	})
}

func TestToolGrepGetTimeout(t *testing.T) {
	t.Parallel()

	t.Run("returns default when nil", func(t *testing.T) {
		t.Parallel()
		g := ToolGrep{}
		assert.Equal(t, 5*time.Second, g.GetTimeout())
	})

	t.Run("returns configured timeout", func(t *testing.T) {
		t.Parallel()
		d := 10 * time.Second
		g := ToolGrep{Timeout: &d}
		assert.Equal(t, 10*time.Second, g.GetTimeout())
	})
}

func TestToolLsLimits(t *testing.T) {
	t.Parallel()

	t.Run("returns zeros when nil", func(t *testing.T) {
		t.Parallel()
		ls := ToolLs{}
		depth, items := ls.Limits()
		assert.Equal(t, 0, depth)
		assert.Equal(t, 0, items)
	})

	t.Run("returns configured values", func(t *testing.T) {
		t.Parallel()
		d, i := 5, 200
		ls := ToolLs{MaxDepth: &d, MaxItems: &i}
		depth, items := ls.Limits()
		assert.Equal(t, 5, depth)
		assert.Equal(t, 200, items)
	})
}

func TestCompletionsLimits(t *testing.T) {
	t.Parallel()

	t.Run("returns zeros when nil", func(t *testing.T) {
		t.Parallel()
		c := Completions{}
		depth, items := c.Limits()
		assert.Equal(t, 0, depth)
		assert.Equal(t, 0, items)
	})

	t.Run("returns configured values", func(t *testing.T) {
		t.Parallel()
		d, i := 3, 50
		c := Completions{MaxDepth: &d, MaxItems: &i}
		depth, items := c.Limits()
		assert.Equal(t, 3, depth)
		assert.Equal(t, 50, items)
	})
}

func TestMCPsSorted(t *testing.T) {
	t.Parallel()

	mcps := MCPs{
		"zebra": MCPConfig{Command: "z-server"},
		"alpha": MCPConfig{Command: "a-server"},
		"mid":   MCPConfig{Command: "m-server"},
	}

	sorted := mcps.Sorted()
	require.Len(t, sorted, 3)
	assert.Equal(t, "alpha", sorted[0].Name)
	assert.Equal(t, "mid", sorted[1].Name)
	assert.Equal(t, "zebra", sorted[2].Name)
}

func TestLSPsSorted(t *testing.T) {
	t.Parallel()

	lsps := LSPs{
		"gopls":         LSPConfig{Command: "gopls"},
		"clangd":        LSPConfig{Command: "clangd"},
		"rust-analyzer": LSPConfig{Command: "rust-analyzer"},
	}

	sorted := lsps.Sorted()
	require.Len(t, sorted, 3)
	assert.Equal(t, "clangd", sorted[0].Name)
	assert.Equal(t, "gopls", sorted[1].Name)
	assert.Equal(t, "rust-analyzer", sorted[2].Name)
}
