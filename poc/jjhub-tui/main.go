// poc/jjhub-tui: gh-dash-inspired TUI for JJHub / Codeplane.
//
// Usage:
//
//	go run ./poc/jjhub-tui/                         # auto-detect repo from cwd
//	go run ./poc/jjhub-tui/ -R roninjin10/jjhub     # explicit owner/repo
//
// Shells out to the `jjhub` CLI for data. Pass -R owner/repo if you're not
// in a directory with a jjhub remote.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/poc/jjhub-tui/tui"
)

func main() {
	repo := ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Println("Usage: jjhub-tui [-R owner/repo]")
			fmt.Println()
			fmt.Println("A gh-dash-inspired terminal dashboard for Codeplane (JJHub).")
			fmt.Println("Shows landings, issues, workspaces, workflows, repos, and notifications.")
			fmt.Println()
			fmt.Println("Options:")
			fmt.Println("  -R owner/repo    Repository to use (default: auto-detect from cwd)")
			os.Exit(0)
		case "-R", "--repo":
			if i+1 < len(args) {
				i++
				repo = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "error: -R requires an argument")
				os.Exit(1)
			}
		default:
			// Also accept bare positional arg for convenience.
			repo = args[i]
		}
	}

	m := tui.NewModel(repo)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
