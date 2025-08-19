# Git Sync

**Centralized Git Repository Synchronization Daemon**

A robust, production-ready Go application that provides automated synchronization for multiple Git repositories through a centralized daemon service with systemd integration.

## Features

- **Automated Sync**: Continuous background synchronization of multiple repositories
- **Flexible Branch Strategies**: Support for current, main, all, and **specific branch** syncing
- **Branch Switching**: Automatically switch to target branch, sync, and switch back
- **Centralized Configuration**: TOML-based configuration with hot-reload support
- **Concurrent Operations**: Configurable concurrent sync limits for performance
- **Safety First**: Comprehensive safety checks and uncommitted change detection
- **Systemd Integration**: Full systemd user service support with auto-start
- **Desktop Notifications**: Real-time sync notifications via notify-send (Linux)
- **Status Monitoring**: Real-time status reporting and logging
- **Secure**: Uses existing SSH keys and Git credentials, no credential storage

## Quick Start

### 1. Install

```bash
# Build from source
go build -o git-sync .
sudo install git-sync /usr/local/bin/

# Or install directly
go install github.com/bnema/git-sync@latest
```

### 2. Initialize a Repository

```bash
cd /path/to/your/repo
git sync init                    # Interactive setup with guided prompts
git sync init -d both -i 600     # Non-interactive: both directions, 10min interval
git sync init --branch-strategy specific --target-branch develop
```

### 3. Install Daemon

```bash
git sync install-daemon          # Install systemd user service
```

### 4. Monitor

```bash
git sync status                  # Show current repo status
git sync status --all            # Show all configured repos
git sync status --daemon         # Show daemon status
```

## Configuration

Configuration is stored in `~/.config/git-sync/config.toml`:

```toml
[global]
log_level = "info"
default_interval = 300      # 5 minutes
max_concurrent_syncs = 5
enable_notifications = true # Desktop notifications (Linux only)
notification_timeout = 5000 # Notification timeout in milliseconds

[[repositories]]
path = "/home/user/projects/my-app"
enabled = true
direction = "push"          # push, pull, both
interval = 300              # seconds
remote = "origin"
branch_strategy = "current" # current, main, all, specific
target_branch = ""          # only used with 'specific' strategy
safety_checks = true
force_push = false

[[repositories]]
path = "/home/user/repos/dotfiles"
enabled = true
direction = "both"
interval = 600
remote = "origin"
branch_strategy = "specific"
target_branch = "main"      # always sync main branch
safety_checks = true
force_push = false
```

## Branch Strategies

### `current` (default)
Syncs whatever branch you're currently on.

### `main`
Always syncs the `main` branch.

### `all`
Syncs all branches (fetch/push --all).

### `specific`
**Advanced feature**: Syncs only a specified branch with automatic branch switching.

```bash
# Use current branch as target
git sync init --branch-strategy specific

# Specify exact branch to sync
git sync init --branch-strategy specific --target-branch develop

# Example: Always keep develop branch in sync regardless of current branch
git sync init --branch-strategy specific --target-branch develop -d both
```

**How `specific` strategy works:**
1. Detects current branch
2. Checks for uncommitted changes (fails safely if found)
3. Switches to target branch
4. Performs sync operation
5. Switches back to original branch
6. Creates local tracking branch from remote if needed

## Commands

### `git sync init`
Initialize current repository for sync daemon.

```bash
git sync init                 # Interactive setup with prompts
git sync init [flags]         # Non-interactive with flags

Flags:
  --branch-strategy string   Branch strategy: current, main, all, specific (default "current")
  -d, --direction string     Sync direction: push, pull, both (default "push")
  --force                    Enable force push (use with caution)
  -i, --interval int         Sync interval in seconds (default 300)
  -r, --remote string        Git remote name (default "origin")
  --safety-checks            Enable safety checks (default true)
  --target-branch string     Target branch (for 'specific' strategy)
```

### `git sync status`
Show sync status for repositories.

```bash
git sync status [flags]

Flags:
  --all      Show all configured repositories
  --daemon   Show daemon status
```

### `git sync edit`
Open the configuration file in your default editor.

```bash
git sync edit
```

Uses the `EDITOR` environment variable to determine which editor to use. Creates a default configuration file if none exists.

### `git sync history`
Show synchronization history for repositories.

