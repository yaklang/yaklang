package coding

import (
	"context"
	"strings"
)

type Branch struct {
	Name   string
	Ref    string
	Commit string
	Remote bool
}

type Branches struct {
	Data            []Branch
	CurrentBranch   string
	CurrentCommit   string
	DefaultRef      string
	HasLocalChanges bool
}

// ListBranches reads the primary checkout only. Remote refs are the engine's
// locally known refs; opening a picker never fetches or changes a checkout.
func ListBranches(ctx context.Context, source string) (*Branches, error) {
	repo, err := InspectRepository(ctx, source)
	if err != nil {
		return nil, err
	}
	result := &Branches{DefaultRef: "HEAD"}
	output, err := git(ctx, repo.Root, "for-each-ref", "--sort=refname", "--format=%(refname)%00%(objectname)%00%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(output), "\n"), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 || fields[2] != "" {
			continue // omit symbolic remote HEAD aliases
		}
		ref := fields[0]
		remote := strings.HasPrefix(ref, "refs/remotes/")
		name := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/remotes/")
		result.Data = append(result.Data, Branch{Name: name, Ref: ref, Commit: fields[1], Remote: remote})
	}
	if output, e := git(ctx, repo.Root, "symbolic-ref", "--quiet", "--short", "HEAD"); e == nil {
		result.CurrentBranch = strings.TrimSpace(string(output))
	}
	// An unborn repository can still be displayed, but has no commit from which
	// to create a worktree. The client disables creation until a commit exists.
	if output, e := git(ctx, repo.Root, "rev-parse", "--verify", "HEAD^{commit}"); e == nil {
		result.CurrentCommit = strings.TrimSpace(string(output))
	}
	output, err = git(ctx, repo.Root, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return nil, err
	}
	result.HasLocalChanges = len(output) > 0
	for _, candidate := range []string{"@{upstream}", "refs/remotes/origin/HEAD"} {
		if output, e := git(ctx, repo.Root, "rev-parse", "--symbolic-full-name", "--verify", candidate); e == nil {
			ref := strings.TrimSpace(string(output))
			if candidate == "refs/remotes/origin/HEAD" {
				if resolved, e := git(ctx, repo.Root, "symbolic-ref", candidate); e == nil {
					ref = strings.TrimSpace(string(resolved))
				}
			}
			for _, branch := range result.Data {
				if branch.Ref == ref {
					result.DefaultRef = ref
					return result, nil
				}
			}
		}
	}
	return result, nil
}
