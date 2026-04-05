package smithers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// propPattern matches {props.Name} interpolation expressions in MDX source.
// It captures the property name (e.g. "prompt" from {props.prompt}).
var propPattern = regexp.MustCompile(`\{props\.([A-Za-z_][A-Za-z0-9_]*)\}`)

// --- Prompts ---

// ListPrompts returns all prompts discovered from .smithers/prompts/.
// Routes: HTTP GET /prompt/list → filesystem (.smithers/prompts/) → exec smithers prompt list.
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var prompts []Prompt
		if err := c.httpGetJSON(ctx, "/prompt/list", &prompts); err == nil {
			return prompts, nil
		}
	}

	// 2. Try filesystem — scan .smithers/prompts/ for .mdx files.
	if prompts, err := listPromptsFromFS(); err == nil {
		return prompts, nil
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "prompt", "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parsePromptsJSON(out)
}

// GetPrompt returns a single prompt by ID with its full MDX source populated.
// Routes: HTTP GET /prompt/get/{id} → filesystem → exec smithers prompt get.
func (c *Client) GetPrompt(ctx context.Context, promptID string) (*Prompt, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var prompt Prompt
		if err := c.httpGetJSON(ctx, "/prompt/get/"+promptID, &prompt); err == nil {
			return &prompt, nil
		}
	}

	// 2. Try filesystem
	if prompt, err := getPromptFromFS(promptID); err == nil {
		return prompt, nil
	}

	// 3. Fall back to exec
	out, err := c.execSmithers(ctx, "prompt", "get", promptID, "--format", "json")
	if err != nil {
		return nil, err
	}
	return parsePromptJSON(out)
}

// UpdatePrompt overwrites the MDX source for the given prompt ID.
// Routes: HTTP POST /prompt/update/{id} → filesystem write → exec smithers prompt update.
func (c *Client) UpdatePrompt(ctx context.Context, promptID string, content string) error {
	// 1. Try HTTP
	if c.isServerAvailable() {
		err := c.httpPostJSON(ctx, "/prompt/update/"+promptID,
			map[string]string{"source": content}, nil)
		if err == nil {
			return nil
		}
	}

	// 2. Try filesystem write — locate the file and overwrite it.
	if err := updatePromptOnFS(promptID, content); err == nil {
		return nil
	}

	// 3. Fall back to exec
	_, err := c.execSmithers(ctx, "prompt", "update", promptID, "--source", content)
	return err
}

// DiscoverPromptProps parses the MDX source for promptID and returns the list
// of interpolation variables ({props.X}) found in the template.
// Routes: HTTP GET /prompt/props/{id} → local parse of filesystem source.
func (c *Client) DiscoverPromptProps(ctx context.Context, promptID string) ([]PromptProp, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var props []PromptProp
		if err := c.httpGetJSON(ctx, "/prompt/props/"+promptID, &props); err == nil {
			return props, nil
		}
	}

	// 2. Parse from filesystem source
	prompt, err := getPromptFromFS(promptID)
	if err != nil {
		// 3. Fall back to exec — get the prompt then parse locally
		out, execErr := c.execSmithers(ctx, "prompt", "get", promptID, "--format", "json")
		if execErr != nil {
			return nil, fmt.Errorf("discover prompt props %s: %w", promptID, err)
		}
		p, parseErr := parsePromptJSON(out)
		if parseErr != nil {
			return nil, fmt.Errorf("discover prompt props %s: %w", promptID, parseErr)
		}
		if len(p.Props) > 0 {
			return p.Props, nil
		}
		return discoverPropsFromSource(p.Source), nil
	}

	if len(prompt.Props) > 0 {
		return prompt.Props, nil
	}
	return discoverPropsFromSource(prompt.Source), nil
}

// PreviewPrompt renders the prompt template with the supplied props map and
// returns the resulting string.
// Routes: HTTP POST /prompt/render/{id} → local template render → exec smithers prompt render.
func (c *Client) PreviewPrompt(ctx context.Context, promptID string, props map[string]any) (string, error) {
	// 1. Try HTTP
	if c.isServerAvailable() {
		var result struct {
			Result string `json:"result"`
		}
		if err := c.httpPostJSON(ctx, "/prompt/render/"+promptID, map[string]any{"input": props}, &result); err == nil {
			return result.Result, nil
		}
	}

	// 2. Render locally from filesystem source
	prompt, err := getPromptFromFS(promptID)
	if err == nil {
		rendered := renderTemplate(prompt.Source, props)
		return rendered, nil
	}

	// 3. Fall back to exec — pass props as JSON
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("marshal props: %w", err)
	}
	out, err := c.execSmithers(ctx, "prompt", "render", promptID,
		"--input", string(propsJSON), "--format", "json")
	if err != nil {
		return "", err
	}
	return parseRenderResultJSON(out)
}