```bash
git sync history [flags]

Flags:
  --all         Show history for all repositories
  --limit int   Limit number of entries (default 20)
  --repo string Specific repository path to show history for
```

### `git sync daemon`
Run the sync daemon (usually via systemd).

### `git sync install-daemon`
Install systemd user service.

```bash
git sync install-daemon [flags]

Flags:
  --auto-start        Start daemon after installation (default true)
  --enable-linger     Enable systemd user lingering (default true)
  --uninstall         Uninstall the systemd service
```

### `git sync notifications`
Configure desktop notifications for sync events.

```bash
git sync notifications [enable|disable|status]

Examples:
  git sync notifications enable   # Enable desktop notifications
  git sync notifications disable  # Disable desktop notifications  
  git sync notifications status   # Show current notification settings
```

**Note**: Desktop notifications require `notify-send` (available on most Linux distributions). Notifications show sync success/failure with repository name, direction, duration, and error details.

## Advanced Usage

### Multiple Repository Setup

```bash
# Interactive setup for first repository  
cd ~/projects/webapp  
git sync init                            # Interactive prompts

# Non-interactive setup for additional repositories
cd ~/projects/api
git sync init -d both --branch-strategy specific --target-branch develop

cd ~/dotfiles
git sync init -d both --branch-strategy current -i 1800  # 30min interval
```

### Daemon Management

```bash
# Check daemon status
systemctl --user status git-sync-daemon.service

# View logs
journalctl --user -u git-sync-daemon -f

# Restart daemon
systemctl --user restart git-sync-daemon.service

# Reload configuration (or send SIGHUP)
systemctl --user reload git-sync-daemon.service
```

### Configuration Hot-Reload

The daemon supports configuration hot-reload via SIGHUP:

```bash
# Edit config file
vim ~/.config/git-sync/config.toml

# Reload without restart
systemctl --user reload git-sync-daemon.service
```

### Desktop Notifications

Git Sync provides desktop notifications for sync events on Linux systems:

```bash
# Enable notifications
git sync notifications enable

# Check current status
git sync notifications status

# Disable notifications
git sync notifications disable
```

**Notification Types:**
- **Success**: ✓ Git Sync: repo-name (with sync direction and duration)
- **Failure**: ✗ Git Sync Failed: repo-name (with error details)

**Requirements:**
- Linux desktop environment with `notify-send` (libnotify)
- Enabled in configuration (default: enabled for new installations)

## Safety Features

- **Uncommitted Change Detection**: Prevents branch switching with dirty working tree
- **Merge Conflict Handling**: Detects and reports merge conflicts
- **Safe Defaults**: No force push by default, safety checks enabled
- **Remote Validation**: Verifies remote exists and is reachable
- **Branch Existence Checks**: Ensures target branches exist before switching
- **Graceful Error Handling**: Continues processing other repos if one fails

## Architecture

```
git-sync/
├── main.go                    # Entry point
├── cmd/                      
│   ├── root.go               # Root command
│   ├── init.go               # Repository initialization
│   ├── status.go             # Status reporting
│   ├── daemon.go             # Daemon command
│   └── install_daemon.go     # Systemd installation
├── internal/
│   ├── config/              # Configuration management
│   ├── daemon/              # Core daemon logic
│   │   ├── daemon.go        # Main daemon
│   │   ├── git_operations.go # Native Git operations (go-git)
│   │   ├── sync.go          # Sync management
│   │   └── scheduler.go     # Timing and scheduling
│   ├── notification/        # Desktop notification system
│   └── systemd/             # Systemd integration
```

## Troubleshooting

### Common Issues

**Daemon won't start:**
```bash
# Check logs
journalctl --user -u git-sync-daemon -f

# Verify configuration
git sync status --daemon
```

**Branch switching fails:**
```bash
# Usually due to uncommitted changes
git status
git stash  # or commit changes
```

**Permission issues:**
```bash
# Ensure proper SSH key setup
ssh -T git@github.com
```

### Debug Mode

```bash
# Run with verbose output
git sync --verbose status --all

# Run daemon in foreground for debugging
git sync daemon --config ~/.config/git-sync/config.toml
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [Cobra](https://github.com/spf13/cobra) for CLI
- Uses [go-git](https://github.com/go-git/go-git) for native Git operations
- [systemd](https://systemd.io/) integration for service management