package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/elevate"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/installer"
)

func newInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the IFEO game launch hook",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall()
		},
	}
}

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the IFEO game launch hook",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall()
		},
	}
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print install status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd)
		},
	}
}

func runInstall() error {
	if err := installer.Install(); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	return nil
}

func runUninstall() error {
	err := installer.Uninstall()
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) && !windows.GetCurrentProcessToken().IsElevated() {
		code, elevationErr := elevate.Run("uninstall")
		if elevationErr != nil {
			return fmt.Errorf("uninstall elevation: %w", elevationErr)
		}
		if code != 0 {
			return fmt.Errorf("elevated uninstall failed with exit code %d", code)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	return nil
}

func runStatus(cmd *cobra.Command) error {
	printStatus(cmd.OutOrStdout(), installer.Status())
	return nil
}

func printStatus(w io.Writer, entries []installer.Entry) {
	anyInstalled := false
	for _, e := range entries {
		if e.Installed {
			fmt.Fprintf(w, "[status] %s -> %s\n", e.Target, e.Debugger)
			anyInstalled = true
			continue
		}
		fmt.Fprintf(w, "[status] %s: not installed\n", e.Target)
	}
	if !anyInstalled {
		fmt.Fprintln(w, "[status] Not installed")
	}
}
