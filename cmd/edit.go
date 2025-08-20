package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/bnema/git-sync/internal/config"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config file in your default editor",
	Long:  `Opens the git-sync configuration file using the editor specified in the EDITOR environment variable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return editConfig()
	},
}

func init() {
	// This command will be added to rootCmd in root.go
}

func editConfig() error {
	// Get config path using helper function
	configPath, err := config.GetConfigPath(configFile)
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	// Get editor from environment variable
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return fmt.Errorf("EDITOR environment variable not set")
	}

	// Ensure config exists with all defaults - this will create it if missing
	// or validate/migrate it if it exists
	if _, err := config.LoadConfig(configPath); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Execute editor command
	editorCmd := exec.Command(editor, configPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	return editorCmd.Run()
}
