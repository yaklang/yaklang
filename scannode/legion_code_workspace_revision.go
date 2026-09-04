package scannode

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yaklang/yaklang/common/utils/yakgit"
)

// restoreLegionCodeGitRevision works only in the freshly cloned, task-owned
// checkout. A branch moving after a Run was pinned must not change retry input.
// Fetch exactly the pinned commit when the shallow clone does not contain it;
// never fall back to HEAD or download unbounded history to hide a missing pin.
func restoreLegionCodeGitRevision(ctx context.Context, local, revision string, opts ...yakgit.Option) error {
	revision = strings.ToLower(strings.TrimSpace(revision))
	decoded, err := hex.DecodeString(revision)
	if err != nil || len(decoded) != 20 || revision == strings.Repeat("0", 40) {
		return fmt.Errorf("source_workspace revision mismatch: expected revision must be a full commit hash")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	repository, err := git.PlainOpen(local)
	if err != nil {
		return fmt.Errorf("source_workspace revision mismatch: task checkout is unavailable")
	}
	hash := plumbing.NewHash(revision)
	if _, err := repository.CommitObject(hash); err != nil {
		if !errors.Is(err, plumbing.ErrObjectNotFound) {
			return fmt.Errorf("source_workspace revision mismatch: pinned commit is unreadable")
		}
		err = yakgit.FetchExactRevision(ctx, local, revision, opts...)
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Transport errors can contain private remote/proxy details.
			return fmt.Errorf("source_workspace revision mismatch: pinned commit cannot be fetched from origin")
		}
		// FetchExactRevision opens its own storage. Reopen here so an earlier
		// missing-object lookup cannot retain the pre-fetch pack index.
		repository, err = git.PlainOpen(local)
		if err != nil {
			return fmt.Errorf("source_workspace revision mismatch: fetched checkout is unavailable")
		}
		if _, err := repository.CommitObject(hash); err != nil {
			return fmt.Errorf("source_workspace revision mismatch: pinned commit is unavailable")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return fmt.Errorf("source_workspace revision mismatch: task worktree is unavailable")
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		return fmt.Errorf("source_workspace revision mismatch: pinned commit cannot be checked out")
	}
	return ctx.Err()
}
