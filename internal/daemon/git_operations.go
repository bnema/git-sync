package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"

	configPkg "github.com/bnema/git-sync/internal/config"
)

type GitOperations struct {
	logger *slog.Logger
}

func NewGitOperations(logger *slog.Logger) *GitOperations {
	return &GitOperations{
		logger: logger,
	}
}

// SyncRepository performs the sync operation using go-git library
func (g *GitOperations) SyncRepository(ctx context.Context, repo configPkg.RepoConfig) error {
	g.logger.Info("Starting sync with go-git", 
		"repo", filepath.Base(repo.Path), 
		"path", repo.Path,
		"direction", repo.Direction)

	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Open repository
	r, err := git.PlainOpen(repo.Path)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Get worktree
	worktree, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Safety checks
	if repo.SafetyChecks {
		if err := g.performSafetyChecks(ctx, r, worktree, repo); err != nil {
			return err
		}
	}

	// Execute sync based on direction
	switch repo.Direction {
	case "push":
		return g.gitPush(ctx, r, repo)
	case "pull":
		return g.gitPull(ctx, r, worktree, repo)
	case "both":
		if err := g.gitPull(ctx, r, worktree, repo); err != nil {
			return fmt.Errorf("pull failed: %w", err)
		}
		return g.gitPush(ctx, r, repo)
	default:
		return fmt.Errorf("invalid direction: %s", repo.Direction)
	}
}

func (g *GitOperations) performSafetyChecks(ctx context.Context, r *git.Repository, w *git.Worktree, repo configPkg.RepoConfig) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check if there are uncommitted changes
	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("failed to get worktree status: %w", err)
	}

	if !status.IsClean() && !repo.ForcePush {
		return fmt.Errorf("repository has uncommitted changes, skipping sync")
	}

	return nil
}

func (g *GitOperations) gitPush(ctx context.Context, r *git.Repository, repo configPkg.RepoConfig) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Handle specific branch strategy
	if repo.BranchStrategy == "specific" {
		return g.gitPushSpecificBranch(ctx, r, repo)
	}

	pushOptions := &git.PushOptions{
		RemoteName: repo.Remote,
		Progress:   nil, // Could add progress reporting later
	}

	if repo.ForcePush {
		pushOptions.Force = true
		g.logger.Warn("Force push enabled", "repo", repo.Path)
	}

	// Set ref specs based on strategy
	refSpecs, err := g.getRefSpecs(r, repo.BranchStrategy, repo.Remote, false)
	if err != nil {
		return err
	}
	pushOptions.RefSpecs = refSpecs

	err = r.Push(pushOptions)
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			g.logger.Debug("Push: already up to date", "repo", filepath.Base(repo.Path))
			return nil
		}
		return fmt.Errorf("git push failed: %w", err)
	}

	g.logger.Info("Push successful", 
		"repo", filepath.Base(repo.Path),
		"strategy", repo.BranchStrategy)

	return nil
}

func (g *GitOperations) gitPull(ctx context.Context, r *git.Repository, w *git.Worktree, repo configPkg.RepoConfig) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Handle specific branch strategy
	if repo.BranchStrategy == "specific" {
		return g.gitPullSpecificBranch(ctx, r, w, repo)
	}

	pullOptions := &git.PullOptions{
		RemoteName: repo.Remote,
		Progress:   nil,
	}

	// For "all" strategy, we do a fetch instead
	if repo.BranchStrategy == "all" {
		return g.gitFetch(ctx, r, repo)
	}

	err := w.Pull(pullOptions)
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			g.logger.Debug("Pull: already up to date", "repo", filepath.Base(repo.Path))
			return nil
		}
		if err == transport.ErrEmptyRemoteRepository {
			g.logger.Info("Remote repository is empty", "repo", filepath.Base(repo.Path))
			return nil
		}
		return fmt.Errorf("git pull failed: %w", err)
	}

	g.logger.Info("Pull successful", 
		"repo", filepath.Base(repo.Path),
		"strategy", repo.BranchStrategy)

	return nil
}

func (g *GitOperations) gitFetch(ctx context.Context, r *git.Repository, repo configPkg.RepoConfig) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fetchOptions := &git.FetchOptions{
		RemoteName: repo.Remote,
		Progress:   nil,
	}

	err := r.Fetch(fetchOptions)
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			g.logger.Debug("Fetch: already up to date", "repo", filepath.Base(repo.Path))
			return nil
		}
		return fmt.Errorf("git fetch failed: %w", err)
	}

	g.logger.Info("Fetch successful", "repo", filepath.Base(repo.Path))
	return nil
}

