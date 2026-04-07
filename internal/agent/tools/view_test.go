package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadTextFileBoundaryCases(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.txt")

	var allLines []string
	for i := range 5 {
		allLines = append(allLines, fmt.Sprintf("line %d", i+1))
	}
	require.NoError(t, os.WriteFile(filePath, []byte(strings.Join(allLines, "\n")), 0o644))

	tests := []struct {
		name        string
		offset      int
		limit       int
		wantContent string
		wantHasMore bool
	}{
		{
			name:        "exactly limit lines remaining",
			offset:      0,
			limit:       5,
			wantContent: "line 1\nline 2\nline 3\nline 4\nline 5",
			wantHasMore: false,
		},
		{
			name:        "limit plus one line remaining",
			offset:      0,
			limit:       4,
			wantContent: "line 1\nline 2\nline 3\nline 4",
			wantHasMore: true,
		},
		{
			name:        "offset at last line",
			offset:      4,
			limit:       3,
			wantContent: "line 5",
			wantHasMore: false,
		},
		{
			name:        "offset beyond eof",
			offset:      10,
			limit:       3,
			wantContent: "",
			wantHasMore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotContent, gotHasMore, err := readTextFile(filePath, tt.offset, tt.limit)
			require.NoError(t, err)
			require.Equal(t, tt.wantContent, gotContent)
			require.Equal(t, tt.wantHasMore, gotHasMore)
		})
	}
}

func TestReadTextFileTruncatesLongLines(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "longline.txt")

	longLine := strings.Repeat("a", MaxLineLength+10)
	require.NoError(t, os.WriteFile(filePath, []byte(longLine), 0o644))

	content, hasMore, err := readTextFile(filePath, 0, 1)
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Equal(t, strings.Repeat("a", MaxLineLength)+"...", content)
}

func TestAddLineNumbers(t *testing.T) {
	t.Parallel()

	t.Run("empty content returns empty", func(t *testing.T) {
		t.Parallel()
		result := addLineNumbers("", 1)
		require.Equal(t, "", result)
	})

	t.Run("single line starting at 1", func(t *testing.T) {
		t.Parallel()
		result := addLineNumbers("hello", 1)
		require.Equal(t, "     1|hello", result)
	})

	t.Run("multiple lines with offset", func(t *testing.T) {
		t.Parallel()
		result := addLineNumbers("line a\nline b\nline c", 10)
		lines := strings.Split(result, "\n")
		require.Len(t, lines, 3)
		require.Contains(t, lines[0], "10|line a")
		require.Contains(t, lines[1], "11|line b")
		require.Contains(t, lines[2], "12|line c")
	})

	t.Run("large line numbers exceed padding width", func(t *testing.T) {
		t.Parallel()
		// Line number >= 6 digits should not be padded
		result := addLineNumbers("content", 1000000)
		require.Equal(t, "1000000|content", result)
	})

	t.Run("carriage return stripped from lines", func(t *testing.T) {
		t.Parallel()
		result := addLineNumbers("hello\r\nworld\r", 1)
		lines := strings.Split(result, "\n")
		require.Len(t, lines, 2)
		// Neither line should end with \r
		for _, line := range lines {
			require.False(t, strings.HasSuffix(line, "\r"))
		}
	})
}

func TestGetImageMimeType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filePath string
		wantOk   bool
		wantMime string
	}{
		{"jpeg", "photo.jpg", true, "image/jpeg"},
		{"jpeg uppercase", "PHOTO.JPEG", true, "image/jpeg"},
		{"png", "icon.png", true, "image/png"},
		{"gif", "anim.gif", true, "image/gif"},
		{"webp", "photo.webp", true, "image/webp"},
		{"go file", "main.go", false, ""},
		{"no extension", "README", false, ""},
		{"svg not supported", "icon.svg", false, ""},
		{"empty path", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotOk, gotMime := getImageMimeType(tt.filePath)
			require.Equal(t, tt.wantOk, gotOk)
			require.Equal(t, tt.wantMime, gotMime)
		})
	}
}

func TestIsInSkillsPath(t *testing.T) {
	t.Parallel()

	t.Run("returns false with empty skills paths", func(t *testing.T) {
		t.Parallel()
		result := isInSkillsPath("/some/file.go", nil)
		require.False(t, result)
	})

	t.Run("returns true for file within skills dir", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))

		filePath := filepath.Join(skillDir, "SKILL.md")
		require.NoError(t, os.WriteFile(filePath, []byte("# Skill"), 0o644))

		result := isInSkillsPath(filePath, []string{skillDir})
		require.True(t, result)
	})

	t.Run("returns false for file outside skills dir", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))

		otherFile := filepath.Join(tmpDir, "other.go")
		require.NoError(t, os.WriteFile(otherFile, []byte("package main"), 0o644))

		result := isInSkillsPath(otherFile, []string{skillDir})
		require.False(t, result)
	})
}

func TestReadBuiltinFile(t *testing.T) {
	t.Parallel()

	t.Run("reads codeplane-config skill", func(t *testing.T) {
		t.Parallel()

		resp, err := readBuiltinFile(ViewParams{
			FilePath: "codeplane://skills/codeplane-config/SKILL.md",
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.Content)
		require.Contains(t, resp.Content, "Codeplane Configuration")
	})

	t.Run("reads legacy builtin skill path", func(t *testing.T) {
		t.Parallel()

		resp, err := readBuiltinFile(ViewParams{
			FilePath: "crush://skills/crush-config/SKILL.md",
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.Content)
		require.Contains(t, resp.Content, "Codeplane Configuration")
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		resp, err := readBuiltinFile(ViewParams{
			FilePath: "codeplane://skills/nonexistent/SKILL.md",
		})
		require.NoError(t, err)
		require.True(t, resp.IsError)
	})

	t.Run("metadata has skill info", func(t *testing.T) {
		t.Parallel()

		resp, err := readBuiltinFile(ViewParams{
			FilePath: "codeplane://skills/codeplane-config/SKILL.md",
		})
		require.NoError(t, err)

		var meta ViewResponseMetadata
		require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
		require.Equal(t, ViewResourceSkill, meta.ResourceType)
		require.Equal(t, "codeplane-config", meta.ResourceName)
		require.NotEmpty(t, meta.ResourceDescription)
	})

	t.Run("respects offset", func(t *testing.T) {
		t.Parallel()

		resp, err := readBuiltinFile(ViewParams{
			FilePath: "codeplane://skills/codeplane-config/SKILL.md",
			Offset:   5,
		})
		require.NoError(t, err)
		require.NotContains(t, resp.Content, "     1|")
	})
}
