package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		wantName    string
		wantDesc    string
		wantLicense string
		wantCompat  string
		wantMeta    map[string]string
		wantTools   string
		wantInstr   string
		wantErr     bool
	}{
		{
			name: "full skill",
			content: `---
name: pdf-processing
description: Extracts text and tables from PDF files, fills PDF forms, and merges multiple PDFs.
license: Apache-2.0
compatibility: Requires python 3.8+, pdfplumber, pdfrw libraries
metadata:
  author: example-org
  version: "1.0"
---

# PDF Processing

## When to use this skill
Use this skill when the user needs to work with PDF files.
`,
			wantName:    "pdf-processing",
			wantDesc:    "Extracts text and tables from PDF files, fills PDF forms, and merges multiple PDFs.",
			wantLicense: "Apache-2.0",
			wantCompat:  "Requires python 3.8+, pdfplumber, pdfrw libraries",
			wantMeta:    map[string]string{"author": "example-org", "version": "1.0"},
			wantInstr:   "# PDF Processing\n\n## When to use this skill\nUse this skill when the user needs to work with PDF files.",
		},
		{
			name: "minimal skill",
			content: `---
name: my-skill
description: A simple skill for testing.
---

# My Skill

Instructions here.
`,
			wantName:  "my-skill",
			wantDesc:  "A simple skill for testing.",
			wantInstr: "# My Skill\n\nInstructions here.",
		},
		{
			name:    "no frontmatter",
			content: "# Just Markdown\n\nNo frontmatter here.",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Write content to temp file.
			dir := t.TempDir()
			path := filepath.Join(dir, "SKILL.md")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o644))

			skill, err := Parse(path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.Equal(t, tt.wantName, skill.Name)
			require.Equal(t, tt.wantDesc, skill.Description)
			require.Equal(t, tt.wantLicense, skill.License)
			require.Equal(t, tt.wantCompat, skill.Compatibility)

			if tt.wantMeta != nil {
				require.Equal(t, tt.wantMeta, skill.Metadata)
			}

			require.Equal(t, tt.wantInstr, skill.Instructions)
		})
	}
}

func TestSkillValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		skill   Skill
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid skill",
			skill: Skill{
				Name:        "pdf-processing",
				Description: "Processes PDF files.",
				Path:        "/skills/pdf-processing",
			},
		},
		{
			name:    "missing name",
			skill:   Skill{Description: "Some description."},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name:    "missing description",
			skill:   Skill{Name: "my-skill", Path: "/skills/my-skill"},
			wantErr: true,
			errMsg:  "description is required",
		},
		{
			name:    "name too long",
			skill:   Skill{Name: strings.Repeat("a", 65), Description: "Some description."},
			wantErr: true,
			errMsg:  "exceeds",
		},
		{
			name:    "valid name - mixed case",
			skill:   Skill{Name: "MySkill", Description: "Some description.", Path: "/skills/MySkill"},
			wantErr: false,
		},
		{
			name:    "invalid name - starts with hyphen",
			skill:   Skill{Name: "-my-skill", Description: "Some description."},
			wantErr: true,
			errMsg:  "alphanumeric with hyphens",
		},
		{
			name:    "name doesn't match directory",
			skill:   Skill{Name: "my-skill", Description: "Some description.", Path: "/skills/other-skill"},
			wantErr: true,
			errMsg:  "must match directory",
		},
		{
			name:    "description too long",
			skill:   Skill{Name: "my-skill", Description: strings.Repeat("a", 1025), Path: "/skills/my-skill"},
			wantErr: true,
			errMsg:  "description exceeds",
		},
		{
			name:    "compatibility too long",
			skill:   Skill{Name: "my-skill", Description: "desc", Compatibility: strings.Repeat("a", 501), Path: "/skills/my-skill"},
			wantErr: true,
			errMsg:  "compatibility exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.skill.Validate()
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDiscover(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create valid skill 1.
	skill1Dir := filepath.Join(tmpDir, "skill-one")
	require.NoError(t, os.MkdirAll(skill1Dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill1Dir, "SKILL.md"), []byte(`---
name: skill-one
description: First test skill.
---
# Skill One
`), 0o644))

	// Create valid skill 2 in nested directory.
	skill2Dir := filepath.Join(tmpDir, "nested", "skill-two")
	require.NoError(t, os.MkdirAll(skill2Dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill2Dir, "SKILL.md"), []byte(`---
name: skill-two
description: Second test skill.
---
# Skill Two
`), 0o644))

	// Create invalid skill (won't be included).
	invalidDir := filepath.Join(tmpDir, "invalid-dir")
	require.NoError(t, os.MkdirAll(invalidDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(invalidDir, "SKILL.md"), []byte(`---
name: wrong-name
description: Name doesn't match directory.
---
`), 0o644))

	skills := Discover([]string{tmpDir})
	require.Len(t, skills, 2)

	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
	}
	require.True(t, names["skill-one"])
	require.True(t, names["skill-two"])
}

