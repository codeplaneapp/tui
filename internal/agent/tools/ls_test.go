package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateFileTree_SimpleFiles(t *testing.T) {
	root := "/project"
	paths := []string{
		"/project/README.md",
		"/project/main.go",
		"/project/go.mod",
	}

	tree := createFileTree(paths, root)

	require.Len(t, tree, 3)

	assert.Equal(t, "README.md", tree[0].Name)
	assert.Equal(t, NodeTypeFile, tree[0].Type)
	assert.Empty(t, tree[0].Children)

	assert.Equal(t, "main.go", tree[1].Name)
	assert.Equal(t, NodeTypeFile, tree[1].Type)

	assert.Equal(t, "go.mod", tree[2].Name)
	assert.Equal(t, NodeTypeFile, tree[2].Type)
}

func TestCreateFileTree_NestedDirs(t *testing.T) {
	root := "/project"
	paths := []string{
		"/project/cmd/",
		"/project/cmd/main.go",
		"/project/internal/",
		"/project/internal/server/",
		"/project/internal/server/server.go",
		"/project/internal/server/handler.go",
		"/project/README.md",
	}

	tree := createFileTree(paths, root)

	require.Len(t, tree, 3, "expected 3 top-level nodes: cmd/, internal/, README.md")

	// cmd/
	cmd := tree[0]
	assert.Equal(t, "cmd", cmd.Name)
	assert.Equal(t, NodeTypeDirectory, cmd.Type)
	require.Len(t, cmd.Children, 1)
	assert.Equal(t, "main.go", cmd.Children[0].Name)
	assert.Equal(t, NodeTypeFile, cmd.Children[0].Type)

	// internal/
	internal := tree[1]
	assert.Equal(t, "internal", internal.Name)
	assert.Equal(t, NodeTypeDirectory, internal.Type)
	require.Len(t, internal.Children, 1)

	// internal/server/
	server := internal.Children[0]
	assert.Equal(t, "server", server.Name)
	assert.Equal(t, NodeTypeDirectory, server.Type)
	require.Len(t, server.Children, 2)
	assert.Equal(t, "server.go", server.Children[0].Name)
	assert.Equal(t, "handler.go", server.Children[1].Name)

	// README.md
	readme := tree[2]
	assert.Equal(t, "README.md", readme.Name)
	assert.Equal(t, NodeTypeFile, readme.Type)
	assert.Empty(t, readme.Children)
}

func TestCreateFileTree_EmptyPaths(t *testing.T) {
	tree := createFileTree([]string{}, "/project")

	assert.Empty(t, tree)
}

func TestPrintTree_FlatFiles(t *testing.T) {
	tree := []*TreeNode{
		{Name: "go.mod", Path: "go.mod", Type: NodeTypeFile, Children: []*TreeNode{}},
		{Name: "main.go", Path: "main.go", Type: NodeTypeFile, Children: []*TreeNode{}},
		{Name: "README.md", Path: "README.md", Type: NodeTypeFile, Children: []*TreeNode{}},
	}

	output := printTree(tree, "/project")

	assert.Contains(t, output, "- /project/\n")
	assert.Contains(t, output, "  - go.mod\n")
	assert.Contains(t, output, "  - main.go\n")
	assert.Contains(t, output, "  - README.md\n")
}

func TestPrintTree_NestedStructure(t *testing.T) {
	tree := []*TreeNode{
		{
			Name: "src",
			Path: "src",
			Type: NodeTypeDirectory,
			Children: []*TreeNode{
				{
					Name: "lib",
					Path: "src/lib",
					Type: NodeTypeDirectory,
					Children: []*TreeNode{
						{Name: "utils.go", Path: "src/lib/utils.go", Type: NodeTypeFile, Children: []*TreeNode{}},
					},
				},
				{Name: "main.go", Path: "src/main.go", Type: NodeTypeFile, Children: []*TreeNode{}},
			},
		},
		{Name: "README.md", Path: "README.md", Type: NodeTypeFile, Children: []*TreeNode{}},
	}

	output := printTree(tree, "/myproject")

	assert.Contains(t, output, "- /myproject/\n")
	assert.Contains(t, output, "  - src/\n")
	assert.Contains(t, output, "    - lib/\n")
	assert.Contains(t, output, "      - utils.go\n")
	assert.Contains(t, output, "    - main.go\n")
	assert.Contains(t, output, "  - README.md\n")
}

func TestPrintTree_TrailingSlashOnRoot(t *testing.T) {
	tree := []*TreeNode{
		{Name: "file.txt", Path: "file.txt", Type: NodeTypeFile, Children: []*TreeNode{}},
	}

	// Root already has trailing slash — should not double it.
	output := printTree(tree, "/project/")
	assert.Contains(t, output, "- /project/\n")

	// Root without trailing slash — function appends one.
	output2 := printTree(tree, "/project")
	assert.Contains(t, output2, "- /project/\n")
}

func TestListDirectoryTree_RealDir(t *testing.T) {
	dir := t.TempDir()

	// Create a small directory structure:
	//   dir/
	//     alpha.txt
	//     sub/
	//       beta.txt
	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "beta.txt"), []byte("b"), 0o644))

	params := LSParams{}
	lsCfg := config.ToolLs{} // zero-value → defaults

	output, meta, err := ListDirectoryTree(dir, params, lsCfg)
	require.NoError(t, err)

	assert.Contains(t, output, "alpha.txt")
	assert.Contains(t, output, "sub/")
	assert.Contains(t, output, "beta.txt")
	assert.GreaterOrEqual(t, meta.NumberOfFiles, 3) // at least: sub/, alpha.txt, beta.txt (sub/beta.txt)
	assert.False(t, meta.Truncated)
}

func TestListDirectoryTree_NonExistentDir(t *testing.T) {
	params := LSParams{}
	lsCfg := config.ToolLs{}

	_, _, err := ListDirectoryTree("/tmp/surely-does-not-exist-12345xyz", params, lsCfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path does not exist")
}
