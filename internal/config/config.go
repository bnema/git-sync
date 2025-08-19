package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Global       GlobalConfig `toml:"global"`
	Repositories []RepoConfig `toml:"repositories"`
}

type GlobalConfig struct {
	LogLevel           string `toml:"log_level"`
	DefaultInterval    int    `toml:"default_interval"`
	MaxConcurrentSyncs int    `toml:"max_concurrent_syncs"`
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

func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		var err error
		configPath, err = getDefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err)
		}
	}

	// Create default config if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := createDefaultConfig(configPath); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
	}

	var config Config
	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	// Set defaults if not specified
	if config.Global.LogLevel == "" {
		config.Global.LogLevel = "info"
	}
	if config.Global.DefaultInterval == 0 {
		config.Global.DefaultInterval = 300
	}
	if config.Global.MaxConcurrentSyncs == 0 {
		config.Global.MaxConcurrentSyncs = 5
	}

	return &config, nil
}

func SaveConfig(config *Config, configPath string) error {
	if configPath == "" {
		var err error
		configPath, err = getDefaultConfigPath()
		if err != nil {
			return fmt.Errorf("failed to get default config path: %w", err)
		}
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer func() { 
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close config file: %v\n", err)
		}
	}()

	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
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

func createDefaultConfig(configPath string) error {
	defaultConfig := &Config{
		Global: GlobalConfig{
			LogLevel:           "info",
			DefaultInterval:    300,
			MaxConcurrentSyncs: 5,
		},
		Repositories: []RepoConfig{},
	}

	return SaveConfig(defaultConfig, configPath)
}