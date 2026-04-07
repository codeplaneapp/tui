package cmd

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	fang "charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/crush/internal/app"
	"github.com/charmbracelet/crush/internal/client"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/event"
	crushlog "github.com/charmbracelet/crush/internal/log"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/charmbracelet/crush/internal/projects"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/server"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/ui/common"
	ui "github.com/charmbracelet/crush/internal/ui/model"
	"github.com/charmbracelet/crush/internal/version"
	"github.com/charmbracelet/crush/internal/workspace"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/charmtone"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
)

var clientHost string

func init() {
	rootCmd.PersistentFlags().StringP("cwd", "c", "", "Current working directory")
	rootCmd.PersistentFlags().StringP("data-dir", "D", "", "Custom Codeplane data directory")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Debug")
	rootCmd.PersistentFlags().StringVarP(&clientHost, "host", "H", server.DefaultHost(), "Connect to a specific Codeplane server host (for advanced users)")
	rootCmd.Flags().BoolP("help", "h", false, "Help")
	rootCmd.Flags().BoolP("yolo", "y", false, "Automatically accept all permissions (dangerous mode)")
	rootCmd.Flags().StringP("session", "s", "", "Continue a previous session by ID")
	rootCmd.Flags().BoolP("continue", "C", false, "Continue the most recent session")
	rootCmd.MarkFlagsMutuallyExclusive("session", "continue")

	rootCmd.AddCommand(
		tuiCmd,
		runCmd,
		dirsCmd,
		workspaceCmd,
		projectsCmd,
		updateProvidersCmd,
		logsCmd,
		schemaCmd,
		loginCmd,
		statsCmd,
		sessionCmd,
	)
}

var rootCmd = &cobra.Command{
	Use:   "codeplane",
	Short: "A terminal-first AI assistant for software development",
	Long:  "A glamorous, terminal-first AI assistant for software development and adjacent tasks",
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		slog.Info("Running Codeplane command", "command", cmd.CommandPath())
	},
	Example: `
# Run in interactive mode
codeplane

# Run non-interactively
codeplane run "Guess my 5 favorite Pokémon"

# Run a non-interactively with pipes and redirection
cat README.md | codeplane run "make this more glamorous" > GLAMOROUS_README.md

# Run with debug logging in a specific directory
codeplane --debug --cwd /path/to/project

# Run in yolo mode (auto-accept all permissions; use with care)
codeplane --yolo

# Run with custom data directory
codeplane --data-dir /path/to/custom/.codeplane

# Continue a previous session
codeplane --session {session-id}

# Continue the most recent session
codeplane --continue
  `,
	RunE: runInteractive,
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the Codeplane terminal UI",
	RunE:  runInteractive,
}

var heartbit = lipgloss.NewStyle().Foreground(charmtone.Dolly).SetString(`
    ▄▄▄▄▄▄▄▄    ▄▄▄▄▄▄▄▄
  ███████████  ███████████
████████████████████████████
████████████████████████████
██████████▀██████▀██████████
██████████ ██████ ██████████
▀▀██████▄████▄▄████▄██████▀▀
  ████████████████████████
    ████████████████████
       ▀▀██████████▀▀
           ▀▀▀▀▀▀
`)

// copied from cobra:
const defaultVersionTemplate = `{{with .DisplayName}}{{printf "%s " .}}{{end}}{{printf "version %s" .Version}}
`

func Execute() {
	crushlog.Setup("", initialDebugLoggingEnabled(), os.Stderr)

	// NOTE: very hacky: we create a colorprofile writer with STDOUT, then make
	// it forward to a bytes.Buffer, write the colored heartbit to it, and then
	// finally prepend it in the version template.
	// Unfortunately cobra doesn't give us a way to set a function to handle
	// printing the version, and PreRunE runs after the version is already
	// handled, so that doesn't work either.
	// This is the only way I could find that works relatively well.
	if term.IsTerminal(os.Stdout.Fd()) {
		var b bytes.Buffer
		w := colorprofile.NewWriter(os.Stdout, os.Environ())
		w.Forward = &b
		_, _ = w.WriteString(heartbit.String())
		rootCmd.SetVersionTemplate(b.String() + "\n" + defaultVersionTemplate)
	}
	if err := fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithVersion(version.Version),
		fang.WithNotifySignal(os.Interrupt),
	); err != nil {
		os.Exit(1)
	}
}

