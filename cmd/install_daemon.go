package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bnema/git-sync/internal/systemd"
)

var (
	enableLinger bool
	autoStart    bool
	uninstall    bool
)

var installDaemonCmd = &cobra.Command{
	Use:   "install-daemon",
	Short: "Install and configure systemd user service",
	Long: `Install the git-sync daemon as a systemd user service.
This will create the necessary service files and enable the daemon to start automatically.

Examples:
  git sync install-daemon                    # Install with defaults
  git sync install-daemon --no-auto-start   # Install but don't start immediately  
  git sync install-daemon --uninstall       # Remove the service`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if uninstall {
			return uninstallDaemon()
		}
		return installDaemon()
	},
}

func init() {
	installDaemonCmd.Flags().BoolVar(&enableLinger, "enable-linger", true,
		"enable systemd user lingering for boot persistence")
	installDaemonCmd.Flags().BoolVar(&autoStart, "auto-start", true,
		"automatically start the daemon after installation")
	installDaemonCmd.Flags().BoolVar(&uninstall, "uninstall", false,
		"uninstall the systemd service")
}

func installDaemon() error {
	// Check if already installed
	if isInstalled, err := systemd.GetServiceStatus(); err == nil && isInstalled {
		fmt.Println("⚠️  Git sync daemon is already installed and running.")
		fmt.Print("Do you want to overwrite the existing installation? (y/N): ")
		
		var response string
		if _, err := fmt.Scanln(&response); err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			return err
		}
		
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Installation cancelled.")
			return nil
		}
		
		// Uninstall existing service before proceeding
		fmt.Println("Uninstalling existing daemon...")
		if err := systemd.UninstallUserService(); err != nil {
			return fmt.Errorf("failed to uninstall existing service: %w", err)
		}
	}

	// Get the path to the current executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the actual binary path
	binaryPath, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	fmt.Printf("Installing git-sync daemon...\n")
	fmt.Printf("Binary path: %s\n\n", binaryPath)

	// Check if binary exists and is executable
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("binary not found at %s: %w", binaryPath, err)
	}

	// Install the systemd service
	if err := systemd.InstallUserService(binaryPath, enableLinger, autoStart); err != nil {
		return fmt.Errorf("failed to install systemd service: %w", err)
	}

	return nil
}

func uninstallDaemon() error {
	fmt.Println("Uninstalling git-sync daemon...")

	if err := systemd.UninstallUserService(); err != nil {
		return fmt.Errorf("failed to uninstall systemd service: %w", err)
	}

	return nil
}