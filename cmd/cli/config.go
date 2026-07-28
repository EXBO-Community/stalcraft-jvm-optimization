package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/config"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/profile"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage JVM config profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newConfigListCommand(),
		newConfigActiveCommand(),
		newConfigSelectCommand(),
		newConfigReleasesCommand(),
		newConfigRegenerateCommand(),
	)
	return cmd
}

func newConfigListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available config profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureProfiles(); err != nil {
				return err
			}
			names, err := config.List()
			if err != nil {
				return err
			}
			for _, name := range names {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), name); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newConfigActiveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "active",
		Short: "Print the selected config profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureProfiles(); err != nil {
				return err
			}
			active := config.ActiveName()
			if active == "" {
				active = profile.LatestDefaultID()
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), active)
			return err
		},
	}
}

func newConfigSelectCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "select <config-id>",
		Short:             "Select an active config profile",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: configIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureProfiles(); err != nil {
				return err
			}
			id := args[0]
			if !config.Exists(id) {
				return fmt.Errorf("config not found: %s", id)
			}
			if err := config.SetActive(id); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "active config set to: %s\n", id)
			return err
		},
	}
}

func newConfigReleasesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "releases",
		Short: "List generator releases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, release := range profile.Releases() {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), release.Version); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newConfigRegenerateCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "regenerate <release>",
		Short:             "Regenerate all profiles for a release",
		Args:              cobra.ExactValidArgs(1),
		ValidArgs:         releaseVersions(),
		ValidArgsFunction: releaseCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			release := args[0]
			generated, err := profile.Regenerate(release, sysinfo.Detect())
			if err != nil {
				return err
			}
			selected, _ := profile.Find(release)
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"regenerated %s (%d profile(s)); active config: %s\n",
				release,
				len(generated),
				selected.DefaultID(),
			)
			return err
		},
	}
}

func configIDCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	names, err := config.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, toComplete) {
			out = append(out, name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func releaseCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	versions := releaseVersions()
	out := make([]string, 0, len(versions))
	for _, version := range versions {
		if strings.HasPrefix(version, toComplete) {
			out = append(out, version)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func releaseVersions() []string {
	releases := profile.Releases()
	out := make([]string, 0, len(releases))
	for _, release := range releases {
		out = append(out, release.Version)
	}
	return out
}

func ensureProfiles() error {
	return profile.Ensure(sysinfo.Detect())
}
