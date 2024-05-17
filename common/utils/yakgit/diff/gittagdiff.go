package diff

import (
	"context"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/utils"
)

func getCommit(repo *git.Repository, i string) (*object.Commit, error) {
	commit, _ := repo.CommitObject(plumbing.NewHash(i))
	if commit != nil {
		return commit, nil
	}
	tag, _ := repo.Tag(i)
	if tag != nil {
		tagIns, err := repo.TagObject(tag.Hash())
		if err != nil {
			return nil, utils.Wrap(err, `repo.TagObject(tag.Hash())`)
		}
		commit, err = tagIns.Commit()
		if err != nil {
			return nil, utils.Wrap(err, `tagIns.Commit()`)
		}
		return commit, nil
	}

	// repo branch
	// 尝试将 i 解释为分支名并获取对应的最新提交
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(i), false)
	if err == nil && branchRef != nil {
		branchCommit, err := repo.CommitObject(branchRef.Hash())
		if err != nil {
			return nil, utils.Wrap(err, "repo.CommitObject(branchRef.Hash())")
		}
		return branchCommit, nil
	}

	return nil, utils.Errorf("hash: %#v is not tag nor hash nor branch name", i)
}

// GitHashDiffContext compares the trees of two git commit hashes and processes the differences using the provided handlers.
func GitHashDiffContext(ctx context.Context, repo *git.Repository, hash1, hash2 string, handler ...DiffHandler) error {
	// Get the first commit using the first hash
	commit1, err := getCommit(repo, hash1)
	if err != nil {
		return err
	}

	// Get the second commit using the first hash
	commit2, err := getCommit(repo, hash2)
	if err != nil {
		return err
	}

	// Get the trees for each commit
	tree1, err := commit1.Tree()
	if err != nil {
		return utils.Wrap(err, "retrieve tree1 failed")
	}
	tree2, err := commit2.Tree()
	if err != nil {
		return utils.Wrap(err, "retrieve tree2 failed")
	}

	// Calculate the diff between the two trees
	changes, err := tree1.DiffContext(ctx, tree2)
	if err != nil {
		return utils.Wrap(err, "calculate diff failed")
	}

	// Process each change found
	for _, change := range changes {
		patch, err := change.Patch()
		if err != nil {
			continue // optionally handle error here
		}

		if len(handler) > 0 {
			for _, handle := range handler {
				err := handle(commit2, change, patch)
				if err != nil {
					return utils.Wrap(err, "handle change failed")
				}
			}
		} else {
			// If no handlers provided, just print the change and the corresponding patch
			fmt.Println(change.String())
			fmt.Println(patch.String())
		}
	}
	return nil
}
