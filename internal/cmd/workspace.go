package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
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

	workspaceID := args[0]
	start := time.Now()
	attrs := []attribute.KeyValue{
		attribute.String("codeplane.workspace.source", "cli"),
		attribute.String("codeplane.workspace.id", workspaceID),
	}

	client := jjhub.NewClient("")
	workspace, err := client.ViewWorkspace(cmd.Context(), workspaceID)
	if err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			err = fmt.Errorf("jjhub CLI not found on PATH")
		}
		recordWorkspaceAttachResult(time.Since(start), err, attrs...)
		return err
	}
	if workspace != nil && workspace.SSHHost != nil {
		attrs = append(attrs, attribute.String("codeplane.workspace.ssh_host", *workspace.SSHHost))
	}

	proc, err := jjhub.AttachWorkspaceCommand(*workspace)
	if err != nil {
		recordWorkspaceAttachResult(time.Since(start), err, attrs...)
		return err
	}
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	err = proc.Run()
	recordWorkspaceAttachResult(time.Since(start), err, attrs...)
	return err
}

func recordWorkspaceAttachResult(duration time.Duration, err error, attrs ...attribute.KeyValue) {
	result := "ok"
	if err != nil {
		result = "error"
		attrs = append(attrs, attribute.String("codeplane.error", err.Error()))
	}
	observability.RecordWorkspaceLifecycle("attach", result, duration, attrs...)
}
