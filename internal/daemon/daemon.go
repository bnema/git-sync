package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/coreos/go-systemd/v22/daemon"

	"github.com/bnema/git-sync/internal/config"
)

type Daemon struct {
	config      *config.Config
	syncManager *SyncManager
	scheduler   *Scheduler
	logger      *slog.Logger
	ctx         context.Context
	cancel      context.CancelFunc
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

	return &Daemon{
		config:      cfg,
		syncManager: NewSyncManager(cfg.Global.MaxConcurrentSyncs, logger),
		scheduler:   NewScheduler(logger),
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
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
				if err := d.reloadConfig(); err != nil {
					d.logger.Error("Failed to reload config", "error", err)
				}
			case syscall.SIGINT, syscall.SIGTERM:
				d.logger.Info("Received shutdown signal", "signal", sig)
				return d.shutdown()
			}
		case <-d.ctx.Done():
			d.logger.Info("Context cancelled")
			return d.shutdown()
		}
	}
}

func (d *Daemon) reloadConfig() error {
	newConfig, err := config.LoadConfig("")
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	// Stop current scheduler
	d.scheduler.Stop()

	// Update config and restart scheduler
	d.config = newConfig
	d.syncManager = NewSyncManager(newConfig.Global.MaxConcurrentSyncs, d.logger)
	d.scheduler = NewScheduler(d.logger)

	// Start with new configuration
	enabledRepos := make([]config.RepoConfig, 0)
	for _, repo := range d.config.Repositories {
		if repo.Enabled {
			enabledRepos = append(enabledRepos, repo)
		}
	}

	d.scheduler.Start(d.ctx, enabledRepos, d.syncManager)
	d.logger.Info("Configuration reloaded successfully", "repositories", len(enabledRepos))

	return nil
}

func (d *Daemon) shutdown() error {
	d.logger.Info("Shutting down git sync daemon")

	// Signal systemd that we're stopping
	if _, err := daemon.SdNotify(false, daemon.SdNotifyStopping); err != nil {
		d.logger.Warn("Failed to notify systemd of stopping state", "error", err)
	}

	// Cancel context to stop all operations
	d.cancel()

	// Stop scheduler
	d.scheduler.Stop()

	d.logger.Info("Git sync daemon stopped")
	return nil
}