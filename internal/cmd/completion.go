package cmd

import (
	"os"

	"github.com/diogo/upkg/internal/config"
	"github.com/diogo/upkg/internal/ui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// NewCompletionCmd creates the completion command
func NewCompletionCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for upkg.

To load completions:

Bash:
  $ source <(upkg completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ upkg completion bash > /etc/bash_completion.d/upkg
  # macOS:
  $ upkg completion bash > $(brew --prefix)/etc/bash_completion.d/upkg

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ upkg completion zsh > "${fpath[1]}/_upkg"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ upkg completion fish | source

  # To load completions for each session, execute once:
  $ upkg completion fish > ~/.config/fish/completions/upkg.fish

PowerShell:
  PS> upkg completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> upkg completion powershell > upkg.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := args[0]

			switch shell {
			case "bash":
				if err := cmd.Root().GenBashCompletion(os.Stdout); err != nil {
					ui.PrintError("Failed to generate bash completion: %v", err)
					return err
				}
			case "zsh":
				if err := cmd.Root().GenZshCompletion(os.Stdout); err != nil {
					ui.PrintError("Failed to generate zsh completion: %v", err)
					return err
				}
			case "fish":
				if err := cmd.Root().GenFishCompletion(os.Stdout, true); err != nil {
					ui.PrintError("Failed to generate fish completion: %v", err)
					return err
				}
			case "powershell":
				if err := cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout); err != nil {
					ui.PrintError("Failed to generate powershell completion: %v", err)
					return err
				}
			}

			log.Info().Str("shell", shell).Msg("generated shell completion")
			return nil
		},
	}

	return cmd
}
