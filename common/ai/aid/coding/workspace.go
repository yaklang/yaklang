// Package coding manages engine-host source directories independently of AI artifacts.
package coding

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Worktree struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Locked   bool
	Prunable bool
	Bare     bool
}

func Directory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("source directory must be an absolute engine-host path")
	}
	path, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source path is not a directory: %s", path)
	}
	return path, nil
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	// Ambient Git overrides must not redirect a workspace operation to another repo.
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func Source(ctx context.Context, path string) (root string, repository bool, err error) {
	root, err = Directory(path)
	if err != nil {
		return
	}
	if _, e := exec.LookPath("git"); e != nil {
		return root, false, nil
	}
	_, e := git(ctx, root, "rev-parse", "--show-toplevel")
	if e != nil {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		if bare, bareErr := git(ctx, root, "rev-parse", "--is-bare-repository"); bareErr == nil && strings.TrimSpace(string(bare)) == "true" {
			return "", false, fmt.Errorf("a bare Git repository is not a source checkout")
		}
		// A normal folder is a supported workspace; do not treat a broken repository as one.
		for dir := root; ; dir = filepath.Dir(dir) {
			if _, statErr := os.Lstat(filepath.Join(dir, ".git")); statErr == nil {
				return "", false, e
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
		return root, false, nil
	}
	return root, true, nil
}

type Repository struct {
	Root              string
	CommonDir         string
	RelativeDirectory string
}

// InspectRepository preserves the workspace's subdirectory within a checkout.
func InspectRepository(ctx context.Context, source string) (*Repository, error) {
	source, isGit, err := Source(ctx, source)
	if err != nil {
		return nil, err
	}
	if !isGit {
		return nil, fmt.Errorf("primary directory is not a Git repository")
	}
	output, err := git(ctx, source, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root, err := Directory(strings.TrimSuffix(string(output), "\n"))
	if err != nil {
		return nil, err
	}
	output, err = git(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	commonDir, err := Directory(strings.TrimSuffix(string(output), "\n"))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, source)
	if err != nil || !Contains(root, source) {
		return nil, fmt.Errorf("source directory is outside repository")
	}
	return &Repository{Root: root, CommonDir: commonDir, RelativeDirectory: rel}, nil
}

func Contains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// ResolveBase freezes a branch/ref to a commit and ensures the primary subdirectory
// exists in that commit before reserving any managed checkout.
func ResolveBase(ctx context.Context, repo *Repository, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	output, err := git(ctx, repo.Root, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(output))
	if repo.RelativeDirectory != "." {
		kind, err := git(ctx, repo.Root, "cat-file", "-t", commit+":"+filepath.ToSlash(repo.RelativeDirectory))
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(string(kind)) != "tree" {
			return "", fmt.Errorf("primary directory is not a directory in selected base")
		}
	}
	return commit, nil
}

// InspectWorktree verifies Git ownership using common-dir rather than trusting a
// persisted path that could now point to a different repository.
func InspectWorktree(ctx context.Context, path, commonDir, workingDirectory string) (*Worktree, error) {
	repo, err := InspectRepository(ctx, path)
	if err != nil {
		return nil, err
	}
	if repo.Root != filepath.Clean(path) || repo.CommonDir != commonDir {
		return nil, fmt.Errorf("managed worktree repository identity changed")
	}
	cwd, err := Directory(workingDirectory)
	if err != nil {
		return nil, err
	}
	if !Contains(repo.Root, cwd) {
		return nil, fmt.Errorf("working directory escapes managed worktree")
	}
	trees, err := ListWorktrees(ctx, repo.Root, true)
	if err != nil {
		return nil, err
	}
	for _, tree := range trees {
		if filepath.Clean(tree.Path) == repo.Root && !tree.Bare && !tree.Prunable {
			return &tree, nil
		}
	}
	return nil, fmt.Errorf("managed worktree is no longer registered with Git")
}

func ParseWorktrees(data []byte) []Worktree {
	var result []Worktree
	var current *Worktree
	for _, field := range strings.Split(string(data), "\x00") {
		if field == "" {
			current = nil
			continue
		}
		key, value, _ := strings.Cut(field, " ")
		if key == "worktree" {
			result = append(result, Worktree{Path: value})
			current = &result[len(result)-1]
			continue
		}
		if current == nil {
			continue
		}
		switch key {
		case "HEAD":
			current.Head = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			current.Detached = true
		case "locked":
			current.Locked = true
		case "prunable":
			current.Prunable = true
		case "bare":
			current.Bare = true
		}
	}
	return result
}

func ListWorktrees(ctx context.Context, root string, repository bool) ([]Worktree, error) {
	root, err := Directory(root)
	if err != nil {
		return nil, err
	}
	if !repository {
		return []Worktree{{Path: root}}, nil
	}
	output, err := git(ctx, root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return ParseWorktrees(output), nil
}

// CreateWorktree uses an isolated, detached checkout; it never switches the source branch.
func CreateWorktree(ctx context.Context, root, destination, base string) error {
	if !filepath.IsAbs(destination) {
		return fmt.Errorf("worktree destination must be absolute")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		return fmt.Errorf("worktree destination already exists or is inaccessible")
	}
	if base == "" {
		base = "HEAD"
	}
	_, err := git(ctx, root, "worktree", "add", "--detach", "--", destination, base)
	return err
}
