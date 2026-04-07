package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func captureMigrationLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(prev)
	})
	return &buf
}

func TestEnvWithFallbackLogsLegacyWarning(t *testing.T) {
	buf := captureMigrationLogs(t)
	t.Setenv("CODEPLANE_TEST_PRIMARY", "")
	t.Setenv("CRUSH_TEST_LEGACY", "legacy-value")

	got := envWithFallback("CODEPLANE_TEST_PRIMARY", "CRUSH_TEST_LEGACY")

	require.Equal(t, "legacy-value", got)
	require.Contains(t, buf.String(), "Using legacy environment variable")
	require.Contains(t, buf.String(), "CRUSH_TEST_LEGACY")
	require.Contains(t, buf.String(), "CODEPLANE_TEST_PRIMARY")
}

func TestGlobalConfigLogsLegacyPathWarning(t *testing.T) {
	buf := captureMigrationLogs(t)
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("CODEPLANE_GLOBAL_CONFIG", "")
	t.Setenv("SMITHERS_TUI_GLOBAL_CONFIG", "")
	t.Setenv("CRUSH_GLOBAL_CONFIG", "")

	legacyPath := filepath.Join(cfgHome, "crush", "crush.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.WriteFile(legacyPath, []byte(`{}`), 0o644))

	got := GlobalConfig()

	require.Equal(t, legacyPath, got)
	require.Contains(t, buf.String(), "Using legacy global_config path")
	require.Contains(t, buf.String(), legacyPath)
	require.Contains(t, buf.String(), filepath.Join(cfgHome, "codeplane", "codeplane.json"))
}

func TestGlobalConfigDataLogsLegacyPathWarning(t *testing.T) {
	buf := captureMigrationLogs(t)
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("CODEPLANE_GLOBAL_DATA", "")
	t.Setenv("SMITHERS_TUI_GLOBAL_DATA", "")
	t.Setenv("CRUSH_GLOBAL_DATA", "")

	legacyPath := filepath.Join(dataHome, "crush", "crush.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.WriteFile(legacyPath, []byte(`{}`), 0o644))

	got := GlobalConfigData()

	require.Equal(t, legacyPath, got)
	require.Contains(t, buf.String(), "Using legacy global_data path")
	require.Contains(t, buf.String(), legacyPath)
	require.Contains(t, buf.String(), filepath.Join(dataHome, "codeplane", "codeplane.json"))
}

func TestWorkspaceConfigPathLogsLegacyPathWarning(t *testing.T) {
	buf := captureMigrationLogs(t)
	dataDir := t.TempDir()
	legacyPath := filepath.Join(dataDir, "crush.json")
	require.NoError(t, os.WriteFile(legacyPath, []byte(`{}`), 0o644))

	got := workspaceConfigPath(dataDir)

	require.Equal(t, legacyPath, got)
	require.Contains(t, buf.String(), "Using legacy workspace_config path")
	require.Contains(t, buf.String(), legacyPath)
	require.Contains(t, buf.String(), filepath.Join(dataDir, "codeplane.json"))
}

func TestLoadFromConfigPathsLogsLegacyConfigFiles(t *testing.T) {
	buf := captureMigrationLogs(t)
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "crush.json")
	require.NoError(t, os.WriteFile(legacyPath, []byte(`{"options":{"data_directory":".crush"}}`), 0o644))

	cfg, err := loadFromConfigPaths([]string{legacyPath})

	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Contains(t, buf.String(), "Loaded legacy config file")
	require.True(t, strings.Contains(buf.String(), legacyPath))
}

func TestConfigStoreSetConfigFieldWritesCodeplanePathOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	globalDir := filepath.Join(dir, "codeplane")
	legacyDir := filepath.Join(dir, "crush")
	globalPath := filepath.Join(globalDir, "codeplane.json")
	legacyPath := filepath.Join(legacyDir, "crush.json")

	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o755))
	require.NoError(t, os.WriteFile(legacyPath, []byte(`{"legacy":true}`), 0o644))

	store := &ConfigStore{
		config:         &Config{},
		globalDataPath: globalPath,
	}

	require.NoError(t, store.SetConfigField(ScopeGlobal, "foo", "bar"))

	codeplaneData, err := os.ReadFile(globalPath)
	require.NoError(t, err)
	require.Contains(t, string(codeplaneData), `"foo":"bar"`)

	legacyData, err := os.ReadFile(legacyPath)
	require.NoError(t, err)
	require.Equal(t, `{"legacy":true}`, string(legacyData))
}
