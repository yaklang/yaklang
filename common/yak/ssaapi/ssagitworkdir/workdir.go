package ssagitworkdir

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const (
	// WorkDirEnv overrides the parent directory used by SSA Git clones.
	WorkDirEnv = "YAK_SSA_GIT_WORKDIR"
	// MinFreeBytesEnv configures the minimum free space required before a clone starts.
	MinFreeBytesEnv = "YAK_SSA_GIT_MIN_FREE_BYTES"
	// OwnerEnv is an internal ownership token set by Scan Node for exact cleanup.
	OwnerEnv = "YAK_SSA_GIT_WORKSPACE_OWNER"

	DefaultMinFreeBytes uint64 = 1 << 30 // 1 GiB
	workspacePrefix            = "yakgit-"
)

var (
	diskUsage  = diskUsageForPath
	createTemp = os.CreateTemp
	mkdirTemp  = os.MkdirTemp
	removeAll  = os.RemoveAll
	removeFile = os.Remove
)

// ResolveRoot returns the absolute managed directory for SSA Git workspaces.
func ResolveRoot() (string, error) {
	root := strings.TrimSpace(os.Getenv(WorkDirEnv))
	if root == "" {
		root = filepath.Join(defaultYakitTempDir(), "ssa-git")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve SSA Git workdir %q: %w", root, err)
	}
	return filepath.Clean(abs), nil
}

func defaultYakitTempDir() string {
	if yakitHome := strings.TrimSpace(os.Getenv("YAKIT_HOME")); yakitHome != "" {
		return filepath.Join(yakitHome, "temp")
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = "."
	}
	return filepath.Join(home, "yakit-projects", "temp")
}

// Prepare validates the managed root and creates an isolated workspace owned by pid.
func Prepare(ctx context.Context, pid int) (string, func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if pid <= 0 {
		return "", nil, fmt.Errorf("prepare SSA Git workdir: invalid process id %d", pid)
	}
	root, err := ResolveRoot()
	if err != nil {
		return "", nil, err
	}
	minimum, err := configuredMinimumFreeBytes()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(root, 0750); err != nil {
		return "", nil, preflightPathError(ctx, root, "create directory", err)
	}
	if err := checkWritable(root); err != nil {
		return "", nil, preflightPathError(ctx, root, "write probe", err)
	}
	usage, err := diskUsage(ctx, root)
	if err != nil {
		return "", nil, fmt.Errorf("SSA Git workdir preflight failed: directory=%q: inspect free space: %w", root, err)
	}
	if usage.Free < minimum {
		return "", nil, fmt.Errorf(
			"SSA Git workdir preflight failed: directory=%q available_bytes=%d required_minimum_bytes=%d; free disk space, clean stale data, or set %s to a larger filesystem",
			root,
			usage.Free,
			minimum,
			WorkDirEnv,
		)
	}
	if usage.InodesTotal > 0 && usage.InodesFree == 0 {
		return "", nil, fmt.Errorf(
			"SSA Git workdir preflight failed: directory=%q available_bytes=%d available_inodes=0; free inodes or set %s to another filesystem",
			root,
			usage.Free,
			WorkDirEnv,
		)
	}

	owner, err := currentOwner(pid)
	if err != nil {
		return "", nil, err
	}
	workspace, err := mkdirTemp(root, workspaceNamePrefix(owner))
	if err != nil {
		return "", nil, preflightPathError(ctx, root, "create isolated workspace", err)
	}
	var once sync.Once
	var cleanupErr error
	cleanup := func() error {
		once.Do(func() {
			cleanupErr = removeAll(workspace)
		})
		return cleanupErr
	}
	return workspace, cleanup, nil
}

// CleanupForOwner removes only SSA Git workspaces owned by a finished child process.
func CleanupForOwner(owner string) error {
	if err := validateOwner(owner); err != nil {
		return err
	}
	root, err := ResolveRoot()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read SSA Git workdir %q for owner %q cleanup: %w", root, owner, err)
	}

	prefix := workspaceNamePrefix(owner)
	var cleanupErr error
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if err := removeAll(candidate); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove %q: %w", candidate, err))
		}
	}
	return cleanupErr
}

// WrapCloneError adds the storage evidence operators need when a clone fails,
// including failures that exhaust the filesystem after the initial preflight.
func WrapCloneError(ctx context.Context, workspace string, cloneErr error) error {
	if cloneErr == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	available := "unknown"
	if usage, err := diskUsage(ctx, workspace); err == nil {
		available = strconv.FormatUint(usage.Free, 10)
	}
	if isCapacityError(cloneErr) {
		return fmt.Errorf(
			"SSA Git clone failed: workspace=%q available_bytes=%s: %w; free disk space, clean stale data, or set %s to a larger filesystem",
			workspace,
			available,
			cloneErr,
			WorkDirEnv,
		)
	}
	return fmt.Errorf("SSA Git clone failed: workspace=%q available_bytes=%s: %w", workspace, available, cloneErr)
}

func workspaceNamePrefix(owner string) string {
	return workspacePrefix + owner + "-"
}

func currentOwner(pid int) (string, error) {
	owner := strings.TrimSpace(os.Getenv(OwnerEnv))
	if owner == "" {
		owner = "pid-" + strconv.Itoa(pid)
	}
	if err := validateOwner(owner); err != nil {
		return "", err
	}
	return owner, nil
}

func validateOwner(owner string) error {
	if owner == "" || len(owner) > 64 {
		return fmt.Errorf("invalid SSA Git workspace owner %q", owner)
	}
	for _, char := range owner {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			continue
		}
		return fmt.Errorf("invalid SSA Git workspace owner %q", owner)
	}
	return nil
}

func configuredMinimumFreeBytes() (uint64, error) {
	raw := strings.TrimSpace(os.Getenv(MinFreeBytesEnv))
	if raw == "" {
		return DefaultMinFreeBytes, nil
	}
	minimum, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: expected a non-negative byte count", MinFreeBytesEnv, raw)
	}
	return minimum, nil
}

func checkWritable(root string) error {
	probe, err := createTemp(root, ".write-check-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	closeErr := probe.Close()
	removeErr := removeFile(name)
	return errors.Join(closeErr, removeErr)
}

func preflightPathError(ctx context.Context, root string, action string, cause error) error {
	available := "unknown"
	if usagePath := nearestExistingPath(root); usagePath != "" {
		if usage, err := diskUsage(ctx, usagePath); err == nil {
			available = strconv.FormatUint(usage.Free, 10)
		}
	}
	if isCapacityError(cause) {
		return fmt.Errorf(
			"SSA Git workdir preflight failed: directory=%q available_bytes=%s: %s: %w; free disk space, clean stale data, or set %s to a larger filesystem",
			root,
			available,
			action,
			cause,
			WorkDirEnv,
		)
	}
	return fmt.Errorf(
		"SSA Git workdir preflight failed: directory=%q available_bytes=%s: %s: %w; set %s to a writable filesystem",
		root,
		available,
		action,
		cause,
		WorkDirEnv,
	)
}

func nearestExistingPath(candidate string) string {
	for {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return ""
		}
		candidate = parent
	}
}

func isCapacityError(err error) bool {
	if errors.Is(err, syscall.ENOSPC) || isPlatformCapacityError(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no space left on device") ||
		strings.Contains(message, "disk quota exceeded")
}
