package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type Config struct {
	Global       GlobalConfig `toml:"global"`
	Repositories []RepoConfig `toml:"repositories"`
}

type GlobalConfig struct {
	LogLevel           string `toml:"log_level"`
	DefaultInterval    int    `toml:"default_interval"`
	MaxConcurrentSyncs int    `toml:"max_concurrent_syncs"`
	
	// History configuration
	HistoryMaxEntries    int    `toml:"history_max_entries"`
	HistoryRetentionDays int    `toml:"history_retention_days"`
	HistoryCacheDir      string `toml:"history_cache_dir"`
	HistoryMaxFileSizeMB int    `toml:"history_max_file_size_mb"`
}

type RepoConfig struct {
	Path           string `toml:"path"`
	Enabled        bool   `toml:"enabled"`
	Direction      string `toml:"direction"`
	Interval       int    `toml:"interval"`
	Remote         string `toml:"remote"`
	BranchStrategy string `toml:"branch_strategy"`
	TargetBranch   string `toml:"target_branch,omitempty"`
	SafetyChecks   bool   `toml:"safety_checks"`
	ForcePush      bool   `toml:"force_push"`
}

// ConfigWatcher handles live configuration file watching
type ConfigWatcher struct {
	viper         *viper.Viper
	configPath    string
	onChange      func(*Config) error
	logger        *slog.Logger
	currentConfig *Config
	mu            sync.RWMutex
	lastChange    time.Time
	debounceDelay time.Duration
}

func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()
	
	var err error
	configPath, err = GetConfigPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	// Create default config if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := createDefaultConfig(configPath); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
	}

	// Configure Viper
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")
	
	// Set defaults
	v.SetDefault("global.log_level", "info")
	v.SetDefault("global.default_interval", 300)
	v.SetDefault("global.max_concurrent_syncs", 5)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

func SaveConfig(config *Config, configPath string) error {
	v := viper.New()
	
	var err error
	configPath, err = GetConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Configure Viper for writing
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")
	
	// Set the config values
	v.Set("global", config.Global)
	v.Set("repositories", config.Repositories)

	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func AddRepository(repoConfig RepoConfig, configPath string) error {
	config, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	// Check if repository already exists
	for i, repo := range config.Repositories {
		if repo.Path == repoConfig.Path {
			// Update existing repository
			config.Repositories[i] = repoConfig
			return SaveConfig(config, configPath)
		}
	}

	// Add new repository
	config.Repositories = append(config.Repositories, repoConfig)
	return SaveConfig(config, configPath)
}

func getDefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "git-sync", "config.toml"), nil
}

// GetConfigPath returns the config file path, using the provided path if not empty,
// otherwise returning the default config path
func GetConfigPath(configFile string) (string, error) {
	if configFile != "" {
		return configFile, nil
	}
	return getDefaultConfigPath()
}

func createDefaultConfig(configPath string) error {
	defaultConfig := &Config{
		Global: GlobalConfig{
			LogLevel:           "info",
			DefaultInterval:    300,
			MaxConcurrentSyncs: 5,
			
			// History defaults
			HistoryMaxEntries:    1000,
			HistoryRetentionDays: 30,
			HistoryCacheDir:      "", // Will use default ~/.cache/git-sync
			HistoryMaxFileSizeMB: 10,
		},
		Repositories: []RepoConfig{},
	}

	return SaveConfig(defaultConfig, configPath)
}

// NewConfigWatcher creates a new ConfigWatcher instance
func NewConfigWatcher(configPath string, onChange func(*Config) error, logger *slog.Logger) (*ConfigWatcher, error) {
	if configPath == "" {
		var err error
		configPath, err = getDefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err)
		}
	}

	// Load initial config
	initialConfig, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load initial config: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")
	
	// Set defaults
	v.SetDefault("global.log_level", "info")
	v.SetDefault("global.default_interval", 300)
	v.SetDefault("global.max_concurrent_syncs", 5)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cw := &ConfigWatcher{
		viper:         v,
		configPath:    configPath,
		onChange:      onChange,
		logger:        logger,
		currentConfig: initialConfig,
		debounceDelay: 500 * time.Millisecond,
	}

	return cw, nil
}

// StartWatching begins watching the config file for changes
func (cw *ConfigWatcher) StartWatching() error {
	cw.viper.OnConfigChange(func(e fsnotify.Event) {
		cw.mu.Lock()
		defer cw.mu.Unlock()
		
		// Debounce rapid file changes
		now := time.Now()
		if now.Sub(cw.lastChange) < cw.debounceDelay {
			return
		}
		cw.lastChange = now
		
		cw.logger.Info("Config file changed, reloading", "file", e.Name)
		
		// Reload config
		var newConfig Config
		if err := cw.viper.Unmarshal(&newConfig); err != nil {
			cw.logger.Error("Failed to unmarshal updated config", "error", err)
			return
		}
		
		// Validate config
		if err := cw.validateConfig(&newConfig); err != nil {
			cw.logger.Error("Invalid config detected, ignoring changes", "error", err)
			return
		}
		
		// Update current config
		cw.currentConfig = &newConfig
		
		// Call the onChange callback
		if cw.onChange != nil {
			if err := cw.onChange(&newConfig); err != nil {
				cw.logger.Error("Failed to apply config changes", "error", err)
				return
			}
		}
		
		cw.logger.Info("Config reloaded successfully")
	})
	
	cw.viper.WatchConfig()
	cw.logger.Info("Started watching config file", "path", cw.configPath)
	return nil
}

// StopWatching stops watching the config file
func (cw *ConfigWatcher) StopWatching() {
	// Viper doesn't provide a direct way to stop watching, so we clear the callback
	cw.viper.OnConfigChange(func(e fsnotify.Event) {})
	cw.logger.Info("Stopped watching config file")
}

// GetCurrentConfig returns the current configuration (thread-safe)
func (cw *ConfigWatcher) GetCurrentConfig() *Config {
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	return cw.currentConfig
}

// validateConfig performs basic validation on the configuration
func (cw *ConfigWatcher) validateConfig(config *Config) error {
	if config.Global.DefaultInterval <= 0 {
		return fmt.Errorf("default_interval must be positive")
	}
	if config.Global.MaxConcurrentSyncs <= 0 {
		return fmt.Errorf("max_concurrent_syncs must be positive")
	}
	
	for i, repo := range config.Repositories {
		if repo.Path == "" {
			return fmt.Errorf("repository %d: path cannot be empty", i)
		}
		if repo.Interval < 0 {
			return fmt.Errorf("repository %d: interval cannot be negative", i)
		}
		if repo.Direction != "push" && repo.Direction != "pull" && repo.Direction != "sync" {
			return fmt.Errorf("repository %d: direction must be 'push', 'pull', or 'sync'", i)
		}
	}
	
	return nil
}