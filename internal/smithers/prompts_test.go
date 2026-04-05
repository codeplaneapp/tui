package smithers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test fixtures ---

// withTempPromptsDir creates a temporary .smithers/prompts directory, writes
// the given files into it, changes the working directory to the temp root, and
// registers cleanup to restore the original working directory.
func withTempPromptsDir(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".smithers", "prompts")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	// Swap working directory so promptsDir() resolves into root.
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return root
}

// --- ListPrompts ---

func TestListPrompts_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/prompt/list", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeEnvelope(t, w, []Prompt{
			{ID: "implement", EntryFile: ".smithers/prompts/implement.mdx"},
			{ID: "review", EntryFile: ".smithers/prompts/review.mdx"},
		})
	})

	prompts, err := c.ListPrompts(context.Background())
	require.NoError(t, err)
	require.Len(t, prompts, 2)
	assert.Equal(t, "implement", prompts[0].ID)
	assert.Equal(t, "review", prompts[1].ID)
}

func TestListPrompts_Filesystem(t *testing.T) {
	withTempPromptsDir(t, map[string]string{
		"implement.mdx": "# Implement\n{props.prompt}",
		"review.mdx":    "# Review\n{props.prompt}",
		"notes.txt":     "not a prompt", // should be ignored
	})
	c := NewClient() // no API URL — goes straight to filesystem

	prompts, err := c.ListPrompts(context.Background())
	require.NoError(t, err)
	require.Len(t, prompts, 2)
	ids := make([]string, len(prompts))
	for i, p := range prompts {
		ids[i] = p.ID
	}
	assert.ElementsMatch(t, []string{"implement", "review"}, ids)
	// EntryFile should be relative
	for _, p := range prompts {
		assert.Contains(t, p.EntryFile, ".smithers/prompts/")
	}
}

func TestListPrompts_Exec(t *testing.T) {
	// No filesystem dir, no HTTP → falls back to exec.
	withTempPromptsDir(t, map[string]string{}) // creates dir but empty
	// Remove the dir so filesystem fails
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"prompt", "list", "--format", "json"}, args)
		return json.Marshal([]Prompt{
			{ID: "plan", EntryFile: ".smithers/prompts/plan.mdx"},
		})
	})

	prompts, err := c.ListPrompts(context.Background())
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	assert.Equal(t, "plan", prompts[0].ID)
}

func TestListPrompts_Exec_WrappedJSON(t *testing.T) {
	// Exec returns wrapped {"prompts": [...]} shape.
	withTempPromptsDir(t, map[string]string{})
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		type wrapped struct {
			Prompts []Prompt `json:"prompts"`
		}
		return json.Marshal(wrapped{Prompts: []Prompt{
			{ID: "ticket", EntryFile: ".smithers/prompts/ticket.mdx"},
		}})
	})

	prompts, err := c.ListPrompts(context.Background())
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	assert.Equal(t, "ticket", prompts[0].ID)
}

// --- GetPrompt ---

func TestGetPrompt_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/prompt/get/implement", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeEnvelope(t, w, Prompt{
			ID:        "implement",
			EntryFile: ".smithers/prompts/implement.mdx",
			Source:    "# Implement\n{props.prompt}\n{props.schema}",
			Props: []PromptProp{
				{Name: "prompt", Type: "string"},
				{Name: "schema", Type: "string"},
			},
		})
	})

	prompt, err := c.GetPrompt(context.Background(), "implement")
	require.NoError(t, err)
	require.NotNil(t, prompt)
	assert.Equal(t, "implement", prompt.ID)
	assert.Contains(t, prompt.Source, "{props.prompt}")
	require.Len(t, prompt.Props, 2)
	assert.Equal(t, "prompt", prompt.Props[0].Name)
}

func TestGetPrompt_Filesystem(t *testing.T) {
	withTempPromptsDir(t, map[string]string{
		"review.mdx": "# Review\n\nReviewer: {props.reviewer}\n\n{props.prompt}\n\n{props.schema}",
	})
	c := NewClient()

	prompt, err := c.GetPrompt(context.Background(), "review")
	require.NoError(t, err)
	require.NotNil(t, prompt)
	assert.Equal(t, "review", prompt.ID)
	assert.Contains(t, prompt.Source, "# Review")
	require.Len(t, prompt.Props, 3)
	assert.Equal(t, "reviewer", prompt.Props[0].Name)
	assert.Equal(t, "prompt", prompt.Props[1].Name)
	assert.Equal(t, "schema", prompt.Props[2].Name)
	for _, p := range prompt.Props {
		assert.Equal(t, "string", p.Type)
	}
}

