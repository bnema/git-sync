package cmd

import (
	"github.com/spf13/cobra"

	"github.com/bnema/git-sync/internal/daemon"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the git sync daemon",
	Long: `Run the git sync daemon process.

This command is typically executed by systemd and should not be run manually.
The daemon will continuously monitor and sync configured repositories.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDaemon()
	},
}

func runDaemon() error {
	d, err := daemon.NewDaemon(configFile)
	if err != nil {
		return err
	}

	return d.Run()
}