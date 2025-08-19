package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SyncHistoryEntry represents a single sync operation record
type SyncHistoryEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	RepoPath   string    `json:"repo_path"`
	Direction  string    `json:"direction"`
	Status     string    `json:"status"`
	DurationMs int64     `json:"duration_ms"`
	ErrorMsg   string    `json:"error_message,omitempty"`
}

// HistoryManager manages persistent sync history using JSON Lines format
type HistoryManager struct {
	cacheDir      string
	historyFile   string
	lockFile      string
	maxEntries    int
	retentionDays int
	maxFileSizeMB int64
	logger        *slog.Logger
	mu            sync.Mutex
}

// NewHistoryManager creates a new history manager
func NewHistoryManager(cacheDir string, maxEntries, retentionDays, maxFileSizeMB int, logger *slog.Logger) (*HistoryManager, error) {
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".cache", "git-sync")
	}

	hm := &HistoryManager{
		cacheDir:      cacheDir,
		historyFile:   filepath.Join(cacheDir, "history.jsonl"),
		lockFile:      filepath.Join(cacheDir, ".history.lock"),
		maxEntries:    maxEntries,
		retentionDays: retentionDays,
		maxFileSizeMB: int64(maxFileSizeMB) * 1024 * 1024, // Convert MB to bytes
		logger:        logger,
	}

	if err := hm.ensureHistoryDir(); err != nil {
		return nil, err
	}

	return hm, nil
}

// ensureHistoryDir creates the cache directory if it doesn't exist
func (hm *HistoryManager) ensureHistoryDir() error {
	if err := os.MkdirAll(hm.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory %s: %w", hm.cacheDir, err)
	}
	return nil
}

// RecordSync records a sync operation to the history file
func (hm *HistoryManager) RecordSync(repoPath, direction, status string, duration time.Duration, errorMsg string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	entry := SyncHistoryEntry{
		Timestamp:  time.Now(),
		RepoPath:   repoPath,
		Direction:  direction,
		Status:     status,
		DurationMs: duration.Milliseconds(),
		ErrorMsg:   errorMsg,
	}

	if err := hm.appendEntry(entry); err != nil {
		hm.logger.Error("Failed to record sync history", "error", err)
		return
	}

	// Check if file rotation is needed
	if hm.shouldRotateFile() {
		if err := hm.rotateFile(); err != nil {
			hm.logger.Error("Failed to rotate history file", "error", err)
		}
	}
}

// appendEntry appends a single entry to the history file
func (hm *HistoryManager) appendEntry(entry SyncHistoryEntry) error {
	// Acquire file lock
	lockFd, err := hm.acquireLock()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer hm.releaseLock(lockFd)

	// For atomic append, we'll directly append to the main file
	// This is safe because we have the lock
	file, err := os.OpenFile(hm.historyFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	// Write JSON line
	jsonData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	if _, err := file.Write(append(jsonData, '\n')); err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// GetHistory retrieves sync history entries with optional filtering
func (hm *HistoryManager) GetHistory(limit int, repoFilter string, failedOnly bool) ([]SyncHistoryEntry, error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Acquire file lock for reading
	lockFd, err := hm.acquireLock()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer hm.releaseLock(lockFd)

	file, err := os.Open(hm.historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []SyncHistoryEntry{}, nil
		}
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	var entries []SyncHistoryEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry SyncHistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			hm.logger.Warn("Failed to parse history line, skipping", "line", line, "error", err)
			continue
		}

		// Apply filters
		if repoFilter != "" && entry.RepoPath != repoFilter {
			continue
		}
		if failedOnly && entry.Status != "failed" {
			continue
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}

	// Sort by timestamp (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	// Apply limit
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return entries, nil
}

// shouldRotateFile checks if the history file should be rotated
func (hm *HistoryManager) shouldRotateFile() bool {
	info, err := os.Stat(hm.historyFile)
	if err != nil {
		return false
	}
	return info.Size() > hm.maxFileSizeMB
}

// rotateFile rotates the current history file
func (hm *HistoryManager) rotateFile() error {
	oldFile := hm.historyFile + ".old"
	
	// Remove old backup if it exists
	if err := os.Remove(oldFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old backup: %w", err)
	}

	// Move current file to backup
	if err := os.Rename(hm.historyFile, oldFile); err != nil {
		return fmt.Errorf("failed to rotate history file: %w", err)
	}

	hm.logger.Info("Rotated history file", "old_file", oldFile)
	return nil
}

// CleanOldEntries removes entries older than the retention period
func (hm *HistoryManager) CleanOldEntries() error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -hm.retentionDays)
	
	// Get all entries
	entries, err := hm.getAllEntries()
	if err != nil {
		return err
	}

	// Filter out old entries
	var validEntries []SyncHistoryEntry
	removedCount := 0
	
	for _, entry := range entries {
		if entry.Timestamp.After(cutoff) {
			validEntries = append(validEntries, entry)
		} else {
			removedCount++
		}
	}

	if removedCount == 0 {
		return nil // No cleanup needed
	}

	// Rewrite file with valid entries only
	if err := hm.rewriteHistoryFile(validEntries); err != nil {
		return err
	}

	hm.logger.Info("Cleaned old history entries", "removed_count", removedCount, "retention_days", hm.retentionDays)
	return nil
}

// getAllEntries reads all entries from the history file
func (hm *HistoryManager) getAllEntries() ([]SyncHistoryEntry, error) {
	file, err := os.Open(hm.historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []SyncHistoryEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var entries []SyncHistoryEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry SyncHistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			hm.logger.Warn("Failed to parse history line during cleanup, skipping", "line", line, "error", err)
			continue
		}

		entries = append(entries, entry)
	}

	return entries, scanner.Err()
}

// rewriteHistoryFile rewrites the history file with the given entries
func (hm *HistoryManager) rewriteHistoryFile(entries []SyncHistoryEntry) error {
	// Acquire file lock
	lockFd, err := hm.acquireLock()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer hm.releaseLock(lockFd)

	// Create temp file
	tempFile := hm.historyFile + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Write all entries
	for _, entry := range entries {
		jsonData, err := json.Marshal(entry)
		if err != nil {
			file.Close()
			os.Remove(tempFile)
			return fmt.Errorf("failed to marshal entry: %w", err)
		}

		if _, err := file.Write(append(jsonData, '\n')); err != nil {
			file.Close()
			os.Remove(tempFile)
			return fmt.Errorf("failed to write entry: %w", err)
		}
	}

	if err := file.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, hm.historyFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// acquireLock acquires an exclusive file lock
func (hm *HistoryManager) acquireLock() (*os.File, error) {
	lockFile, err := os.OpenFile(hm.lockFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		lockFile.Close()
		return nil, err
	}

	return lockFile, nil
}

// releaseLock releases the file lock
func (hm *HistoryManager) releaseLock(lockFile *os.File) {
	syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	lockFile.Close()
}