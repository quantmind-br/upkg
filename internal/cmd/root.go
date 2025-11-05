package cmd

import (
	"github.com/diogo/pkgctl/internal/config"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root command
func NewRootCmd(cfg *config.Config, log *zerolog.Logger, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "pkgctl",
		Short:        "Package control utility",
		Long:         `A modern package manager for Linux supporting AppImage, DEB, RPM, Tarball, and Binary packages.`,
		SilenceUsage: true,
	}

	// Add subcommands
	cmd.AddCommand(NewInstallCmd(cfg, log))
	cmd.AddCommand(NewUninstallCmd(cfg, log))
	cmd.AddCommand(NewListCmd(cfg, log))
	cmd.AddCommand(NewInfoCmd(cfg, log))
	cmd.AddCommand(NewDoctorCmd(cfg, log))
	cmd.AddCommand(NewCompletionCmd(cfg, log))
	cmd.AddCommand(NewVersionCmd(version))

	return cmd
}
