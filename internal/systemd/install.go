package systemd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const serviceTemplate = `[Unit]
Description=Git Sync Daemon
After=network.target

[Service]
Type=notify
ExecStart=%s daemon
Restart=always
RestartSec=10
Environment=HOME=%%h
WorkingDirectory=%%h

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=git-sync-daemon

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=false
ReadWritePaths=%%h

[Install]
WantedBy=default.target`

const timerTemplate = `[Unit]
Description=Git Sync Daemon Timer
Requires=git-sync-daemon.service

[Timer]
OnBootSec=30sec
OnUnitActiveSec=60sec
Persistent=true

[Install]
WantedBy=timers.target`

func InstallUserService(binaryPath string, enableLinger, autoStart bool) error {
	// Get user config directory
	userConfigDir, err := getUserConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get user config directory: %w", err)
	}

	// Create systemd user directory
	systemdDir := filepath.Join(userConfigDir, "systemd", "user")
	if err := os.MkdirAll(systemdDir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd directory: %w", err)
	}

	// Get absolute path to binary
	absPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Create service file
	serviceContent := fmt.Sprintf(serviceTemplate, absPath)
	servicePath := filepath.Join(systemdDir, "git-sync-daemon.service")

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	fmt.Printf("✓ Created systemd service file: %s\n", servicePath)

	// Create timer file
	timerPath := filepath.Join(systemdDir, "git-sync-daemon.timer")
	if err := os.WriteFile(timerPath, []byte(timerTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write timer file: %w", err)
	}

	fmt.Printf("✓ Created systemd timer file: %s\n", timerPath)

	// Reload systemd
	if err := runSystemdCommand("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	fmt.Println("✓ Reloaded systemd user daemon")

	// Enable services
	if err := runSystemdCommand("enable", "git-sync-daemon.service"); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	fmt.Println("✓ Enabled git-sync-daemon.service")

	if err := runSystemdCommand("enable", "git-sync-daemon.timer"); err != nil {
		return fmt.Errorf("failed to enable timer: %w", err)
	}

	fmt.Println("✓ Enabled git-sync-daemon.timer")

	// Start services if requested
	if autoStart {
		if err := runSystemdCommand("start", "git-sync-daemon.service"); err != nil {
			fmt.Printf("⚠️  Warning: Failed to start service: %v\n", err)
		} else {
			fmt.Println("✓ Started git-sync-daemon.service")
		}

		if err := runSystemdCommand("start", "git-sync-daemon.timer"); err != nil {
			fmt.Printf("⚠️  Warning: Failed to start timer: %v\n", err)
		} else {
			fmt.Println("✓ Started git-sync-daemon.timer")
		}
	}

	// Enable lingering if requested
	if enableLinger {
		username := os.Getenv("USER")
		if username != "" {
			cmd := exec.Command("loginctl", "enable-linger", username)
			if err := cmd.Run(); err != nil {
				fmt.Printf("⚠️  Warning: Failed to enable lingering: %v\n", err)
			} else {
				fmt.Printf("✓ Enabled user lingering for %s\n", username)
			}
		}
	}

	fmt.Println("\n🎉 Git sync daemon installed successfully!")
	fmt.Println("The daemon will automatically start with your user session.")
	fmt.Println("\nUseful commands:")
	fmt.Println("  systemctl --user status git-sync-daemon.service")
	fmt.Println("  systemctl --user stop git-sync-daemon.service")
	fmt.Println("  systemctl --user start git-sync-daemon.service")
	fmt.Println("  journalctl --user -u git-sync-daemon -f")

	return nil
}

func UninstallUserService() error {
	// Stop services (ignore errors as services might not be running)
	if err := runSystemdCommand("stop", "git-sync-daemon.service"); err != nil {
		fmt.Printf("Warning: Failed to stop git-sync-daemon.service: %v\n", err)
	}
	if err := runSystemdCommand("stop", "git-sync-daemon.timer"); err != nil {
		fmt.Printf("Warning: Failed to stop git-sync-daemon.timer: %v\n", err)
	}

	// Disable services
	if err := runSystemdCommand("disable", "git-sync-daemon.service"); err != nil {
		fmt.Printf("Warning: Failed to disable git-sync-daemon.service: %v\n", err)
	}
	if err := runSystemdCommand("disable", "git-sync-daemon.timer"); err != nil {
		fmt.Printf("Warning: Failed to disable git-sync-daemon.timer: %v\n", err)
	}

	// Get user config directory
	userConfigDir, err := getUserConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get user config directory: %w", err)
	}

	systemdDir := filepath.Join(userConfigDir, "systemd", "user")

	// Remove service files
	servicePath := filepath.Join(systemdDir, "git-sync-daemon.service")
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	timerPath := filepath.Join(systemdDir, "git-sync-daemon.timer")
	if err := os.Remove(timerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove timer file: %w", err)
	}

	// Reload systemd
	if err := runSystemdCommand("daemon-reload"); err != nil {
		fmt.Printf("Warning: Failed to reload systemd: %v\n", err)
	}

	fmt.Println("✓ Git sync daemon uninstalled successfully")
	return nil
}

func GetServiceStatus() (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", "--quiet", "git-sync-daemon.service")
	err := cmd.Run()
	return err == nil, nil
}

func getUserConfigDir() (string, error) {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return xdgConfig, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".config"), nil
}

func runSystemdCommand(args ...string) error {
	fullArgs := append([]string{"--user"}, args...)
	cmd := exec.Command("systemctl", fullArgs...)
	return cmd.Run()
}