func TestGetPrompt_Exec(t *testing.T) {
	withTempPromptsDir(t, map[string]string{})
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"prompt", "get", "plan", "--format", "json"}, args)
		return json.Marshal(Prompt{
			ID:        "plan",
			EntryFile: ".smithers/prompts/plan.mdx",
			Source:    "# Plan\n{props.prompt}",
		})
	})

	prompt, err := c.GetPrompt(context.Background(), "plan")
	require.NoError(t, err)
	require.NotNil(t, prompt)
	assert.Equal(t, "plan", prompt.ID)
	assert.Equal(t, "# Plan\n{props.prompt}", prompt.Source)
}

func TestGetPrompt_NotFound(t *testing.T) {
	withTempPromptsDir(t, map[string]string{})
	// Remove dir so filesystem fails; no exec fallback either.
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("smithers prompt get nonexistent: not found")
	})

	_, err := c.GetPrompt(context.Background(), "nonexistent")
	require.Error(t, err)
}

// --- UpdatePrompt ---

func TestUpdatePrompt_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/prompt/update/implement", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "new content", body["source"])

		writeEnvelope(t, w, nil)
	})

	err := c.UpdatePrompt(context.Background(), "implement", "new content")
	require.NoError(t, err)
}

func TestUpdatePrompt_Filesystem(t *testing.T) {
	root := withTempPromptsDir(t, map[string]string{
		"implement.mdx": "# Old content",
	})
	c := NewClient()

	err := c.UpdatePrompt(context.Background(), "implement", "# New content\n{props.prompt}")
	require.NoError(t, err)

	// Verify the file was updated
	data, err := os.ReadFile(filepath.Join(root, ".smithers", "prompts", "implement.mdx"))
	require.NoError(t, err)
	assert.Equal(t, "# New content\n{props.prompt}", string(data))
}

func TestUpdatePrompt_Filesystem_NotExist(t *testing.T) {
	withTempPromptsDir(t, map[string]string{})
	// Remove dir so filesystem fails.
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	execCalled := false
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		execCalled = true
		assert.Equal(t, "prompt", args[0])
		assert.Equal(t, "update", args[1])
		assert.Equal(t, "newprompt", args[2])
		assert.Equal(t, "--source", args[3])
		return nil, nil
	})

	err := c.UpdatePrompt(context.Background(), "newprompt", "some content")
	require.NoError(t, err)
	assert.True(t, execCalled)
}

func TestUpdatePrompt_Exec(t *testing.T) {
	withTempPromptsDir(t, map[string]string{})
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "prompt", args[0])
		assert.Equal(t, "update", args[1])
		assert.Equal(t, "plan", args[2])
		assert.Equal(t, "--source", args[3])
		assert.Equal(t, "updated source", args[4])
		return nil, nil
	})

	err := c.UpdatePrompt(context.Background(), "plan", "updated source")
	require.NoError(t, err)
}

// --- DiscoverPromptProps ---

func TestDiscoverPromptProps_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/prompt/props/review", r.URL.Path)
		writeEnvelope(t, w, []PromptProp{
			{Name: "reviewer", Type: "string"},
			{Name: "prompt", Type: "string"},
		})
	})

	props, err := c.DiscoverPromptProps(context.Background(), "review")
	require.NoError(t, err)
	require.Len(t, props, 2)
	assert.Equal(t, "reviewer", props[0].Name)
}

func TestDiscoverPromptProps_Filesystem(t *testing.T) {
	withTempPromptsDir(t, map[string]string{
		"review.mdx": "Reviewer: {props.reviewer}\n{props.prompt}\n{props.schema}",
	})
	c := NewClient()

	props, err := c.DiscoverPromptProps(context.Background(), "review")
	require.NoError(t, err)
	require.Len(t, props, 3)
	assert.Equal(t, "reviewer", props[0].Name)
	assert.Equal(t, "prompt", props[1].Name)
	assert.Equal(t, "schema", props[2].Name)
}

