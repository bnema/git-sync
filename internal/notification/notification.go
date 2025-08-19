package notification

import (
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type NotificationManager struct {
	enabled bool
	timeout int // milliseconds
	logger  *slog.Logger
}

func NewNotificationManager(enabled bool, timeout int, logger *slog.Logger) *NotificationManager {
	return &NotificationManager{
		enabled: enabled,
		timeout: timeout,
		logger:  logger,
	}
}

func (nm *NotificationManager) SendSyncNotification(repoPath, direction, status string, duration time.Duration, errorMsg string) {
	if !nm.enabled {
		return
	}
	
	// Check if notify-send is available
	if !nm.isNotifySendAvailable() {
		nm.logger.Debug("notify-send not available, skipping notification")
		return
	}
	
	// Prepare notification details
	title := nm.buildTitle(repoPath, status)
	body := nm.buildBody(direction, duration, errorMsg)
	urgency := nm.getUrgency(status)
	icon := nm.getIcon(status)
	
	// Send notification
	err := nm.sendNotification(title, body, urgency, icon)
	if err != nil {
		nm.logger.Debug("Failed to send notification", "error", err)
	}
}

func (nm *NotificationManager) isNotifySendAvailable() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("notify-send")
	return err == nil
}

func (nm *NotificationManager) buildTitle(repoPath, status string) string {
	repoName := getRepoName(repoPath)
	if status == "success" {
		return fmt.Sprintf("✓ Git Sync: %s", repoName)
	}
	return fmt.Sprintf("✗ Git Sync Failed: %s", repoName)
}

func (nm *NotificationManager) buildBody(direction string, duration time.Duration, errorMsg string) string {
	if errorMsg != "" {
		return fmt.Sprintf("Direction: %s\nDuration: %s\nError: %s", 
			direction, formatDuration(duration), truncateError(errorMsg, 100))
	}
	return fmt.Sprintf("Successfully synced\nDirection: %s\nDuration: %s", 
		direction, formatDuration(duration))
}

func (nm *NotificationManager) getUrgency(status string) string {
	if status == "success" {
		return "normal"
	}
	return "critical"
}

func (nm *NotificationManager) getIcon(status string) string {
	if status == "success" {
		return "dialog-information"
	}
	return "dialog-error"
}

func (nm *NotificationManager) sendNotification(title, body, urgency, icon string) error {
	args := []string{
		title,
		body,
		"--urgency", urgency,
		"--icon", icon,
		"--expire-time", fmt.Sprintf("%d", nm.timeout),
		"--app-name", "git-sync",
	}
	
	cmd := exec.Command("notify-send", args...)
	return cmd.Run()
}

// Helper functions
func getRepoName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", d.Seconds()*1000)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func truncateError(err string, maxLen int) string {
	if len(err) <= maxLen {
		return err
	}
	return err[:maxLen-3] + "..."
}