func runInteractive(cmd *cobra.Command, _ []string) error {
	sessionID, _ := cmd.Flags().GetString("session")
	continueLast, _ := cmd.Flags().GetBool("continue")

	ws, cleanup, err := setupWorkspaceWithProgressBar(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	if sessionID != "" {
		sess, err := resolveWorkspaceSessionID(cmd.Context(), ws, sessionID)
		if err != nil {
			return err
		}
		sessionID = sess.ID
	}

	event.AppInitialized()

	com := common.DefaultCommon(ws)
	model := ui.New(com, sessionID, continueLast)
	observability.RecordStartupFlow("entrypoint", "tui", "ok")

	var env uv.Environ = os.Environ()
	program := tea.NewProgram(
		model,
		tea.WithEnvironment(env),
		tea.WithContext(cmd.Context()),
		tea.WithFilter(ui.MouseEventFilter),
	)
	go ws.Subscribe(program)

	if _, err := program.Run(); err != nil {
		event.Error(err)
		slog.Error("TUI run error", "error", err)
		return errors.New("Codeplane crashed. If metrics are enabled, we were notified about it. If you'd like to report it, please copy the stacktrace above and open an issue at https://github.com/charmbracelet/crush/issues/new?template=bug.yml") //nolint:staticcheck
	}
	return nil
}

func initialDebugLoggingEnabled() bool {
	for _, arg := range os.Args[1:] {
		if arg == "-d" || arg == "--debug" {
			return true
		}
	}
	return false
}

// supportsProgressBar tries to determine whether the current terminal supports
// progress bars by looking into environment variables.
func supportsProgressBar() bool {
	if !term.IsTerminal(os.Stderr.Fd()) {
		return false
	}
	termProg := os.Getenv("TERM_PROGRAM")
	_, isWindowsTerminal := os.LookupEnv("WT_SESSION")

	return isWindowsTerminal || strings.Contains(strings.ToLower(termProg), "ghostty")
}

// useClientServer returns true when the client/server architecture is
// enabled via the CODEPLANE_CLIENT_SERVER environment variable.
func useClientServer() bool {
	v, _ := strconv.ParseBool(envWithFallback("CODEPLANE_CLIENT_SERVER", "SMITHERS_TUI_CLIENT_SERVER", "CRUSH_CLIENT_SERVER"))
	return v
}

// setupWorkspaceWithProgressBar wraps setupWorkspace with an optional
// terminal progress bar shown during initialization.
func setupWorkspaceWithProgressBar(cmd *cobra.Command) (workspace.Workspace, func(), error) {
	showProgress := supportsProgressBar()
	if showProgress {
		_, _ = fmt.Fprintf(os.Stderr, ansi.SetIndeterminateProgressBar)
	}

	ws, cleanup, err := setupWorkspace(cmd)

	if showProgress {
		_, _ = fmt.Fprintf(os.Stderr, ansi.ResetProgressBar)
	}

	return ws, cleanup, err
}

// setupWorkspace returns a Workspace and cleanup function. When
// CODEPLANE_CLIENT_SERVER=1, it connects to a server process and returns a
// ClientWorkspace. Otherwise it creates an in-process app.App and returns an
// AppWorkspace.
func setupWorkspace(cmd *cobra.Command) (workspace.Workspace, func(), error) {
	if useClientServer() {
		return setupClientServerWorkspace(cmd)
	}
	return setupLocalWorkspace(cmd)
}

// setupLocalWorkspace creates an in-process app.App and wraps it in an
// AppWorkspace.
func setupLocalWorkspace(cmd *cobra.Command) (workspace.Workspace, func(), error) {
	debug, _ := cmd.Flags().GetBool("debug")
	yolo, _ := cmd.Flags().GetBool("yolo")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	ctx := cmd.Context()

	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return nil, nil, err
	}

	store, err := config.Init(cwd, dataDir, debug)
	if err != nil {
		return nil, nil, err
	}

	cfg := store.Config()
	store.Overrides().SkipPermissionRequests = yolo

	if err := os.MkdirAll(cfg.Options.DataDirectory, 0o700); err != nil {
		return nil, nil, fmt.Errorf("failed to create data directory: %q %w", cfg.Options.DataDirectory, err)
	}

	if err := createDataDir(cfg.Options.DataDirectory); err != nil {
		return nil, nil, err
	}

	if err := projects.Register(cwd, cfg.Options.DataDirectory); err != nil {
		slog.Warn("Failed to register project", "error", err)
	}

	conn, err := db.Connect(ctx, cfg.Options.DataDirectory)
	if err != nil {
		return nil, nil, err
	}

	logFile := filepath.Join(cfg.Options.DataDirectory, "logs", "codeplane.log")
	crushlog.Setup(logFile, debug)
	if err := configureObservability(ctx, cfg, observability.ModeLocal, true); err != nil {
		return nil, nil, err
	}
	observability.RecordStartupFlow("workspace_mode", "local", "ok",
		attribute.String("codeplane.cwd", cwd),
		attribute.String("codeplane.data_dir", cfg.Options.DataDirectory),
	)
	recordConfigSelections(store)

	appInstance, err := app.New(ctx, conn, store)
	if err != nil {
		_ = conn.Close()
		slog.Error("Failed to create app instance", "error", err)
		return nil, nil, err
	}

	if shouldEnableMetrics(cfg) {
		event.Init()
	}

	ws := workspace.NewAppWorkspace(appInstance, store)
	cleanup := func() {
		appInstance.Shutdown()
		shutdownObservability()
	}
	return ws, cleanup, nil
}