func TestToPromptXML(t *testing.T) {
	t.Parallel()

	skills := []*Skill{
		{Name: "pdf-processing", Description: "Extracts text from PDFs.", SkillFilePath: "/skills/pdf-processing/SKILL.md"},
		{Name: "data-analysis", Description: "Analyzes datasets & charts.", SkillFilePath: "/skills/data-analysis/SKILL.md"},
	}

	xml := ToPromptXML(skills)

	require.Contains(t, xml, "<available_skills>")
	require.Contains(t, xml, "<name>pdf-processing</name>")
	require.Contains(t, xml, "<description>Extracts text from PDFs.</description>")
	require.Contains(t, xml, "&amp;") // XML escaping
}

func TestToPromptXMLEmpty(t *testing.T) {
	t.Parallel()
	require.Empty(t, ToPromptXML(nil))
	require.Empty(t, ToPromptXML([]*Skill{}))
}

func TestToPromptXMLBuiltinType(t *testing.T) {
	t.Parallel()

	skills := []*Skill{
		{Name: "builtin-skill", Description: "A builtin.", SkillFilePath: "crush://skills/builtin-skill/SKILL.md", Builtin: true},
		{Name: "user-skill", Description: "A user skill.", SkillFilePath: "/home/user/.config/crush/skills/user-skill/SKILL.md"},
	}
	xml := ToPromptXML(skills)
	require.Contains(t, xml, "<type>builtin</type>")
	require.Equal(t, 1, strings.Count(xml, "<type>builtin</type>"))
}

func TestParseContent(t *testing.T) {
	t.Parallel()

	content := []byte(`---
name: my-skill
description: A test skill.
---

# My Skill

Instructions here.
`)
	skill, err := ParseContent(content)
	require.NoError(t, err)
	require.Equal(t, "my-skill", skill.Name)
	require.Equal(t, "A test skill.", skill.Description)
	require.Equal(t, "# My Skill\n\nInstructions here.", skill.Instructions)
	require.Empty(t, skill.Path)
	require.Empty(t, skill.SkillFilePath)
}

func TestParseContent_NoFrontmatter(t *testing.T) {
	t.Parallel()

	_, err := ParseContent([]byte("# Just Markdown"))
	require.Error(t, err)
}

func TestDiscoverBuiltin(t *testing.T) {
	t.Parallel()

	discovered := DiscoverBuiltin()
	require.NotEmpty(t, discovered)

	var found bool
	for _, s := range discovered {
		if s.Name == "crush-config" {
			found = true
			require.True(t, strings.HasPrefix(s.SkillFilePath, BuiltinPrefix))
			require.True(t, strings.HasPrefix(s.Path, BuiltinPrefix))
			require.Equal(t, "crush://skills/crush-config/SKILL.md", s.SkillFilePath)
			require.Equal(t, "crush://skills/crush-config", s.Path)
			require.NotEmpty(t, s.Description)
			require.NotEmpty(t, s.Instructions)
			require.True(t, s.Builtin)
		}
	}
	require.True(t, found, "crush-config builtin skill not found")
}

func TestDeduplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []*Skill
		wantLen  int
		wantName string
		wantPath string
	}{
		{
			name:    "no duplicates",
			input:   []*Skill{{Name: "a", Path: "/a"}, {Name: "b", Path: "/b"}},
			wantLen: 2,
		},
		{
			name:     "user overrides builtin",
			input:    []*Skill{{Name: "crush-config", Path: "crush://skills/crush-config"}, {Name: "crush-config", Path: "/user/crush-config"}},
			wantLen:  1,
			wantName: "crush-config",
			wantPath: "/user/crush-config",
		},
		{
			name:    "empty",
			input:   nil,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Deduplicate(tt.input)
			require.Len(t, result, tt.wantLen)
			if tt.wantName != "" {
				require.Equal(t, tt.wantName, result[0].Name)
				require.Equal(t, tt.wantPath, result[0].Path)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()

	all := []*Skill{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}

	tests := []struct {
		name     string
		disabled []string
		wantLen  int
	}{
		{"no filter", nil, 3},
		{"filter one", []string{"b"}, 2},
		{"filter all", []string{"a", "b", "c"}, 0},
		{"filter nonexistent", []string{"d"}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Filter(all, tt.disabled)
			require.Len(t, result, tt.wantLen)
		})
	}
}

// ---------------------------------------------------------------------------
// Additional Validate tests
// ---------------------------------------------------------------------------

func TestSkill_Validate_Valid(t *testing.T) {
	t.Parallel()
	s := Skill{
		Name:        "my-skill",
		Description: "Does something useful.",
		Path:        "/skills/my-skill",
	}
	assert.NoError(t, s.Validate())
}

func TestSkill_Validate_EmptyName(t *testing.T) {
	t.Parallel()
	s := Skill{Name: "", Description: "Has a description."}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestSkill_Validate_NameTooLong(t *testing.T) {
	t.Parallel()
	s := Skill{Name: strings.Repeat("x", MaxNameLength+1), Description: "desc"}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name exceeds")
}

func TestSkill_Validate_NameExactlyAtMaxLength(t *testing.T) {
	t.Parallel()
	// 64 chars should be valid.
	s := Skill{Name: strings.Repeat("a", MaxNameLength), Description: "desc"}
	assert.NoError(t, s.Validate())
}

func TestSkill_Validate_InvalidNamePattern(t *testing.T) {
	t.Parallel()

	cases := []struct {
		label string
		name  string
	}{
		{"spaces", "my skill"},
		{"leading hyphen", "-leading"},
		{"trailing hyphen", "trailing-"},
		{"consecutive hyphens", "double--hyphen"},
		{"special chars", "my_skill!"},
		{"underscore", "my_skill"},
		{"dot", "my.skill"},
		{"slash", "my/skill"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()
			s := Skill{Name: tc.name, Description: "desc"}
			err := s.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "alphanumeric with hyphens")
		})
	}
}

func TestSkill_Validate_NameMustMatchDir(t *testing.T) {
	t.Parallel()
	s := Skill{Name: "alpha", Description: "desc", Path: "/skills/beta"}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must match directory")
}

func TestSkill_Validate_NameMatchesDirCaseInsensitive(t *testing.T) {
	t.Parallel()
	// EqualFold is used, so case-insensitive match should pass.
	s := Skill{Name: "MySkill", Description: "desc", Path: "/skills/myskill"}
	assert.NoError(t, s.Validate())
}

func TestSkill_Validate_EmptyPathSkipsDirCheck(t *testing.T) {
	t.Parallel()
	// When Path is empty the directory check is skipped entirely.
	s := Skill{Name: "anything", Description: "desc", Path: ""}
	assert.NoError(t, s.Validate())
}

func TestSkill_Validate_EmptyDescription(t *testing.T) {
	t.Parallel()
	s := Skill{Name: "valid-name", Description: "", Path: "/skills/valid-name"}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description is required")
}

func TestSkill_Validate_DescriptionTooLong(t *testing.T) {
	t.Parallel()
	s := Skill{
		Name:        "my-skill",
		Description: strings.Repeat("d", MaxDescriptionLength+1),
		Path:        "/skills/my-skill",
	}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description exceeds")
}

func TestSkill_Validate_DescriptionExactlyAtMax(t *testing.T) {
	t.Parallel()
	s := Skill{
		Name:        "my-skill",
		Description: strings.Repeat("d", MaxDescriptionLength),
		Path:        "/skills/my-skill",
	}
	assert.NoError(t, s.Validate())
}

func TestSkill_Validate_CompatibilityTooLong(t *testing.T) {
	t.Parallel()
	s := Skill{
		Name:          "my-skill",
		Description:   "desc",
		Compatibility: strings.Repeat("c", MaxCompatibilityLength+1),
		Path:          "/skills/my-skill",
	}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compatibility exceeds")
}

func TestSkill_Validate_CompatibilityExactlyAtMax(t *testing.T) {
	t.Parallel()
	s := Skill{
		Name:          "my-skill",
		Description:   "desc",
		Compatibility: strings.Repeat("c", MaxCompatibilityLength),
		Path:          "/skills/my-skill",
	}
	assert.NoError(t, s.Validate())
}

func TestSkill_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()
	// Both name and description are empty -- errors.Join produces both messages.
	s := Skill{}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
	assert.Contains(t, err.Error(), "description is required")
}

// ---------------------------------------------------------------------------
// splitFrontmatter tests
// ---------------------------------------------------------------------------

