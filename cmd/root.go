package cmd

import (
	"github.com/spf13/cobra"
)

var (
	configFile string
	verbose    bool
)

var rootCmd = &cobra.Command{
	Use:   "git-sync",
	Short: "Centralized Git repository synchronization",
	Long: `Git Sync provides automated synchronization for multiple 
Git repositories through a centralized daemon service.

Examples:
  git sync init                    # Initialize current repo for sync
  git sync status                  # Show sync status
  git sync daemon                  # Run daemon (usually via systemd)
  git sync install-daemon          # Install systemd service`,
	Version: "1.0.0",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", 
		"config file (default: ~/.config/git-sync/config.toml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, 
		"verbose output")
	
	// Add subcommands
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(installDaemonCmd)
	rootCmd.AddCommand(historyCmd)
}

