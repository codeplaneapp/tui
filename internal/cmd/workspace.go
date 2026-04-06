package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:                "workspace",
	Aliases:            []string{"workspaces"},
	Short:              "Manage JJHub/codeplane workspaces",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
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
