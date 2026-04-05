package cmd

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/crush/internal/app"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/event"
	"github.com/charmbracelet/crush/internal/projects"
	"github.com/charmbracelet/crush/internal/ui/common"
	ui "github.com/charmbracelet/crush/internal/ui/model"
	"github.com/charmbracelet/crush/internal/version"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/charmtone"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.PersistentFlags().StringP("cwd", "c", "", "Current working directory")
	rootCmd.PersistentFlags().StringP("data-dir", "D", "", "Custom smithers-tui data directory")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Debug")
	rootCmd.Flags().BoolP("help", "h", false, "Help")
	rootCmd.Flags().BoolP("yolo", "y", false, "Automatically accept all permissions (dangerous mode)")
	rootCmd.Flags().StringP("session", "s", "", "Continue a previous session by ID")
	rootCmd.Flags().BoolP("continue", "C", false, "Continue the most recent session")
	rootCmd.MarkFlagsMutuallyExclusive("session", "continue")

	rootCmd.AddCommand(
		runCmd,
		dirsCmd,
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
	Use:   "smithers-tui",
	Short: "A terminal-first AI assistant for software development",
	Long:  "A glamorous, terminal-first AI assistant for software development and adjacent tasks",
	Example: `
# Run in interactive mode
smithers-tui

# Run non-interactively
smithers-tui run "Guess my 5 favorite Pokémon"

# Run a non-interactively with pipes and redirection
cat README.md | smithers-tui run "make this more glamorous" > GLAMOROUS_README.md

# Run with debug logging in a specific directory
smithers-tui --debug --cwd /path/to/project

# Run in yolo mode (auto-accept all permissions; use with care)
smithers-tui --yolo

# Run with custom data directory
smithers-tui --data-dir /path/to/custom/.smithers-tui

# Continue a previous session
smithers-tui --session {session-id}

# Continue the most recent session
smithers-tui --continue
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID, _ := cmd.Flags().GetString("session")
		continueLast, _ := cmd.Flags().GetBool("continue")

		app, err := setupAppWithProgressBar(cmd)
		if err != nil {
			return err
		}
		defer app.Shutdown()

		// Resolve session ID if provided
		if sessionID != "" {
			sess, err := resolveSessionID(cmd.Context(), app.Sessions, sessionID)
			if err != nil {
				return err
			}
			sessionID = sess.ID
		}

		event.AppInitialized()

		// Set up the TUI.
		var env uv.Environ = os.Environ()

		com := common.DefaultCommon(app)
		model := ui.New(com, sessionID, continueLast)

		program := tea.NewProgram(
			model,
			tea.WithEnvironment(env),
			tea.WithContext(cmd.Context()),
			tea.WithFilter(ui.MouseEventFilter), // Filter mouse events based on focus state
		)
		go app.Subscribe(program)

		if _, err := program.Run(); err != nil {
			event.Error(err)
			slog.Error("TUI run error", "error", err)
			return errors.New("Crush crashed. If metrics are enabled, we were notified about it. If you'd like to report it, please copy the stacktrace above and open an issue at https://github.com/charmbracelet/crush/issues/new?template=bug.yml") //nolint:staticcheck
		}
		return nil
	},
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

func setupAppWithProgressBar(cmd *cobra.Command) (*app.App, error) {
	app, err := setupApp(cmd)
	if err != nil {
		return nil, err
	}

	// Check if progress bar is enabled in config (defaults to true if nil)
	progressEnabled := app.Config().Options.Progress == nil || *app.Config().Options.Progress
	if progressEnabled && supportsProgressBar() {
		_, _ = fmt.Fprintf(os.Stderr, ansi.SetIndeterminateProgressBar)
		defer func() { _, _ = fmt.Fprintf(os.Stderr, ansi.ResetProgressBar) }()
	}

	return app, nil
}

// setupApp handles the common setup logic for both interactive and non-interactive modes.
// It returns the app instance, config, cleanup function, and any error.
func setupApp(cmd *cobra.Command) (*app.App, error) {
	debug, _ := cmd.Flags().GetBool("debug")
	yolo, _ := cmd.Flags().GetBool("yolo")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	ctx := cmd.Context()

	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return nil, err
	}

	store, err := config.Init(cwd, dataDir, debug)
	if err != nil {
		return nil, err
	}

	cfg := store.Config()
	if cfg.Permissions == nil {
		cfg.Permissions = &config.Permissions{}
	}
	cfg.Permissions.SkipRequests = yolo

	if err := createDataDir(cfg.Options.DataDirectory); err != nil {
		return nil, err
	}

	// Register this project in the centralized projects list.
	if err := projects.Register(cwd, cfg.Options.DataDirectory); err != nil {
		slog.Warn("Failed to register project", "error", err)
		// Non-fatal: continue even if registration fails
	}

	// Connect to DB; this will also run migrations.
	conn, err := db.Connect(ctx, cfg.Options.DataDirectory)
	if err != nil {
		return nil, err
	}

	appInstance, err := app.New(ctx, conn, store)
	if err != nil {
		slog.Error("Failed to create app instance", "error", err)
		return nil, err
	}

	if shouldEnableMetrics(cfg) {
		event.Init()
	}

	return appInstance, nil
}

// envWithFallback returns the value of the primary env var, falling back to
// the legacy CRUSH_* name if unset. A warning is logged when the legacy name
// is used so operators can migrate.
// TODO(smithers-tui): remove CRUSH_* fallback after v1.0
func envWithFallback(primary, legacy string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	if v := os.Getenv(legacy); v != "" {
		slog.Warn("Using legacy environment variable; please migrate to the new name",
			"legacy", legacy, "replacement", primary)
		return v
	}
	return ""
}

func shouldEnableMetrics(cfg *config.Config) bool {
	if v, _ := strconv.ParseBool(envWithFallback("SMITHERS_TUI_DISABLE_METRICS", "CRUSH_DISABLE_METRICS")); v {
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