// --- Filesystem helpers ---

// promptsDir returns the path to the .smithers/prompts directory relative to
// the current working directory. This mirrors how Smithers CLI resolves prompts.
func promptsDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ".smithers/prompts"
	}
	return filepath.Join(cwd, ".smithers", "prompts")
}

// listPromptsFromFS scans the .smithers/prompts directory for .mdx files and
// returns a Prompt entry for each (without loading the full source).
func listPromptsFromFS() ([]Prompt, error) {
	dir := promptsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read prompts dir: %w", err)
	}

	var prompts []Prompt
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".mdx") {
			continue
		}
		id := strings.TrimSuffix(name, ".mdx")
		prompts = append(prompts, Prompt{
			ID:        id,
			EntryFile: filepath.Join(".smithers", "prompts", name),
		})
	}
	return prompts, nil
}

// getPromptFromFS reads the .mdx file for promptID and populates Source and Props.
func getPromptFromFS(promptID string) (*Prompt, error) {
	dir := promptsDir()
	path := filepath.Join(dir, promptID+".mdx")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read prompt %s: %w", promptID, err)
	}
	source := string(data)
	props := discoverPropsFromSource(source)
	return &Prompt{
		ID:        promptID,
		EntryFile: filepath.Join(".smithers", "prompts", promptID+".mdx"),
		Source:    source,
		Props:     props,
	}, nil
}

// updatePromptOnFS overwrites the .mdx file for promptID with content.
func updatePromptOnFS(promptID string, content string) error {
	dir := promptsDir()
	path := filepath.Join(dir, promptID+".mdx")
	// Verify the file exists before writing to avoid creating stray files.
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("prompt %s not found: %w", promptID, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write prompt %s: %w", promptID, err)
	}
	return nil
}

// discoverPropsFromSource scans MDX source for {props.X} expressions and
// returns one PromptProp per unique variable name, in order of first appearance.
func discoverPropsFromSource(source string) []PromptProp {
	seen := make(map[string]bool)
	var props []PromptProp

	scanner := bufio.NewScanner(bytes.NewBufferString(source))
	for scanner.Scan() {
		line := scanner.Text()
		matches := propPattern.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			name := m[1]
			if !seen[name] {
				seen[name] = true
				props = append(props, PromptProp{
					Name: name,
					Type: "string",
				})
			}
		}
	}
	return props
}

// renderTemplate performs simple {props.X} substitution on an MDX template.
// Keys in the props map are matched case-sensitively against variable names.
func renderTemplate(source string, props map[string]any) string {
	return propPattern.ReplaceAllStringFunc(source, func(match string) string {
		// Extract variable name from {props.NAME}
		sub := propPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := sub[1]
		if val, ok := props[name]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match // leave unresolved placeholders intact
	})
}

// --- JSON parse helpers ---

// parsePromptsJSON parses exec output (or direct HTTP JSON) into a Prompt slice.
// Tolerates both a direct array and an envelope-wrapped {"prompts": [...]} shape.
func parsePromptsJSON(data []byte) ([]Prompt, error) {
	// Try direct array first.
	var prompts []Prompt
	if err := json.Unmarshal(data, &prompts); err == nil {
		return prompts, nil
	}
	// Try wrapped shape {"prompts": [...]}.
	var wrapped struct {
		Prompts []Prompt `json:"prompts"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("parse prompts: %w", err)
	}
	return wrapped.Prompts, nil
}

// parsePromptJSON parses exec output into a single Prompt.
func parsePromptJSON(data []byte) (*Prompt, error) {
	var prompt Prompt
	if err := json.Unmarshal(data, &prompt); err != nil {
		return nil, fmt.Errorf("parse prompt: %w", err)
	}
	return &prompt, nil
}

// parseRenderResultJSON parses a render response.
// Tolerates plain string, {"result": "..."}, and {"rendered": "..."} shapes.
func parseRenderResultJSON(data []byte) (string, error) {
	// Try plain string.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return s, nil
	}
	// Try {"result": "..."}.
	var r struct {
		Result   string `json:"result"`
		Rendered string `json:"rendered"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("parse render result: %w", err)
	}
	if r.Result != "" {
		return r.Result, nil
	}
	return r.Rendered, nil
}