func TestDiscoverPromptProps_NoDuplicates(t *testing.T) {
	withTempPromptsDir(t, map[string]string{
		"dupe.mdx": "{props.prompt}\n{props.prompt}\n{props.schema}\n{props.prompt}",
	})
	c := NewClient()

	props, err := c.DiscoverPromptProps(context.Background(), "dupe")
	require.NoError(t, err)
	// prompt appears 3 times but should only be in the list once
	require.Len(t, props, 2)
	assert.Equal(t, "prompt", props[0].Name)
	assert.Equal(t, "schema", props[1].Name)
}

func TestDiscoverPromptProps_NoProps(t *testing.T) {
	withTempPromptsDir(t, map[string]string{
		"static.mdx": "# Static prompt\n\nNo variables here.",
	})
	c := NewClient()

	props, err := c.DiscoverPromptProps(context.Background(), "static")
	require.NoError(t, err)
	assert.Empty(t, props)
}

func TestDiscoverPromptProps_Exec(t *testing.T) {
	withTempPromptsDir(t, map[string]string{})
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal(Prompt{
			ID:     "plan",
			Source: "# Plan\n{props.goal}\n{props.constraints}",
			Props: []PromptProp{
				{Name: "goal", Type: "string"},
				{Name: "constraints", Type: "string"},
			},
		})
	})

	props, err := c.DiscoverPromptProps(context.Background(), "plan")
	require.NoError(t, err)
	require.Len(t, props, 2)
	assert.Equal(t, "goal", props[0].Name)
}

// --- PreviewPrompt ---

func TestPreviewPrompt_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/prompt/render/implement", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		input, ok := body["input"].(map[string]any)
		require.True(t, ok, "expected 'input' key with object value")
		assert.Equal(t, "build a feature", input["prompt"])

		writeEnvelope(t, w, map[string]string{
			"result": "# Implement\nbuild a feature\n",
		})
	})

	result, err := c.PreviewPrompt(context.Background(), "implement", map[string]any{
		"prompt": "build a feature",
	})
	require.NoError(t, err)
	assert.Equal(t, "# Implement\nbuild a feature\n", result)
}

func TestPreviewPrompt_Filesystem(t *testing.T) {
	withTempPromptsDir(t, map[string]string{
		"review.mdx": "Reviewer: {props.reviewer}\n\nRequest: {props.prompt}",
	})
	c := NewClient()

	result, err := c.PreviewPrompt(context.Background(), "review", map[string]any{
		"reviewer": "Alice",
		"prompt":   "check this code",
	})
	require.NoError(t, err)
	assert.Equal(t, "Reviewer: Alice\n\nRequest: check this code", result)
}

func TestPreviewPrompt_UnresolvedPlaceholders(t *testing.T) {
	withTempPromptsDir(t, map[string]string{
		"implement.mdx": "# Implement\n{props.prompt}\n{props.schema}",
	})
	c := NewClient()

	// Only supply one of two required props
	result, err := c.PreviewPrompt(context.Background(), "implement", map[string]any{
		"prompt": "do something",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "do something")
	// Unresolved placeholder should remain intact
	assert.Contains(t, result, "{props.schema}")
}

func TestPreviewPrompt_Exec(t *testing.T) {
	withTempPromptsDir(t, map[string]string{})
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "prompt", args[0])
		assert.Equal(t, "render", args[1])
		assert.Equal(t, "plan", args[2])
		assert.Equal(t, "--input", args[3])
		// Verify the JSON props are passed
		var props map[string]any
		require.NoError(t, json.Unmarshal([]byte(args[4]), &props))
		assert.Equal(t, "my goal", props["goal"])

		return json.Marshal(map[string]string{"result": "rendered output"})
	})

	result, err := c.PreviewPrompt(context.Background(), "plan", map[string]any{
		"goal": "my goal",
	})
	require.NoError(t, err)
	assert.Equal(t, "rendered output", result)
}

func TestPreviewPrompt_Exec_PlainStringResult(t *testing.T) {
	withTempPromptsDir(t, map[string]string{})
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal("plain string result")
	})

	result, err := c.PreviewPrompt(context.Background(), "plan", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "plain string result", result)
}

func TestPreviewPrompt_Exec_RenderedField(t *testing.T) {
	withTempPromptsDir(t, map[string]string{})
	require.NoError(t, os.RemoveAll(filepath.Join(mustGetwd(t), ".smithers", "prompts")))

	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal(map[string]string{"rendered": "alternate shape"})
	})

	result, err := c.PreviewPrompt(context.Background(), "plan", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "alternate shape", result)
}

