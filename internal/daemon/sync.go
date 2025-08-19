package daemon

import (
	"log/slog"

	"github.com/bnema/git-sync/internal/config"
)

type SyncManager struct {
	maxConcurrent int
	semaphore     chan struct{}
	gitOps        *GitOperations
	logger        *slog.Logger
}

func NewSyncManager(maxConcurrent int, logger *slog.Logger) *SyncManager {
	return &SyncManager{
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
		gitOps:        NewGitOperations(logger),
		logger:        logger,
	}
}

func (sm *SyncManager) SyncRepository(repo config.RepoConfig) error {
	// Acquire semaphore to limit concurrent operations
	sm.semaphore <- struct{}{}
	defer func() { <-sm.semaphore }()

	// Delegate to GitOperations which handles all the complexity
	return sm.gitOps.SyncRepository(repo)
}