package yakdiff

import (
	"context"
	"fmt"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"os"
	"path"
	"time"
)

type DiffHandler func(*object.Commit, *object.Change, *object.Patch) error

func Diff(raw1, raw2 any, handler ...DiffHandler) error {
	return DiffContext(context.Background(), raw1, raw2, handler...)
}

func DiffContext(ctx context.Context, raw1, raw2 any, handler ...DiffHandler) error {
	if len(handler) == 0 {
		handler = append(handler, _defaultPatchHandler)
	}

	r1, r2 := codec.AnyToBytes(raw1), codec.AnyToBytes(raw2)

	storage := memory.NewStorage()
	repo, err := git.Init(storage, memfs.New())
	if err != nil {
		return utils.Wrap(err, "init git repos")
	}
	wt, err := repo.Worktree()
	if err != nil {
		return utils.Wrap(err, "get worktree")
	}
	err = wt.Filesystem.MkdirAll("main", 0755)
	if err != nil {
		return utils.Wrap(err, "mkdir main")
	}

	filename := path.Join("main", "main.txt")
	commitAndGetTree := func(content []byte) (*object.Commit, *object.Tree, error) {
		fp, err := wt.Filesystem.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, nil, utils.Wrap(err, "open file")
		}

		fp.Write(content)
		fp.Close()
		_, err = wt.Add(filename)
		if err != nil {
			return nil, nil, utils.Wrap(err, "add file")
		}
		commit, err := wt.Commit("add first file", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Yaklang",
				Email: "yaklang@example.com",
				When:  time.Now(),
			},
		})
		if err != nil {
			return nil, nil, utils.Wrap(err, "commit")
		}
		_ = commit
		commitIns, err := repo.CommitObject(commit)
		if err != nil {
			return nil, nil, utils.Wrap(err, "get commit object")
		}
		tree, err := commitIns.Tree()
		if err != nil {
			return nil, nil, utils.Wrap(err, "get tree")
		}
		return commitIns, tree, nil
	}

	commit1, tree1, err := commitAndGetTree(r1)
	if err != nil {
		return utils.Wrap(err, "commitAndGetTree(1)")
	}
	wt.Filesystem.Remove(filename)

	commit2, tree2, err := commitAndGetTree(r2)
	changes, err := tree1.DiffContext(ctx, tree2)
	if err != nil {
		return utils.Wrap(err, "diff")
	}
	_ = commit1
	_ = commit2
	for _, i := range changes {
		patch, _ := i.Patch()
		for _, handle := range handler {
			err := handle(commit2, i, patch)
			if err != nil {
				return utils.Wrap(err, "handle change failed")
			}
		}

		if len(handler) <= 0 {
			patch, err := i.Patch()
			if err != nil {
				continue
			}
			fmt.Println(i.String())
			fmt.Println(patch.String())
		}
	}
	return nil
}