// setupClientServerWorkspace connects to a server process and wraps the
// result in a ClientWorkspace.
func setupClientServerWorkspace(cmd *cobra.Command) (workspace.Workspace, func(), error) {
	c, protoWs, cleanupServer, err := connectToServer(cmd)
	if err != nil {
		return nil, nil, err
	}

	clientWs := workspace.NewClientWorkspace(c, *protoWs)

	if protoWs.Config.IsConfigured() {
		if err := clientWs.InitCoderAgent(cmd.Context()); err != nil {
			slog.Error("Failed to initialize coder agent", "error", err)
		}
	}
	observability.RecordStartupFlow("workspace_mode", "client_server", "ok",
		attribute.String("codeplane.workspace_id", protoWs.ID),
	)

	return clientWs, cleanupServer, nil
}

// connectToServer ensures the server is running, creates a client and
// workspace, and returns a cleanup function that deletes the workspace.
func connectToServer(cmd *cobra.Command) (*client.Client, *proto.Workspace, func(), error) {
	hostURL, err := server.ParseHostURL(clientHost)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid host URL: %v", err)
	}

	if err := ensureServer(cmd, hostURL); err != nil {
		return nil, nil, nil, err
	}

	debug, _ := cmd.Flags().GetBool("debug")
	yolo, _ := cmd.Flags().GetBool("yolo")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	ctx := cmd.Context()

	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return nil, nil, nil, err
	}

	c, err := client.NewClient(cwd, hostURL.Scheme, hostURL.Host)
	if err != nil {
		return nil, nil, nil, err
	}

	wsReq := proto.Workspace{
		Path:    cwd,
		DataDir: dataDir,
		Debug:   debug,
		YOLO:    yolo,
		Version: version.Version,
		Env:     os.Environ(),
	}

	ws, err := c.CreateWorkspace(ctx, wsReq)
	if err != nil {
		// The server socket may exist before the HTTP handler is ready.
		// Retry a few times with a short backoff.
		for range 5 {
			select {
			case <-ctx.Done():
				return nil, nil, nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
			ws, err = c.CreateWorkspace(ctx, wsReq)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create workspace: %v", err)
		}
	}

	if shouldEnableMetrics(ws.Config) {
		event.Init()
	}

	if ws.Config != nil {
		logFile := filepath.Join(ws.Config.Options.DataDirectory, "logs", "codeplane.log")
		crushlog.Setup(logFile, debug)
		if err := configureObservability(ctx, ws.Config, observability.ModeClient, false); err != nil {
			return nil, nil, nil, err
		}
	}

	cleanup := func() {
		_ = c.DeleteWorkspace(context.Background(), ws.ID)
		shutdownObservability()
	}
	return c, ws, cleanup, nil
}

