package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/bnema/git-sync/internal/config"
)

var (
	showAll      bool
	daemonStatus bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status for repositories",
	Long: `Show the current sync status for configured repositories.

Examples:
  git sync status                    # Show status for current repo
  git sync status --all              # Show all configured repos  
  git sync status --daemon           # Show daemon status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return showStatus()
	},
}

func init() {
	statusCmd.Flags().BoolVar(&showAll, "all", false, 
		"show all configured repositories")
	statusCmd.Flags().BoolVar(&daemonStatus, "daemon", false,
		"show daemon status")
}

func showStatus() error {
	if daemonStatus {
		return showDaemonStatus()
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.Repositories) == 0 {
		fmt.Println("No repositories configured for sync.")
		fmt.Println("Run 'git sync init' in a repository to add it to the sync daemon.")
		return nil
	}

	currentDir, _ := os.Getwd()

	if showAll {
		return showAllRepositories(cfg.Repositories)
	}

	// Show status for current repository only
	for _, repo := range cfg.Repositories {
		if repo.Path == currentDir {
			return showRepositoryStatus(repo)
		}
	}

	fmt.Printf("Current repository (%s) is not configured for sync.\n", currentDir)
	fmt.Println("Run 'git sync init' to add it to the sync daemon.")
	return nil
}

func showAllRepositories(repos []config.RepoConfig) error {
	fmt.Printf("Git Sync Configuration (%d repositories)\n\n", len(repos))

	for i, repo := range repos {
		if i > 0 {
			fmt.Println()
		}
		if err := showRepositoryStatus(repo); err != nil {
			fmt.Printf("Error getting status for %s: %v\n", repo.Path, err)
		}
	}

	return nil
}

func showRepositoryStatus(repo config.RepoConfig) error {
	fmt.Printf("Repository: %s\n", filepath.Base(repo.Path))
	fmt.Printf("  Path: %s\n", repo.Path)
	fmt.Printf("  Status: %s\n", getEnabledStatus(repo.Enabled))
	fmt.Printf("  Direction: %s\n", repo.Direction)
	fmt.Printf("  Interval: %ds (%s)\n", repo.Interval, formatDuration(repo.Interval))
	fmt.Printf("  Remote: %s\n", repo.Remote)
	fmt.Printf("  Branch Strategy: %s\n", repo.BranchStrategy)
	fmt.Printf("  Safety Checks: %s\n", getBoolStatus(repo.SafetyChecks))
	fmt.Printf("  Force Push: %s\n", getBoolStatus(repo.ForcePush))

	// Check Git status if accessible
	if gitStatus, err := getGitStatus(repo.Path); err == nil {
		fmt.Printf("  Git Status: %s\n", gitStatus)
	}

	return nil
}

func showDaemonStatus() error {
	// Check if systemd service exists
	cmd := exec.Command("systemctl", "--user", "is-active", "git-sync-daemon.service")
	if err := cmd.Run(); err != nil {
		fmt.Println("Daemon Status: Not installed or not running")
		fmt.Println("Run 'git sync install-daemon' to install the systemd service.")
		return nil
	}

	fmt.Println("Daemon Status: Running")

	// Get service status
	cmd = exec.Command("systemctl", "--user", "status", "git-sync-daemon.service", "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get daemon status: %w", err)
	}

	fmt.Printf("\nSystemd Service Status:\n%s\n", output)

	// Show recent logs
	cmd = exec.Command("journalctl", "--user", "-u", "git-sync-daemon.service", "--no-pager", "-n", "10")
	logs, err := cmd.Output()
	if err == nil {
		fmt.Printf("Recent Logs:\n%s\n", logs)
	}

	return nil
}

func getGitStatus(repoPath string) (string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	if len(output) == 0 {
		return "Clean", nil
	}

	return "Modified files present", nil
}

func getEnabledStatus(enabled bool) string {
	if enabled {
		return "✓ Enabled"
	}
	return "✗ Disabled"
}

func getBoolStatus(value bool) string {
	if value {
		return "✓ Yes"
	}
	return "✗ No"
}

func formatDuration(seconds int) string {
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", seconds)
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}