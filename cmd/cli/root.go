package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/buildinfo"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/ui"
)

type legacyFlags struct {
	install   bool
	uninstall bool
	status    bool
}

func newRootCommand() *cobra.Command {
	cobra.MousetrapHelpText = ""

	legacy := &legacyFlags{}

	root := &cobra.Command{
		Use:           "cli",
		Short:         "STALZONE JVM optimization wrapper",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch legacy.count() {
			case 0:
				return ui.Run()
			case 1:
				return runLegacyCommand(cmd, legacy)
			default:
				return fmt.Errorf("legacy flags --install, --uninstall and --status are mutually exclusive")
			}
		},
	}

	root.Flags().BoolVar(&legacy.install, "install", false, "install wrapper")
	root.Flags().BoolVar(&legacy.uninstall, "uninstall", false, "uninstall wrapper")
	root.Flags().BoolVar(&legacy.status, "status", false, "print install status")
	hideFlag(root, "install")
	hideFlag(root, "uninstall")
	hideFlag(root, "status")

	root.AddCommand(
		newMenuCommand(),
		newInstallCommand(),
		newUninstallCommand(),
		newStatusCommand(),
		newConfigCommand(),
		newVersionCommand(),
	)
	return root
}

func newMenuCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "menu",
		Short: "Open the interactive menu",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ui.Run()
		},
	}
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", buildinfo.Version, buildinfo.Commit)
			return err
		},
	}
}

func runLegacyCommand(cmd *cobra.Command, legacy *legacyFlags) error {
	switch {
	case legacy.install:
		return runInstall()
	case legacy.uninstall:
		return runUninstall()
	case legacy.status:
		return runStatus(cmd)
	default:
		return nil
	}
}

func (f legacyFlags) count() int {
	var n int
	if f.install {
		n++
	}
	if f.uninstall {
		n++
	}
	if f.status {
		n++
	}
	return n
}

func hideFlag(cmd *cobra.Command, name string) {
	_ = cmd.Flags().MarkHidden(name)
}
