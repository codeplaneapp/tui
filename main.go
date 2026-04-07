// Package main is the entry point for the Codeplane CLI.
//
//	@title			Codeplane API
//	@version		1.0
//	@description	Codeplane is a terminal-based AI coding assistant. This API is served over a Unix socket (or Windows named pipe) and provides programmatic access to workspaces, sessions, agents, LSP, MCP, and more.
//	@contact.name	Charm
//	@contact.url	https://charm.sh
//	@license.name	MIT
//	@license.url	https://github.com/charmbracelet/crush/blob/main/LICENSE
//	@BasePath		/v1
package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/charmbracelet/crush/internal/cmd"
	_ "github.com/joho/godotenv/autoload"
)

func profileEnabled() bool {
	for _, key := range []string{"CODEPLANE_PROFILE", "SMITHERS_TUI_PROFILE", "CRUSH_PROFILE"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

func main() {
	if profileEnabled() {
		go func() {
			slog.Info("Serving pprof at localhost:6060")
			if httpErr := http.ListenAndServe("localhost:6060", nil); httpErr != nil {
				slog.Error("Failed to pprof listen", "error", httpErr)
			}
		}()
	}

	cmd.Execute()
}
