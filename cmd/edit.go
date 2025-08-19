package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create config file if it doesn't exist (create a basic empty config)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		basicConfig := `[global]
log_level = "info"
default_interval = 300
max_concurrent_syncs = 5
history_max_entries = 1000
history_retention_days = 30
history_cache_dir = ""
history_max_file_size_mb = 10

[[repositories]]
# Add your repositories here
# path = "/path/to/your/repo"
# remote_name = "origin"
# interval = 300
# enabled = true
`
		if err := os.WriteFile(configPath, []byte(basicConfig), 0644); err != nil {
			return fmt.Errorf("failed to create default config: %w", err)
		}
	}

	// Execute editor command
	editorCmd := exec.Command(editor, configPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	return editorCmd.Run()
}
