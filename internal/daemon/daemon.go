package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"

	"github.com/bnema/git-sync/internal/config"
	"github.com/bnema/git-sync/internal/notification"
)

type Daemon struct {
	config              *config.Config
	configWatcher       *config.ConfigWatcher
	syncManager         *SyncManager
	scheduler           *Scheduler
	historyManager      *HistoryManager
	notificationManager *notification.NotificationManager
	logger              *slog.Logger
	ctx                 context.Context
	cancel              context.CancelFunc
	mu                  sync.RWMutex
}

func NewDaemon(configPath string) (*Daemon, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Setup logger based on config
	logLevel := slog.LevelInfo
	switch cfg.Global.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Create history manager
	historyManager, err := NewHistoryManager(
		cfg.Global.HistoryCacheDir,
		cfg.Global.HistoryMaxEntries,
		cfg.Global.HistoryRetentionDays,
		cfg.Global.HistoryMaxFileSizeMB,
		logger,
	)
	if err != nil {
		logger.Warn("Failed to create history manager, history will be disabled", "error", err)
		historyManager = nil
	}

	// Create notification manager
	notificationManager := notification.NewNotificationManager(
		cfg.Global.EnableNotifications,
		cfg.Global.NotificationTimeout,
		logger,
	)

	// Create daemon instance
	d := &Daemon{
		config:              cfg,
		syncManager:         NewSyncManager(cfg.Global.MaxConcurrentSyncs, logger),
		scheduler:           NewScheduler(logger, historyManager, notificationManager),
		historyManager:      historyManager,
		notificationManager: notificationManager,
		logger:              logger,
		ctx:                 ctx,
		cancel:              cancel,
	}

	// Create config watcher with callback to daemon's reload method
	configWatcher, err := config.NewConfigWatcher(configPath, d.reloadConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create config watcher: %w", err)
	}
	d.configWatcher = configWatcher

	return d, nil
}

func (d *Daemon) Run() error {
	// Signal systemd that we're ready
	if sent, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		d.logger.Warn("Failed to notify systemd of ready state", "error", err)
	} else if !sent {
		d.logger.Debug("Not running under systemd")
	}

	d.logger.Info("Git sync daemon starting",
		"repositories", len(d.config.Repositories),
		"max_concurrent", d.config.Global.MaxConcurrentSyncs)

	// Start sync scheduler for all enabled repositories
	enabledRepos := make([]config.RepoConfig, 0)
	for _, repo := range d.config.Repositories {
		if repo.Enabled {
			enabledRepos = append(enabledRepos, repo)
		}
	}

	if len(enabledRepos) == 0 {
		d.logger.Warn("No enabled repositories configured")
	} else {
		d.logger.Info("Starting scheduler", "enabled_repos", len(enabledRepos))
		d.scheduler.Start(d.ctx, enabledRepos, d.syncManager)
	}

	// Start config file watching
	if err := d.configWatcher.StartWatching(); err != nil {
		d.logger.Error("Failed to start config watcher", "error", err)
		return fmt.Errorf("failed to start config watcher: %w", err)
	}

	// Start history cleanup routine (runs once per day)
	if d.historyManager != nil {
		go d.startHistoryCleanup()
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	d.logger.Info("Git sync daemon started successfully")

	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGHUP:
				d.logger.Info("Received SIGHUP, reloading configuration")
				if err := d.reloadConfigFromSignal(); err != nil {
					d.logger.Error("Failed to reload config", "error", err)
				}
			case syscall.SIGINT, syscall.SIGTERM:
				d.logger.Info("Received shutdown signal", "signal", sig)
				
				// Create a channel for shutdown completion
				shutdownComplete := make(chan error, 1)
				
				// Start shutdown in a goroutine
				go func() {
					shutdownComplete <- d.shutdown()
				}()
				
				// Wait for shutdown with timeout
				select {
				case err := <-shutdownComplete:
					return err
				case <-time.After(10 * time.Second):
					d.logger.Error("Shutdown timeout exceeded, forcing exit")
					return fmt.Errorf("shutdown timeout exceeded")
				}
			}
		case <-d.ctx.Done():
			d.logger.Info("Context cancelled")
			return d.shutdown()
		}
	}
}

// startHistoryCleanup starts a goroutine that periodically cleans old history entries
func (d *Daemon) startHistoryCleanup() {
	ticker := time.NewTicker(24 * time.Hour) // Run once per day
	defer ticker.Stop()

	// Run initial cleanup after 1 hour
	initialDelay := time.NewTimer(1 * time.Hour)
	defer initialDelay.Stop()

	for {
		select {
		case <-initialDelay.C:
			d.logger.Debug("Running initial history cleanup")
			if err := d.historyManager.CleanOldEntries(); err != nil {
				d.logger.Error("Failed to clean old history entries", "error", err)
			}
			initialDelay.Stop() // Disable initial timer
		case <-ticker.C:
			d.logger.Debug("Running scheduled history cleanup")
			if err := d.historyManager.CleanOldEntries(); err != nil {
				d.logger.Error("Failed to clean old history entries", "error", err)
			}
		case <-d.ctx.Done():
			d.logger.Debug("History cleanup routine stopping")
			return
		}
	}
}

func (d *Daemon) reloadConfig(newConfig *config.Config) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Info("Reloading configuration")

	// Stop current scheduler
	d.scheduler.Stop()

	// Update config and restart scheduler
	d.config = newConfig
	d.syncManager = NewSyncManager(newConfig.Global.MaxConcurrentSyncs, d.logger)
	
	// Update notification manager with new config
	d.notificationManager = notification.NewNotificationManager(
		newConfig.Global.EnableNotifications,
		newConfig.Global.NotificationTimeout,
		d.logger,
	)
	
	d.scheduler = NewScheduler(d.logger, d.historyManager, d.notificationManager)

	// Start with new configuration
	enabledRepos := make([]config.RepoConfig, 0)
	for _, repo := range d.config.Repositories {
		if repo.Enabled {
			enabledRepos = append(enabledRepos, repo)
		}
	}

	if len(enabledRepos) > 0 {
		d.scheduler.Start(d.ctx, enabledRepos, d.syncManager)
	}
	
	d.logger.Info("Configuration reloaded successfully", "repositories", len(enabledRepos))

	return nil
}

// reloadConfigFromSignal handles SIGHUP-triggered config reloads
func (d *Daemon) reloadConfigFromSignal() error {
	newConfig, err := config.LoadConfig("")
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}
	return d.reloadConfig(newConfig)
}

func (d *Daemon) shutdown() error {
	d.logger.Info("Shutting down git sync daemon")

	// Signal systemd that we're stopping
	if _, err := daemon.SdNotify(false, daemon.SdNotifyStopping); err != nil {
		d.logger.Warn("Failed to notify systemd of stopping state", "error", err)
	}

	// Stop config watcher
	if d.configWatcher != nil {
		d.configWatcher.StopWatching()
	}

	// Cancel context to stop all operations
	d.cancel()

	// Stop scheduler (with timeout handling built-in)
	d.scheduler.Stop()

	d.logger.Info("Git sync daemon stopped")
	return nil
}