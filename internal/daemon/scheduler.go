package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bnema/git-sync/internal/config"
	"github.com/bnema/git-sync/internal/notification"
)

type Scheduler struct {
	timers              map[string]*time.Timer
	tickers             map[string]*time.Ticker
	mutex               sync.RWMutex
	logger              *slog.Logger
	wg                  sync.WaitGroup
	historyManager      *HistoryManager
	notificationManager *notification.NotificationManager
	ctx                 context.Context
}

func NewScheduler(logger *slog.Logger, historyManager *HistoryManager, notificationManager *notification.NotificationManager) *Scheduler {
	return &Scheduler{
		timers:              make(map[string]*time.Timer),
		tickers:             make(map[string]*time.Ticker),
		logger:              logger,
		historyManager:      historyManager,
		notificationManager: notificationManager,
	}
}

func (s *Scheduler) Start(ctx context.Context, repos []config.RepoConfig, sm *SyncManager) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.ctx = ctx
	s.logger.Info("Starting scheduler", "repositories", len(repos))

	for _, repo := range repos {
		if !repo.Enabled {
			s.logger.Debug("Skipping disabled repository", "path", repo.Path)
			continue
		}

		s.scheduleRepo(ctx, repo, sm)
	}
}

func (s *Scheduler) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Info("Stopping scheduler")

	// Stop all timers
	for path, timer := range s.timers {
		timer.Stop()
		delete(s.timers, path)
	}

	// Stop all tickers
	for path, ticker := range s.tickers {
		ticker.Stop()
		delete(s.tickers, path)
	}

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	// Create a timeout context for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-done:
		s.logger.Info("All scheduler goroutines stopped gracefully")
	case <-shutdownCtx.Done():
		s.logger.Warn("Scheduler shutdown timed out, some goroutines may still be running")
	}

	s.logger.Info("Scheduler stopped")
}

func (s *Scheduler) scheduleRepo(ctx context.Context, repo config.RepoConfig, sm *SyncManager) {
	s.logger.Info("Scheduling repository", 
		"path", repo.Path, 
		"interval", repo.Interval)

	interval := time.Duration(repo.Interval) * time.Second

	// Create ticker for regular syncing
	ticker := time.NewTicker(interval)
	s.tickers[repo.Path] = ticker

	s.wg.Add(1)
	go func(repoConfig config.RepoConfig) {
		defer s.wg.Done()
		defer func() {
			s.mutex.Lock()
			if ticker, exists := s.tickers[repoConfig.Path]; exists {
				ticker.Stop()
				delete(s.tickers, repoConfig.Path)
			}
			s.mutex.Unlock()
		}()

		// Perform initial sync after a short delay
		initialDelay := time.NewTimer(10 * time.Second)
		select {
		case <-initialDelay.C:
			s.performSync(repoConfig, sm)
		case <-ctx.Done():
			initialDelay.Stop()
			return
		}

		// Regular sync loop
		for {
			select {
			case <-ticker.C:
				s.performSync(repoConfig, sm)
			case <-ctx.Done():
				s.logger.Debug("Context cancelled for repository", "path", repoConfig.Path)
				return
			}
		}
	}(repo)
}

func (s *Scheduler) performSync(repo config.RepoConfig, sm *SyncManager) {
	s.logger.Debug("Performing scheduled sync", "repo", repo.Path)

	// Use the scheduler's context for the sync operation
	start := time.Now()
	err := sm.SyncRepository(s.ctx, repo)
	duration := time.Since(start)

	// Determine status and error message
	status := "success"
	errorMsg := ""
	if err != nil {
		status = "failed"
		errorMsg = err.Error()
	}

	// Record in history if history manager is available
	if s.historyManager != nil {
		s.historyManager.RecordSync(repo.Path, repo.Direction, status, duration, errorMsg)
	}

	// Send notification if notification manager is available
	if s.notificationManager != nil {
		s.notificationManager.SendSyncNotification(repo.Path, repo.Direction, status, duration, errorMsg)
	}

	if err != nil {
		s.logger.Error("Sync failed", 
			"repo", repo.Path, 
			"error", err,
			"duration", duration)
	} else {
		s.logger.Info("Sync completed successfully", 
			"repo", repo.Path,
			"duration", duration)
	}
}

// GetStatus returns the current status of all scheduled repositories
func (s *Scheduler) GetStatus() map[string]SchedulerStatus {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	status := make(map[string]SchedulerStatus)
	
	for path := range s.tickers {
		status[path] = SchedulerStatus{
			Path:      path,
			Active:    true,
			NextSync:  time.Now(), // This would need to be tracked more precisely
		}
	}

	return status
}

type SchedulerStatus struct {
	Path     string
	Active   bool
	NextSync time.Time
}