// --- discoverPropsFromSource unit tests ---

func TestDiscoverPropsFromSource_Basic(t *testing.T) {
	source := "Hello {props.name}, your score is {props.score}."
	props := discoverPropsFromSource(source)
	require.Len(t, props, 2)
	assert.Equal(t, "name", props[0].Name)
	assert.Equal(t, "string", props[0].Type)
	assert.Equal(t, "score", props[1].Name)
}

func TestDiscoverPropsFromSource_MultiLine(t *testing.T) {
	source := "Line 1: {props.a}\nLine 2: {props.b}\nLine 3: {props.a} again"
	props := discoverPropsFromSource(source)
	require.Len(t, props, 2, "duplicate props.a should appear only once")
	assert.Equal(t, "a", props[0].Name)
	assert.Equal(t, "b", props[1].Name)
}

func TestDiscoverPropsFromSource_Empty(t *testing.T) {
	props := discoverPropsFromSource("no interpolation here")
	assert.Empty(t, props)
}

func TestDiscoverPropsFromSource_MalformedIgnored(t *testing.T) {
	source := "{props.} {props} props.valid {props.valid}"
	props := discoverPropsFromSource(source)
	require.Len(t, props, 1)
	assert.Equal(t, "valid", props[0].Name)
}

// --- renderTemplate unit tests ---

func TestRenderTemplate_AllResolved(t *testing.T) {
	tmpl := "# {props.title}\n\n{props.body}"
	result := renderTemplate(tmpl, map[string]any{
		"title": "Hello",
		"body":  "World",
	})
	assert.Equal(t, "# Hello\n\nWorld", result)
}

func TestRenderTemplate_PartialResolved(t *testing.T) {
	tmpl := "{props.a} and {props.b}"
	result := renderTemplate(tmpl, map[string]any{"a": "alpha"})
	assert.Equal(t, "alpha and {props.b}", result)
}

func TestRenderTemplate_EmptyProps(t *testing.T) {
	tmpl := "{props.x}"
	result := renderTemplate(tmpl, nil)
	assert.Equal(t, "{props.x}", result)
}

func TestRenderTemplate_NumericValue(t *testing.T) {
	tmpl := "count: {props.n}"
	result := renderTemplate(tmpl, map[string]any{"n": 42})
	assert.Equal(t, "count: 42", result)
}

// --- parsePromptsJSON unit tests ---

func TestParsePromptsJSON_DirectArray(t *testing.T) {
	data, err := json.Marshal([]Prompt{{ID: "a"}, {ID: "b"}})
	require.NoError(t, err)
	prompts, err := parsePromptsJSON(data)
	require.NoError(t, err)
	assert.Len(t, prompts, 2)
}

func TestParsePromptsJSON_WrappedShape(t *testing.T) {
	type wrapped struct {
		Prompts []Prompt `json:"prompts"`
	}
	data, err := json.Marshal(wrapped{Prompts: []Prompt{{ID: "c"}}})
	require.NoError(t, err)
	prompts, err := parsePromptsJSON(data)
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	assert.Equal(t, "c", prompts[0].ID)
}

func TestParsePromptsJSON_Malformed(t *testing.T) {
	_, err := parsePromptsJSON([]byte("not json"))
	require.Error(t, err)
}

// --- parseRenderResultJSON unit tests ---

func TestParseRenderResultJSON_PlainString(t *testing.T) {
	data, err := json.Marshal("hello")
	require.NoError(t, err)
	s, err := parseRenderResultJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "hello", s)
}

func TestParseRenderResultJSON_ResultField(t *testing.T) {
	data, err := json.Marshal(map[string]string{"result": "rendered"})
	require.NoError(t, err)
	s, err := parseRenderResultJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "rendered", s)
}

func TestParseRenderResultJSON_RenderedField(t *testing.T) {
	data, err := json.Marshal(map[string]string{"rendered": "alt"})
	require.NoError(t, err)
	s, err := parseRenderResultJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "alt", s)
}

func TestParseRenderResultJSON_Malformed(t *testing.T) {
	_, err := parseRenderResultJSON([]byte("not json"))
	require.Error(t, err)
}

// --- Helpers ---

// mustGetwd is a test helper that returns the current working directory or fails.
func mustGetwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return cwd
}
