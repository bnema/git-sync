package validation

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ValidateGitURL validates if the provided URL is a valid git repository URL
func ValidateGitURL(repoURL string) error {
	if repoURL == "" {
		return errors.New("repository URL cannot be empty")
	}

	// Handle SSH URLs (git@github.com:user/repo.git)
	if strings.HasPrefix(repoURL, "git@") {
		parts := strings.Split(repoURL, ":")
		if len(parts) != 2 {
			return errors.New("invalid SSH URL format (expected: git@host:user/repo.git)")
		}
		
		hostPart := parts[0]
		pathPart := parts[1]
		
		if !strings.HasPrefix(hostPart, "git@") || len(hostPart) <= 4 {
			return errors.New("invalid SSH URL format (missing or invalid host)")
		}
		
		if pathPart == "" {
			return errors.New("invalid SSH URL format (missing repository path)")
		}
		
		return nil
	}

	// Handle HTTP/HTTPS URLs
	if strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://") {
		parsedURL, err := url.Parse(repoURL)
		if err != nil {
			return fmt.Errorf("invalid URL format: %w", err)
		}
		
		if parsedURL.Host == "" {
			return errors.New("invalid URL: missing host")
		}
		
		if parsedURL.Path == "" || parsedURL.Path == "/" {
			return errors.New("invalid URL: missing repository path")
		}
		
		return nil
	}

	// Handle file:// URLs
	if strings.HasPrefix(repoURL, "file://") {
		parsedURL, err := url.Parse(repoURL)
		if err != nil {
			return fmt.Errorf("invalid file URL format: %w", err)
		}
		
		path := parsedURL.Path
		if path == "" {
			return errors.New("invalid file URL: missing path")
		}
		
		// Check if the path exists and is a git repository
		gitDir := filepath.Join(path, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			return fmt.Errorf("path '%s' is not a git repository", path)
		}
		
		return nil
	}

	return errors.New("unsupported URL scheme (supported: https://, http://, git@, file://)")
}

// ValidatePath validates if the provided path is valid and writable
func ValidatePath(path string) error {
	if path == "" {
		return errors.New("path cannot be empty")
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if path exists
	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		// Check if parent directory exists and is writable
		parentDir := filepath.Dir(absPath)
		parentInfo, err := os.Stat(parentDir)
		if err != nil {
			return fmt.Errorf("parent directory '%s' does not exist", parentDir)
		}
		
		if !parentInfo.IsDir() {
			return fmt.Errorf("parent path '%s' is not a directory", parentDir)
		}
		
		// Check if we can write to parent directory
		testFile := filepath.Join(parentDir, ".git-sync-write-test")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			return fmt.Errorf("cannot write to parent directory '%s': %w", parentDir, err)
		}
		if err := os.Remove(testFile); err != nil {
			return fmt.Errorf("failed to clean up test file '%s': %w", testFile, err)
		}
		
		return nil
	}
	
	if err != nil {
		return fmt.Errorf("cannot access path '%s': %w", absPath, err)
	}

	// If path exists, check if it's a directory
	if !info.IsDir() {
		return fmt.Errorf("path '%s' exists but is not a directory", absPath)
	}

	// Check if directory is writable
	testFile := filepath.Join(absPath, ".git-sync-write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("directory '%s' is not writable: %w", absPath, err)
	}
	if err := os.Remove(testFile); err != nil {
		return fmt.Errorf("failed to clean up test file '%s': %w", testFile, err)
	}

	return nil
}

// ValidateBranch validates if the provided branch name follows git branch naming rules
func ValidateBranch(branch string) error {
	if branch == "" {
		return nil // Empty is allowed (will use default)
	}

	// Check basic git branch naming rules
	if strings.HasPrefix(branch, "-") || strings.HasSuffix(branch, ".lock") {
		return errors.New("invalid branch name format")
	}

	// Check for invalid characters
	invalidChars := []string{" ", "~", "^", ":", "?", "*", "[", "\\", "..", "//", "@{"}
	for _, char := range invalidChars {
		if strings.Contains(branch, char) {
			return fmt.Errorf("branch name contains invalid character: '%s'", char)
		}
	}

	// Check if branch name starts or ends with '/'
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") {
		return errors.New("branch name cannot start or end with '/'")
	}

	return nil
}

// ValidateInterval validates if the sync interval is within reasonable bounds
func ValidateInterval(intervalStr string) error {
	if intervalStr == "" {
		return errors.New("interval cannot be empty")
	}

	interval, err := strconv.Atoi(intervalStr)
	if err != nil {
		return errors.New("interval must be a number")
	}

	if interval < 30 {
		return errors.New("interval too low: minimum is 30 seconds to avoid excessive load")
	}

	if interval > 86400 {
		return errors.New("interval too high: maximum is 24 hours (86400 seconds)")
	}

	return nil
}

// ValidateDirection validates if the sync direction is valid
func ValidateDirection(direction string) error {
	if direction == "" {
		return errors.New("direction cannot be empty")
	}

	validDirections := []string{"push", "pull", "both"}
	for _, valid := range validDirections {
		if direction == valid {
			return nil
		}
	}

	return fmt.Errorf("invalid direction '%s': must be push, pull, or both", direction)
}

// ValidateBranchStrategy validates if the branch strategy is valid
func ValidateBranchStrategy(strategy string) error {
	if strategy == "" {
		return errors.New("branch strategy cannot be empty")
	}

	validStrategies := []string{"current", "main", "all", "specific"}
	for _, valid := range validStrategies {
		if strategy == valid {
			return nil
		}
	}

	return fmt.Errorf("invalid branch strategy '%s': must be current, main, all, or specific", strategy)
}

// ValidateRemote validates if the git remote exists in the current repository
func ValidateRemote(remoteName string) error {
	if remoteName == "" {
		return errors.New("remote name cannot be empty")
	}

	cmd := exec.Command("git", "remote", "get-url", remoteName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remote '%s' does not exist", remoteName)
	}

	return nil
}

// ValidateGitRepository validates if the current directory is a git repository
func ValidateGitRepository() error {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return errors.New("not a git repository (or any of the parent directories)")
	}
	return nil
}

// ValidateTargetBranch validates if the target branch exists (for specific branch strategy)
func ValidateTargetBranch(branchName string) error {
	if branchName == "" {
		return errors.New("target branch name cannot be empty for 'specific' strategy")
	}

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