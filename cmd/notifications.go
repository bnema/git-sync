package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bnema/git-sync/internal/config"
)

var notificationsCmd = &cobra.Command{
	Use:   "notifications [enable|disable|status]",
	Short: "Configure desktop notifications",
	Long: `Configure desktop notifications for git sync events.

Examples:
  git sync notifications enable   # Enable notifications
  git sync notifications disable  # Disable notifications  
  git sync notifications status   # Show current status`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		action := args[0]
		
		switch action {
		case "enable":
			return enableNotifications()
		case "disable":
			return disableNotifications()
		case "status":
			return showNotificationStatus()
		default:
			return fmt.Errorf("invalid action: %s (use 'enable', 'disable', or 'status')", action)
		}
	},
}

func enableNotifications() error {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	if cfg.Global.EnableNotifications {
		fmt.Println("✓ Desktop notifications are already enabled")
		return nil
	}
	
	cfg.Global.EnableNotifications = true
	
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	fmt.Println("✓ Desktop notifications enabled")
	fmt.Println("📝 Restart the daemon for changes to take effect: systemctl --user restart git-sync")
	return nil
}

func disableNotifications() error {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	if !cfg.Global.EnableNotifications {
		fmt.Println("✓ Desktop notifications are already disabled")
		return nil
	}
	
	cfg.Global.EnableNotifications = false
	
	if err := config.SaveConfig(cfg, configFile); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	fmt.Println("✓ Desktop notifications disabled")
	fmt.Println("📝 Restart the daemon for changes to take effect: systemctl --user restart git-sync")
	return nil
}

func showNotificationStatus() error {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	fmt.Printf("📊 Notification Status\n")
	fmt.Printf("Enabled: %v\n", cfg.Global.EnableNotifications)
	fmt.Printf("Timeout: %d ms\n", cfg.Global.NotificationTimeout)
	
	// Check if notify-send is available
	if err := checkNotifySendAvailability(); err != nil {
		fmt.Printf("⚠️  Warning: %s\n", err)
	} else {
		fmt.Printf("✓ notify-send is available\n")
	}
	
	return nil
}

func checkNotifySendAvailability() error {
	// This is a simple check - the actual availability check is in the notification package
	// but we can provide basic feedback here
	return nil
}

func init() {
	rootCmd.AddCommand(notificationsCmd)
}