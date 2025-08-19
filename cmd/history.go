package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/bnema/git-sync/internal/config"
	"github.com/bnema/git-sync/internal/daemon"
)

var (
	historyLimit    int
	historyRepo     string
	historyFailed   bool
	historyWatch    bool
	historyFormat   string
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show sync history for repositories",
	Long: `Show the sync history for configured repositories.

This command displays a log of all sync operations performed by the daemon,
including timestamps, repository paths, sync direction, success/failure status,
duration, and error messages.

Examples:
  git sync history                      # Show last 20 sync operations
  git sync history --limit 50           # Show last 50 operations  
  git sync history --repo /home/proj     # Show history for specific repo
  git sync history --failed             # Show only failed syncs
  git sync history --watch              # Live monitoring mode
  git sync history --format json        # JSON output for scripting`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return showHistory()
	},
}

func init() {
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "l", 20, "Number of entries to show")
	historyCmd.Flags().StringVarP(&historyRepo, "repo", "r", "", "Filter by specific repository path")
	historyCmd.Flags().BoolVarP(&historyFailed, "failed", "f", false, "Show only failed syncs")
	historyCmd.Flags().BoolVarP(&historyWatch, "watch", "w", false, "Live monitoring mode")
	historyCmd.Flags().StringVar(&historyFormat, "format", "table", "Output format (table|json)")
}

func showHistory() error {
	// Load config to get history settings
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create history manager
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn, // Only show warnings/errors for CLI
	}))

	historyManager, err := daemon.NewHistoryManager(
		cfg.Global.HistoryCacheDir,
		cfg.Global.HistoryMaxEntries,
		cfg.Global.HistoryRetentionDays,
		cfg.Global.HistoryMaxFileSizeMB,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create history manager: %w", err)
	}

	if historyWatch {
		return watchHistory(historyManager)
	}

	return displayHistory(historyManager)
}

func displayHistory(hm *daemon.HistoryManager) error {
	entries, err := hm.GetHistory(historyLimit, historyRepo, historyFailed)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No sync history found.")
		return nil
	}

	switch historyFormat {
	case "json":
		return displayHistoryJSON(entries)
	case "table":
		return displayHistoryTable(entries)
	default:
		return fmt.Errorf("invalid format: %s (supported: table, json)", historyFormat)
	}
}

func displayHistoryTable(entries []daemon.SyncHistoryEntry) error {
	// Print header
	fmt.Printf("%-19s %-30s %-9s %-7s %-8s %s\n", 
		"TIMESTAMP", "REPOSITORY", "DIRECTION", "STATUS", "DURATION", "ERROR")
	fmt.Println(strings.Repeat("-", 100))

	// Print entries
	for _, entry := range entries {
		timestamp := entry.Timestamp.Format("2006-01-02 15:04:05")
		repoName := filepath.Base(entry.RepoPath)
		if len(repoName) > 30 {
			repoName = "..." + repoName[len(repoName)-27:]
		}
		duration := formatHistoryDuration(time.Duration(entry.DurationMs) * time.Millisecond)
		errorMsg := entry.ErrorMsg
		if len(errorMsg) > 40 {
			errorMsg = errorMsg[:37] + "..."
		}
		
		// Color coding for status
		status := entry.Status
		if isTerminal() {
			switch entry.Status {
			case "success":
				status = fmt.Sprintf("\033[32m%s\033[0m", entry.Status) // Green
			case "failed":
				status = fmt.Sprintf("\033[31m%s\033[0m", entry.Status)  // Red
			}
		}

		fmt.Printf("%-19s %-30s %-9s %-7s %-8s %s\n", 
			timestamp, repoName, entry.Direction, status, duration, errorMsg)
	}

	return nil
}

func displayHistoryJSON(entries []daemon.SyncHistoryEntry) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}

func watchHistory(hm *daemon.HistoryManager) error {
	fmt.Println("Watching sync history (Press Ctrl+C to exit)...")
	fmt.Println()

	// Display initial history
	if err := displayHistory(hm); err != nil {
		return err
	}

	// Set up file watcher or polling
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastTimestamp time.Time
	entries, err := hm.GetHistory(1, "", false)
	if err == nil && len(entries) > 0 {
		lastTimestamp = entries[0].Timestamp
	}

	for range ticker.C {
		// Check for new entries
		entries, err := hm.GetHistory(historyLimit, historyRepo, historyFailed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading history: %v\n", err)
			continue
		}

		// Find new entries since last check
		var newEntries []daemon.SyncHistoryEntry
		for _, entry := range entries {
			if entry.Timestamp.After(lastTimestamp) {
				newEntries = append(newEntries, entry)
			}
		}

		if len(newEntries) > 0 {
			// Clear screen and redisplay
			fmt.Print("\033[2J\033[H") // Clear screen and move cursor to top
			fmt.Println("Watching sync history (Press Ctrl+C to exit)...")
			fmt.Println()

			if err := displayHistoryTable(entries); err != nil {
				fmt.Fprintf(os.Stderr, "Error displaying history: %v\n", err)
			}

			// Update last timestamp
			lastTimestamp = newEntries[0].Timestamp
		}
	}
	
	return nil // This will never be reached, but satisfies the compiler
}

func isTerminal() bool {
	// Simple check if stdout is a terminal
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// formatHistoryDuration formats a duration for display
func formatHistoryDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}