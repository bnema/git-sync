package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bnema/git-sync/internal/config"
	"github.com/bnema/git-sync/internal/prompt"
	"github.com/bnema/git-sync/internal/validation"
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

Interactive Mode (default):
  git sync init                     # Interactive prompt for all settings

Non-Interactive Mode:
  git sync init --non-interactive   # Use flags or defaults, no prompts
  git sync init -d both -i 600      # Both directions, 10 min interval
  git sync init --branch-strategy main --force  # Force push to main branch`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInitCommand(cmd, args)
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
	initCmd.Flags().Bool("non-interactive", false,
		"run in non-interactive mode using flags or defaults")
}

func runInitCommand(cmd *cobra.Command, _ []string) error {
	// Check if we're in a git repository first
	if err := validation.ValidateGitRepository(); err != nil {
		return err
	}

	// Check if non-interactive flag is set or if any config flags are provided
	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	hasConfigFlags := cmd.Flags().Changed("direction") || 
		cmd.Flags().Changed("interval") || 
		cmd.Flags().Changed("remote") || 
		cmd.Flags().Changed("branch-strategy") || 
		cmd.Flags().Changed("target-branch") || 
		cmd.Flags().Changed("safety-checks") || 
		cmd.Flags().Changed("force")

	if nonInteractive || hasConfigFlags {
		return initRepository()
	}

	// Run interactive mode
	return runInteractiveInit()
}

func runInteractiveInit() error {
	p := prompt.New()
	
	fmt.Println("🔄 Git Sync Interactive Setup")
	fmt.Println("Configure your git sync settings:")
	fmt.Println()

	// Get current directory for display
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	
	fmt.Printf("📂 Repository: %s\n", repoPath)
	fmt.Println()

	// 1. Sync Direction
	fmt.Println("1️⃣ Sync Direction")
	directionOptions := []string{
		"push - Only push local changes to remote",
		"pull - Only pull remote changes locally", 
		"both - Bidirectional sync (push and pull)",
	}
	directionIndex := p.SelectWithDefault("Choose sync direction:", directionOptions, 0)
	directionValues := []string{"push", "pull", "both"}
	direction = directionValues[directionIndex]
	fmt.Println()

	// 2. Sync Interval
	fmt.Println("2️⃣ Sync Interval")
	intervalOptions := []string{
		"30 seconds (fast)",
		"5 minutes (recommended)",
		"15 minutes",
		"30 minutes", 
		"1 hour",
		"Custom interval",
	}
	intervalIndex := p.SelectWithDefault("Choose sync interval:", intervalOptions, 1)
	intervalValues := []int{30, 300, 900, 1800, 3600, 0}
	
	if intervalIndex == 5 { // Custom interval
		customInterval := p.Input("Enter custom interval in seconds:", validation.ValidateInterval)
		interval, _ = strconv.Atoi(customInterval)
	} else {
		interval = intervalValues[intervalIndex]
	}
	fmt.Println()

	// 3. Remote
	fmt.Println("3️⃣ Git Remote")
	// Get available remotes
	cmd := exec.Command("git", "remote")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get git remotes: %w", err)
	}
	
	remotes := strings.Fields(strings.TrimSpace(string(output)))
	if len(remotes) == 0 {
		return fmt.Errorf("no git remotes found. Please add a remote first with: git remote add origin <url>")
	}
	
	if len(remotes) == 1 {
		remote = remotes[0]
		fmt.Printf("Using remote: %s\n", remote)
	} else {
		fmt.Println("Available remotes:")
		remoteIndex := p.SelectWithDefault("Choose git remote:", remotes, 0)
		remote = remotes[remoteIndex]
	}
	fmt.Println()

	// 4. Branch Strategy
	fmt.Println("4️⃣ Branch Strategy")
	strategyOptions := []string{
		"current - Sync only the current branch",
		"main - Always sync main/master branch",
		"all - Sync all branches",
		"specific - Sync a specific branch",
	}
	strategyIndex := p.SelectWithDefault("Choose branch strategy:", strategyOptions, 0)
	strategyValues := []string{"current", "main", "all", "specific"}
	branchStrategy = strategyValues[strategyIndex]
	
	// If specific strategy, ask for target branch
	if branchStrategy == "specific" {
		targetBranch = p.Input("Enter target branch name:", validation.ValidateBranch)
		if err := validation.ValidateTargetBranch(targetBranch); err != nil {
			return err
		}
	}
	fmt.Println()

	// 5. Safety Options
	fmt.Println("5️⃣ Safety Options")
	safetyChecks = p.Confirm("Enable safety checks before sync operations?", true)
	
	if direction == "push" || direction == "both" {
		forcePush = p.Confirm("Enable force push? (⚠️  Use with caution)", false)
		if forcePush && !safetyChecks {
			fmt.Println("⚠️  WARNING: Force push enabled without safety checks!")
		}
	}
	fmt.Println()

	// Show configuration summary
	summaryItems := map[string]string{
		"Repository Path":   repoPath,
		"Sync Direction":    direction,
		"Sync Interval":     fmt.Sprintf("%d seconds", interval),
		"Remote":           remote,
		"Branch Strategy":  branchStrategy,
		"Safety Checks":    fmt.Sprintf("%v", safetyChecks),
		"Force Push":       fmt.Sprintf("%v", forcePush),
	}
	
	if targetBranch != "" {
		summaryItems["Target Branch"] = targetBranch
	}
	
	p.ShowSummary("Configuration Summary", summaryItems)
	
	if !p.Confirm("Proceed with this configuration?", true) {
		fmt.Println("Setup cancelled.")
		return nil
	}

	// Run the actual initialization
	return initRepository()
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
	return slices.Contains(validDirections, dir)
}

func isValidBranchStrategy(strategy string) bool {
	validStrategies := []string{"current", "main", "all", "specific"}
	return slices.Contains(validStrategies, strategy)
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