// ensureServer auto-starts a detached server if the socket file does not
// exist. When the socket exists, it verifies that the running server
// version matches the client; on mismatch it shuts down the old server
// and starts a fresh one.
func ensureServer(cmd *cobra.Command, hostURL *url.URL) error {
	switch hostURL.Scheme {
	case "unix", "npipe":
		needsStart := false
		if _, err := os.Stat(hostURL.Host); err != nil && errors.Is(err, fs.ErrNotExist) {
			needsStart = true
		} else if err == nil {
			if err := restartIfStale(cmd, hostURL); err != nil {
				slog.Warn("Failed to check server version, restarting", "error", err)
				needsStart = true
			}
		}

		if needsStart {
			startedAt := time.Now()
			if err := startDetachedServer(cmd); err != nil {
				observability.RecordStartupFlow("server_autostart", hostURL.Scheme, "error",
					attribute.String("codeplane.server.host", hostURL.Host),
					attribute.String("codeplane.error", err.Error()),
				)
				return err
			}
			observability.RecordStartupFlow("server_autostart", hostURL.Scheme, "ok",
				attribute.String("codeplane.server.host", hostURL.Host),
				attribute.Int64("codeplane.duration_ms", time.Since(startedAt).Milliseconds()),
			)
		}

		var err error
		for range 10 {
			_, err = os.Stat(hostURL.Host)
			if err == nil {
				break
			}
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
		if err != nil {
			return fmt.Errorf("failed to initialize Codeplane server: %v", err)
		}
	}

	return nil
}

// restartIfStale checks whether the running server matches the current
// client version. When they differ, it sends a shutdown command and
// removes the stale socket so the caller can start a fresh server.
func restartIfStale(cmd *cobra.Command, hostURL *url.URL) error {
	c, err := client.NewClient("", hostURL.Scheme, hostURL.Host)
	if err != nil {
		return err
	}
	vi, err := c.VersionInfo(cmd.Context())
	if err != nil {
		return err
	}
	if vi.Version == version.Version {
		return nil
	}
	slog.Info("Server version mismatch, restarting",
		"server", vi.Version,
		"client", version.Version,
	)
	startedAt := time.Now()
	_ = c.ShutdownServer(cmd.Context())
	// Give the old process a moment to release the socket.
	for range 20 {
		if _, err := os.Stat(hostURL.Host); errors.Is(err, fs.ErrNotExist) {
			break
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	// Force-remove if the socket is still lingering.
	_ = os.Remove(hostURL.Host)
	observability.RecordStartupFlow("server_restart", hostURL.Scheme, "ok",
		attribute.String("codeplane.server.host", hostURL.Host),
		attribute.String("codeplane.server.version", vi.Version),
		attribute.String("codeplane.client.version", version.Version),
		attribute.Int64("codeplane.duration_ms", time.Since(startedAt).Milliseconds()),
	)
	return nil
}

var safeNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func startDetachedServer(cmd *cobra.Command) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	safeClientHost := safeNameRegexp.ReplaceAllString(clientHost, "_")
	chDir := filepath.Join(config.GlobalCacheDir(), "server-"+safeClientHost)
	if err := os.MkdirAll(chDir, 0o700); err != nil {
		return fmt.Errorf("failed to create server working directory: %v", err)
	}

	cmdArgs := []string{"server"}
	if clientHost != server.DefaultHost() {
		cmdArgs = append(cmdArgs, "--host", clientHost)
	}

	c := exec.CommandContext(cmd.Context(), exe, cmdArgs...)
	stdoutPath := filepath.Join(chDir, "stdout.log")
	stderrPath := filepath.Join(chDir, "stderr.log")
	detachProcess(c)

	stdout, err := os.Create(stdoutPath)
	if err != nil {
		return fmt.Errorf("failed to create stdout log file: %v", err)
	}
	defer stdout.Close()
	c.Stdout = stdout

	stderr, err := os.Create(stderrPath)
	if err != nil {
		return fmt.Errorf("failed to create stderr log file: %v", err)
	}
	defer stderr.Close()
	c.Stderr = stderr

	if err := c.Start(); err != nil {
		return fmt.Errorf("failed to start Codeplane server: %v", err)
	}

	if err := c.Process.Release(); err != nil {
		return fmt.Errorf("failed to detach Codeplane server process: %v", err)
	}

	return nil
}

// envWithFallback returns the value of the primary env var, falling back to
// legacy names if unset. A warning is logged when a legacy name is used so
// operators can migrate.
// TODO(codeplane): remove SMITHERS_TUI_* and CRUSH_* fallbacks after v1.0.
func envWithFallback(primary string, legacy ...string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	for _, name := range legacy {
		if v := os.Getenv(name); v != "" {
			slog.Warn("Using legacy environment variable; please migrate to the new name",
				"legacy", name, "replacement", primary)
			return v
		}
	}
	return ""
}

func configureObservability(ctx context.Context, cfg *config.Config, mode observability.Mode, enableHTTPServer bool) error {
	if cfg == nil || cfg.Options == nil || cfg.Options.Observability == nil {
		return nil
	}

	obs := cfg.Options.Observability
	sampleRatio := 1.0
	if obs.TraceSampleRatio != nil {
		sampleRatio = *obs.TraceSampleRatio
	}
	insecure := false
	if obs.OTLPInsecure != nil {
		insecure = *obs.OTLPInsecure
	}

	return observability.Configure(ctx, observability.Config{
		ServiceName:      "codeplane",
		ServiceVersion:   version.Version,
		Mode:             mode,
		DebugServerAddr:  obs.Address,
		EnableHTTPServer: enableHTTPServer,
		TraceBufferSize:  obs.TraceBufferSize,
		TraceSampleRatio: sampleRatio,
		OTLPEndpoint:     obs.OTLPEndpoint,
		OTLPHeaders:      obs.OTLPHeaders,
		OTLPInsecure:     insecure,
	})
}

func shutdownObservability() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := observability.Shutdown(ctx); err != nil {
		slog.Error("Failed to shutdown observability", "error", err)
	}
}

func shouldEnableMetrics(cfg *config.Config) bool {
	if v, _ := strconv.ParseBool(envWithFallback("CODEPLANE_DISABLE_METRICS", "SMITHERS_TUI_DISABLE_METRICS", "CRUSH_DISABLE_METRICS")); v {
		return false
	}
	if v, _ := strconv.ParseBool(os.Getenv("DO_NOT_TRACK")); v {
		return false
	}
	if cfg.Options.DisableMetrics {
		return false
	}
	return true
}

func MaybePrependStdin(prompt string) (string, error) {
	if term.IsTerminal(os.Stdin.Fd()) {
		return prompt, nil
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return prompt, err
	}
	// Check if stdin is a named pipe ( | ) or regular file ( < ).
	if fi.Mode()&os.ModeNamedPipe == 0 && !fi.Mode().IsRegular() {
		return prompt, nil
	}
	bts, err := io.ReadAll(os.Stdin)
	if err != nil {
		return prompt, err
	}
	return string(bts) + "\n\n" + prompt, nil
}

// resolveWorkspaceSessionID resolves a session ID that may be a full
// UUID, full hash, or hash prefix. Works against the Workspace
// interface so both local and client/server paths get hash prefix
// support.
func resolveWorkspaceSessionID(ctx context.Context, ws workspace.Workspace, id string) (session.Session, error) {
	if sess, err := ws.GetSession(ctx, id); err == nil {
		return sess, nil
	}

	sessions, err := ws.ListSessions(ctx)
	if err != nil {
		return session.Session{}, err
	}

	var matches []session.Session
	for _, s := range sessions {
		hash := session.HashID(s.ID)
		if hash == id || strings.HasPrefix(hash, id) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return session.Session{}, fmt.Errorf("session not found: %s", id)
	case 1:
		return matches[0], nil
	default:
		return session.Session{}, fmt.Errorf("session ID %q is ambiguous (%d matches)", id, len(matches))
	}
}

func ResolveCwd(cmd *cobra.Command) (string, error) {
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd != "" {
		err := os.Chdir(cwd)
		if err != nil {
			return "", fmt.Errorf("failed to change directory: %v", err)
		}
		return cwd, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %v", err)
	}
	return cwd, nil
}

func recordConfigSelections(store *config.ConfigStore) {
	if store == nil {
		return
	}
	recordConfigSelection("global_config", config.GlobalConfig())
	recordConfigSelection("global_data", store.GlobalDataPath())
	recordConfigSelection("workspace_config", store.WorkspaceConfigPath())
}

func recordConfigSelection(kind, path string) {
	if strings.TrimSpace(path) == "" {
		return
	}

	result := "codeplane"
	if config.IsLegacyConfigPath(path) {
		result = "legacy"
	}

	slog.Info("Resolved Codeplane config source",
		"kind", kind,
		"path", path,
		"legacy", result == "legacy",
	)
	observability.RecordStartupFlow("config_source", kind, result,
		attribute.String("codeplane.config.path", path),
	)
}

func createDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %q %w", dir, err)
	}

	gitIgnorePath := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(gitIgnorePath)

	// create or update if old version
	if os.IsNotExist(err) || string(content) == oldGitIgnore {
		if err := os.WriteFile(gitIgnorePath, []byte(defaultGitIgnore), 0o644); err != nil {
			return fmt.Errorf("failed to create .gitignore file: %q %w", gitIgnorePath, err)
		}
	}

	return nil
}

//go:embed gitignore/old
var oldGitIgnore string

//go:embed gitignore/default
var defaultGitIgnore string