func TestSplitFrontmatter_Valid(t *testing.T) {
	t.Parallel()
	input := "---\nname: test\n---\nBody content"
	fm, body, err := splitFrontmatter(input)
	require.NoError(t, err)
	assert.Equal(t, "name: test", fm)
	assert.Equal(t, "\nBody content", body)
}

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	t.Parallel()
	_, _, err := splitFrontmatter("no frontmatter here")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no YAML frontmatter found")
}

func TestSplitFrontmatter_UnclosedFrontmatter(t *testing.T) {
	t.Parallel()
	_, _, err := splitFrontmatter("---\nname: test\nno closing delimiter")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unclosed frontmatter")
}

func TestSplitFrontmatter_CRLFNormalization(t *testing.T) {
	t.Parallel()
	// Windows-style line endings should be normalized to \n.
	input := "---\r\nname: test\r\n---\r\nBody"
	fm, body, err := splitFrontmatter(input)
	require.NoError(t, err)
	assert.Equal(t, "name: test", fm)
	assert.Equal(t, "\nBody", body)
}

func TestSplitFrontmatter_EmptyBody(t *testing.T) {
	t.Parallel()
	input := "---\nname: test\n---"
	fm, body, err := splitFrontmatter(input)
	require.NoError(t, err)
	assert.Equal(t, "name: test", fm)
	assert.Empty(t, body)
}

// ---------------------------------------------------------------------------
// Additional ParseContent tests
// ---------------------------------------------------------------------------

func TestParseContent_ValidSkill(t *testing.T) {
	t.Parallel()
	content := []byte("---\nname: demo\ndescription: A demo skill.\nlicense: MIT\n---\n\n# Demo\n\nInstructions.")
	skill, err := ParseContent(content)
	require.NoError(t, err)
	assert.Equal(t, "demo", skill.Name)
	assert.Equal(t, "A demo skill.", skill.Description)
	assert.Equal(t, "MIT", skill.License)
	assert.Equal(t, "# Demo\n\nInstructions.", skill.Instructions)
}