func (g *GitOperations) gitPushSpecificBranch(ctx context.Context, r *git.Repository, repo configPkg.RepoConfig) error {
	return g.withBranchSwitch(ctx, r, repo, func() error {
		// Check context before push operation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pushOptions := &git.PushOptions{
			RemoteName: repo.Remote,
			Progress:   nil,
		}

		if repo.ForcePush {
			pushOptions.Force = true
			g.logger.Warn("Force push enabled", "repo", repo.Path)
		}

		// Push only the target branch
		refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", 
			repo.TargetBranch, repo.TargetBranch))
		pushOptions.RefSpecs = []config.RefSpec{refSpec}

		err := r.Push(pushOptions)
		if err != nil {
			if err == git.NoErrAlreadyUpToDate {
				g.logger.Debug("Push: already up to date", "repo", filepath.Base(repo.Path))
				return nil
			}
			return fmt.Errorf("git push failed: %w", err)
		}

		g.logger.Info("Push successful", 
			"repo", filepath.Base(repo.Path),
			"target_branch", repo.TargetBranch)

		return nil
	})
}

func (g *GitOperations) gitPullSpecificBranch(ctx context.Context, r *git.Repository, w *git.Worktree, repo configPkg.RepoConfig) error {
	return g.withBranchSwitch(ctx, r, repo, func() error {
		// Check context before pull operation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pullOptions := &git.PullOptions{
			RemoteName: repo.Remote,
			Progress:   nil,
		}

		err := w.Pull(pullOptions)
		if err != nil {
			if err == git.NoErrAlreadyUpToDate {
				g.logger.Debug("Pull: already up to date", "repo", filepath.Base(repo.Path))
				return nil
			}
			return fmt.Errorf("git pull failed: %w", err)
		}

		g.logger.Info("Pull successful", 
			"repo", filepath.Base(repo.Path),
			"target_branch", repo.TargetBranch)

		return nil
	})
}

// withBranchSwitch executes a function after switching to the target branch
func (g *GitOperations) withBranchSwitch(ctx context.Context, r *git.Repository, repo configPkg.RepoConfig, operation func() error) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Get current branch
	head, err := r.Head()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	currentBranch := head.Name().Short()

	// If we're already on the target branch, just execute the operation
	if currentBranch == repo.TargetBranch {
		return operation()
	}

	g.logger.Debug("Switching to target branch", 
		"from", currentBranch, 
		"to", repo.TargetBranch,
		"repo", filepath.Base(repo.Path))

	// Check for uncommitted changes
	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("failed to get worktree status: %w", err)
	}

	if !status.IsClean() {
		return fmt.Errorf("cannot switch branches due to uncommitted changes")
	}

	// Check context before checkout
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Try to checkout the target branch
	checkoutOptions := &git.CheckoutOptions{
		Branch: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", repo.TargetBranch)),
	}

	err = w.Checkout(checkoutOptions)
	if err != nil {
		// If branch doesn't exist locally, try to create it from remote
		remoteBranch := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", repo.Remote, repo.TargetBranch))
		checkoutOptions.Branch = plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", repo.TargetBranch))
		checkoutOptions.Create = true
		checkoutOptions.Hash = plumbing.ZeroHash // Will be resolved from remote

		// Get remote branch reference
		remoteRef, err := r.Reference(remoteBranch, true)
		if err != nil {
			return fmt.Errorf("failed to find remote branch '%s': %w", repo.TargetBranch, err)
		}
		checkoutOptions.Hash = remoteRef.Hash()

		err = w.Checkout(checkoutOptions)
		if err != nil {
			return fmt.Errorf("failed to create and checkout branch '%s': %w", repo.TargetBranch, err)
		}
	}

	// Defer switching back to original branch
	defer func() {
		originalCheckout := &git.CheckoutOptions{
			Branch: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", currentBranch)),
		}
		
		if switchErr := w.Checkout(originalCheckout); switchErr != nil {
			g.logger.Error("Failed to switch back to original branch", 
				"original", currentBranch, 
				"error", switchErr,
				"repo", filepath.Base(repo.Path))
		} else {
			g.logger.Debug("Switched back to original branch", 
				"branch", currentBranch,
				"repo", filepath.Base(repo.Path))
		}
	}()

	// Execute the operation on the target branch
	return operation()
}

func (g *GitOperations) getRefSpecs(r *git.Repository, strategy, remoteName string, isPull bool) ([]config.RefSpec, error) {
	switch strategy {
	case "current":
		head, err := r.Head()
		if err != nil {
			return nil, fmt.Errorf("failed to get current branch: %w", err)
		}
		branch := head.Name().Short()
		
		if isPull {
			return []config.RefSpec{
				config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/remotes/%s/%s", branch, remoteName, branch)),
			}, nil
		}
		return []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)),
		}, nil
		
	case "main":
		if isPull {
			return []config.RefSpec{
				config.RefSpec(fmt.Sprintf("refs/heads/main:refs/remotes/%s/main", remoteName)),
			}, nil
		}
		return []config.RefSpec{
			config.RefSpec("refs/heads/main:refs/heads/main"),
		}, nil
		
	case "all":
		return []config.RefSpec{
			config.RefSpec("refs/heads/*:refs/heads/*"),
		}, nil
		
	default:
		return nil, fmt.Errorf("invalid branch strategy: %s", strategy)
	}
}