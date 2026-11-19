package coding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RecoverWorktree is only for a persisted reservation that has never become
// ready. A complete checkout is adopted without touching its files. Incomplete
// checkouts are removed before retrying the same immutable commit.
func RecoverWorktree(ctx context.Context, root, destination, commonDir, cwd, commit string) error {
	if _, err := InspectWorktree(ctx, destination, commonDir, cwd); err == nil {
		index, err := git(ctx, destination, "rev-parse", "--path-format=absolute", "--git-path", "index")
		if err != nil {
			return err
		}
		if _, err := os.Stat(strings.TrimSpace(string(index))); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if err := RemoveWorktree(ctx, root, destination, commonDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	return CreateWorktree(ctx, root, destination, commit)
}

// RemoveWorktree must only receive an owned managed path, checked by the caller.
// Git unregisters complete checkouts; interrupted creations can leave just a
// directory or a stale registration, which require targeted cleanup.
func RemoveWorktree(ctx context.Context, root, destination, commonDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repo, err := InspectRepository(ctx, root)
	if err != nil {
		return err
	}
	if repo.CommonDir != commonDir {
		return fmt.Errorf("source repository identity changed")
	}
	if !filepath.IsAbs(destination) || Contains(destination, repo.Root) || Contains(destination, commonDir) {
		return fmt.Errorf("refusing to remove a source repository")
	}
	linkedGitFile := false
	info, err := os.Lstat(destination)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed worktree path is not a directory")
		}
		// Never remove an unrelated checkout that replaced our reservation.
		gitFile := filepath.Join(destination, ".git")
		if gitInfo, statErr := os.Lstat(gitFile); statErr == nil {
			if !gitInfo.Mode().IsRegular() {
				return fmt.Errorf("managed worktree .git is not a regular file")
			}
			if err := validateLinkedWorktreeFile(gitFile, commonDir); err != nil {
				return err
			}
			linked, inspectErr := InspectRepository(ctx, destination)
			if inspectErr == nil {
				if linked.Root != destination || linked.CommonDir != commonDir {
					return fmt.Errorf("managed worktree repository identity changed")
				}
				linkedGitFile = true
			}
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
	}
	trees, err := ListWorktrees(ctx, repo.Root, true)
	if err != nil {
		return err
	}
	for _, tree := range trees {
		if filepath.Clean(tree.Path) != destination {
			continue
		}
		if tree.Bare || tree.Path == repo.Root {
			return fmt.Errorf("refusing to remove a source checkout")
		}
		// Explicit deletion includes dirty and locked managed checkouts.
		if _, err := git(ctx, repo.Root, "worktree", "remove", "--force", "--force", "--", destination); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		} else if linkedGitFile {
			return err
		}
		// A crash can leave a registration without its .git backlink. Remove
		// only this reservation, then ask Git to unregister that exact path.
		if err := os.RemoveAll(destination); err != nil {
			return err
		}
		_, err := git(ctx, repo.Root, "worktree", "remove", "--force", "--force", "--", destination)
		return err
	}
	return os.RemoveAll(destination)
}

// Even when Git's administrative files are incomplete, the .git pointer and
// any surviving backlink must belong to this repository and this checkout.
func validateLinkedWorktreeFile(gitFile, commonDir string) error {
	content, err := os.ReadFile(gitFile)
	if err != nil {
		return err
	}
	pointer, ok := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir: ")
	if !ok {
		return fmt.Errorf("invalid managed worktree .git pointer")
	}
	if !filepath.IsAbs(pointer) {
		pointer = filepath.Join(filepath.Dir(gitFile), pointer)
	}
	pointer = filepath.Clean(pointer)
	if filepath.Dir(pointer) != filepath.Join(commonDir, "worktrees") {
		return fmt.Errorf("managed worktree repository identity changed")
	}
	for _, dir := range []string{filepath.Dir(pointer), pointer} {
		if info, err := os.Lstat(dir); err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("managed worktree administration directory changed")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	backlink, err := os.ReadFile(filepath.Join(pointer, "gitdir"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	backPath := strings.TrimSpace(string(backlink))
	if !filepath.IsAbs(backPath) {
		backPath = filepath.Join(pointer, backPath)
	}
	if filepath.Clean(backPath) != gitFile {
		return fmt.Errorf("managed worktree backlink points to another checkout")
	}
	return nil
}