func TestParseContent_MissingFrontmatter(t *testing.T) {
	t.Parallel()
	_, err := ParseContent([]byte("no delimiters at all"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no YAML frontmatter found")
}

func TestParseContent_UnclosedFrontmatter(t *testing.T) {
	t.Parallel()
	_, err := ParseContent([]byte("---\nname: test\nno close"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unclosed frontmatter")
}

func TestParseContent_CRLFContent(t *testing.T) {
	t.Parallel()
	content := []byte("---\r\nname: win\r\ndescription: CRLF skill.\r\n---\r\n\r\nBody.")
	skill, err := ParseContent(content)
	require.NoError(t, err)
	assert.Equal(t, "win", skill.Name)
	assert.Equal(t, "CRLF skill.", skill.Description)
	assert.Equal(t, "Body.", skill.Instructions)
}

func TestParseContent_EmptyBody(t *testing.T) {
	t.Parallel()
	content := []byte("---\nname: empty-body\ndescription: No instructions.\n---")
	skill, err := ParseContent(content)
	require.NoError(t, err)
	assert.Equal(t, "empty-body", skill.Name)
	assert.Empty(t, skill.Instructions)
}

func TestParseContent_InvalidYAML(t *testing.T) {
	t.Parallel()
	content := []byte("---\n: :\n  bad yaml {{{\n---\nBody")
	_, err := ParseContent(content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing frontmatter")
}

// ---------------------------------------------------------------------------
// Additional Deduplicate tests
// ---------------------------------------------------------------------------

func TestDeduplicate_ThreeWayDuplicate(t *testing.T) {
	t.Parallel()
	input := []*Skill{
		{Name: "dup", Path: "/first"},
		{Name: "dup", Path: "/second"},
		{Name: "dup", Path: "/third"},
	}
	result := Deduplicate(input)
	require.Len(t, result, 1)
	// Last occurrence wins.
	assert.Equal(t, "/third", result[0].Path)
}

func TestDeduplicate_PreservesOrder(t *testing.T) {
	t.Parallel()
	input := []*Skill{
		{Name: "aaa", Path: "/a"},
		{Name: "bbb", Path: "/b1"},
		{Name: "ccc", Path: "/c"},
		{Name: "bbb", Path: "/b2"},
	}
	result := Deduplicate(input)
	require.Len(t, result, 3)
	// Order: aaa, then bbb (last-wins at index 3, placed at its final position
	// in iteration order), then ccc.
	assert.Equal(t, "aaa", result[0].Name)
	assert.Equal(t, "ccc", result[1].Name)
	assert.Equal(t, "bbb", result[2].Name)
	assert.Equal(t, "/b2", result[2].Path)
}

func TestDeduplicate_SingleElement(t *testing.T) {
	t.Parallel()
	input := []*Skill{{Name: "solo", Path: "/solo"}}
	result := Deduplicate(input)
	require.Len(t, result, 1)
	assert.Equal(t, "solo", result[0].Name)
}

// ---------------------------------------------------------------------------
// Additional Filter tests
// ---------------------------------------------------------------------------

func TestFilter_VerifiesRemainingSkills(t *testing.T) {
	t.Parallel()
	all := []*Skill{
		{Name: "keep-me"},
		{Name: "remove-me"},
		{Name: "also-keep"},
	}
	result := Filter(all, []string{"remove-me"})
	require.Len(t, result, 2)
	assert.Equal(t, "keep-me", result[0].Name)
	assert.Equal(t, "also-keep", result[1].Name)
}

func TestFilter_EmptyInput(t *testing.T) {
	t.Parallel()
	result := Filter(nil, []string{"anything"})
	assert.Empty(t, result)
}

func TestFilter_EmptyDisabledList(t *testing.T) {
	t.Parallel()
	all := []*Skill{{Name: "a"}, {Name: "b"}}
	result := Filter(all, nil)
	// Should return the same slice.
	assert.Len(t, result, 2)
	assert.Equal(t, "a", result[0].Name)
	assert.Equal(t, "b", result[1].Name)
}

// ---------------------------------------------------------------------------
// Additional ToPromptXML tests
// ---------------------------------------------------------------------------

func TestToPromptXML_FullStructure(t *testing.T) {
	t.Parallel()
	skills := []*Skill{
		{
			Name:          "my-tool",
			Description:   "Does things.",
			SkillFilePath: "/skills/my-tool/SKILL.md",
		},
	}
	xml := ToPromptXML(skills)
	expected := `<available_skills>
  <skill>
    <name>my-tool</name>
    <description>Does things.</description>
    <location>/skills/my-tool/SKILL.md</location>
  </skill>
</available_skills>`
	assert.Equal(t, expected, xml)
}

func TestToPromptXML_EscapesAllXMLSpecialChars(t *testing.T) {
	t.Parallel()
	skills := []*Skill{
		{
			Name:          "esc",
			Description:   `Handles & < > " ' chars.`,
			SkillFilePath: "/skills/esc/SKILL.md",
		},
	}
	xml := ToPromptXML(skills)
	assert.Contains(t, xml, "&amp;")
	assert.Contains(t, xml, "&lt;")
	assert.Contains(t, xml, "&gt;")
	assert.Contains(t, xml, "&quot;")
	assert.Contains(t, xml, "&apos;")
	// Make sure raw special chars are not present in the description portion.
	assert.NotContains(t, xml, "Handles & <")
}

func TestToPromptXML_MultipleSkillsOrder(t *testing.T) {
	t.Parallel()
	skills := []*Skill{
		{Name: "alpha", Description: "First.", SkillFilePath: "/a/SKILL.md"},
		{Name: "beta", Description: "Second.", SkillFilePath: "/b/SKILL.md"},
		{Name: "gamma", Description: "Third.", SkillFilePath: "/c/SKILL.md"},
	}
	xml := ToPromptXML(skills)

	// Verify order: alpha appears before beta, beta before gamma.
	alphaIdx := strings.Index(xml, "alpha")
	betaIdx := strings.Index(xml, "beta")
	gammaIdx := strings.Index(xml, "gamma")
	assert.Greater(t, betaIdx, alphaIdx)
	assert.Greater(t, gammaIdx, betaIdx)
}

// ---------------------------------------------------------------------------
// escape function test
// ---------------------------------------------------------------------------

func TestEscape(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "&amp;", escape("&"))
	assert.Equal(t, "&lt;", escape("<"))
	assert.Equal(t, "&gt;", escape(">"))
	assert.Equal(t, "&quot;", escape("\""))
	assert.Equal(t, "&apos;", escape("'"))
	assert.Equal(t, "hello", escape("hello"))
	assert.Equal(t, "&lt;tag attr=&quot;val&quot;&gt;", escape(`<tag attr="val">`))
}

// ---------------------------------------------------------------------------
// Parse sets Path and SkillFilePath
// ---------------------------------------------------------------------------

func TestParse_SetsPathFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nname: test\ndescription: d\n---\nBody"), 0o644))

	skill, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, dir, skill.Path)
	assert.Equal(t, path, skill.SkillFilePath)
}

func TestParse_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := Parse("/nonexistent/SKILL.md")
	require.Error(t, err)
}
