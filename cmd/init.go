package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bnema/git-sync/internal/config"
)

var (
	direction      string
	interval       int
	remote         string
	branchStrategy string
	targetBranch   string
	safetyChecks   bool
	forcePush      bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize current repository for sync daemon",
	Long: `Add the current Git repository to the centralized sync configuration.
The daemon will automatically sync this repository based on the configured settings.

Examples:
  git sync init                                    # Use defaults (push, 300s interval)
  git sync init -d both -i 600                    # Both directions, 10 min interval
  git sync init --branch-strategy main --force    # Force push to main branch
  git sync init --branch-strategy specific --target-branch develop  # Always sync develop branch`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return initRepository()
	},
}

func init() {
	initCmd.Flags().StringVarP(&direction, "direction", "d", "push", 
		"sync direction: push, pull, both")
	initCmd.Flags().IntVarP(&interval, "interval", "i", 300, 
		"sync interval in seconds")
	initCmd.Flags().StringVarP(&remote, "remote", "r", "origin", 
		"git remote name")
	initCmd.Flags().StringVar(&branchStrategy, "branch-strategy", "current",
		"branch strategy: current, main, all, specific")
	initCmd.Flags().StringVar(&targetBranch, "target-branch", "",
		"target branch name (required when using 'specific' branch strategy)")
	initCmd.Flags().BoolVar(&safetyChecks, "safety-checks", true,
		"enable safety checks before sync operations")
	initCmd.Flags().BoolVar(&forcePush, "force", false,
		"enable force push (use with caution)")
}

func initRepository() error {
	// Get current working directory
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Verify this is a Git repository
	if err := verifyGitRepository(repoPath); err != nil {
		return err
	}

	// Verify remote exists
	if err := verifyRemoteExists(remote); err != nil {
		return err
	}

	// Validate direction parameter
	if !isValidDirection(direction) {
		return fmt.Errorf("invalid direction '%s': must be push, pull, or both", direction)
	}

	// Validate branch strategy
	if !isValidBranchStrategy(branchStrategy) {
		return fmt.Errorf("invalid branch strategy '%s': must be current, main, all, or specific", branchStrategy)
	}

	// Validate specific branch strategy requirements
	if branchStrategy == "specific" {
		if targetBranch == "" {
			// Default to current branch if not specified
			currentBranch, err := getCurrentBranch()
			if err != nil {
				return fmt.Errorf("failed to get current branch for 'specific' strategy: %w", err)
			}
			targetBranch = currentBranch
			fmt.Printf("Using current branch '%s' as target branch\n", targetBranch)
		}
		// Verify the target branch exists
		if err := verifyBranchExists(targetBranch); err != nil {
			return err
		}
	} else if targetBranch != "" {
		return fmt.Errorf("target-branch can only be used with 'specific' branch strategy")
	}

	// Validate configuration combinations
	if err := validateConfigCombination(); err != nil {
		return err
	}

	// Create repository configuration
	repoConfig := config.RepoConfig{
		Path:           repoPath,
		Enabled:        true,
		Direction:      direction,
		Interval:       interval,
		Remote:         remote,
		BranchStrategy: branchStrategy,
		TargetBranch:   targetBranch,
		SafetyChecks:   safetyChecks,
		ForcePush:      forcePush,
	}

	// Add to configuration
	if err := config.AddRepository(repoConfig, configFile); err != nil {
		return fmt.Errorf("failed to add repository to config: %w", err)
	}

	fmt.Printf("✓ Repository initialized for sync\n")
	fmt.Printf("  Path: %s\n", repoPath)
	fmt.Printf("  Direction: %s\n", direction)
	fmt.Printf("  Interval: %ds\n", interval)
	fmt.Printf("  Remote: %s\n", remote)
	fmt.Printf("  Branch Strategy: %s\n", branchStrategy)
	if targetBranch != "" {
		fmt.Printf("  Target Branch: %s\n", targetBranch)
	}
	fmt.Printf("  Safety Checks: %v\n", safetyChecks)
	fmt.Printf("  Force Push: %v\n", forcePush)
	fmt.Printf("\nThe daemon will automatically sync this repository when running.\n")

	return nil
}

func verifyGitRepository(path string) error {
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository (missing .git directory)")
	}
	return nil
}

func verifyRemoteExists(remoteName string) error {
	cmd := exec.Command("git", "remote", "get-url", remoteName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remote '%s' does not exist", remoteName)
	}
	return nil
}

func verifyBranchExists(branchName string) error {
	// Check if branch exists locally
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	if err := cmd.Run(); err != nil {
		// If not local, check if it exists on remote
		cmd = exec.Command("git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branchName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("branch '%s' does not exist locally or on remote", branchName)
		}
	}
	return nil
}

func getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", fmt.Errorf("not on any branch (detached HEAD?)")
	}

	return branch, nil
}

func isValidDirection(dir string) bool {
	validDirections := []string{"push", "pull", "both"}
	for _, valid := range validDirections {
		if dir == valid {
			return true
		}
	}
	return false
}

func isValidBranchStrategy(strategy string) bool {
	validStrategies := []string{"current", "main", "all", "specific"}
	for _, valid := range validStrategies {
		if strategy == valid {
			return true
		}
	}
	return false
}

func validateConfigCombination() error {
	// Validate interval is reasonable
	if interval < 30 {
		return fmt.Errorf("interval too low (%ds): minimum is 30 seconds to avoid excessive load", interval)
	}
	if interval > 86400 {
		return fmt.Errorf("interval too high (%ds): maximum is 24 hours (86400 seconds)", interval)
	}

	// Warn about dangerous combinations
	if forcePush && !safetyChecks {
		fmt.Printf("⚠️  WARNING: Force push enabled without safety checks - this can overwrite remote changes\n")
	}

	if direction == "both" && forcePush {
		fmt.Printf("⚠️  WARNING: Bidirectional sync with force push may cause data loss\n")
	}

	return nil
}