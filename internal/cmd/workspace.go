package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:                "workspace",
	Aliases:            []string{"workspaces"},
	Short:              "Manage JJHub/codeplane workspaces",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && args[0] == "attach" {
			return runWorkspaceAttach(cmd, args[1:])
		}
		allArgs := append([]string{"workspace"}, args...)
		proc := exec.CommandContext(cmd.Context(), "jjhub", allArgs...) //nolint:gosec
		proc.Stdin = os.Stdin
		proc.Stdout = os.Stdout
		proc.Stderr = os.Stderr
		if err := proc.Run(); err != nil {
			if _, ok := err.(*exec.Error); ok {
				return fmt.Errorf("jjhub CLI not found on PATH")
			}
			return err
		}
		return nil
	},
}

func runWorkspaceAttach(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: %s attach <workspace-id>", cmd.CommandPath())
	}

	client := jjhub.NewClient("")
	workspace, err := client.ViewWorkspace(cmd.Context(), args[0])
	if err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return fmt.Errorf("jjhub CLI not found on PATH")
		}
		return err
	}

	proc, err := jjhub.AttachWorkspaceCommand(*workspace)
	if err != nil {
		return err
	}
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	return proc.Run()
}
