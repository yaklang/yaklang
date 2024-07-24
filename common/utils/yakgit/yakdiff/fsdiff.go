package yakdiff

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func FileSystemDiffContext(ctx context.Context, fs1 fi.FileSystem, fs2 fi.FileSystem, handler ...DiffHandler) error {
	if len(handler) == 0 {
		handler = append(handler, _defaultPatchHandler)
	}

	storage := memory.NewStorage()

	rootFS := memfs.New()

	repo, err := git.Init(storage, rootFS)
	if err != nil {
		return utils.Wrap(err, "git.Init")
	}
	wt, err := repo.Worktree()
	if err != nil {
		return utils.Wrap(err, `repo.Worktree()`)
	}

	copyFs := func(wt *git.Worktree, gitFS billy.Filesystem, originFS fi.FileSystem) (retCommit *object.Commit, retTree *object.Tree, retErr error) {
		err = filesys.Recursive(".", filesys.WithFileSystem(originFS), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
			defer func() {
				wt.Add(pathname)
			}()
			if isDir {
				return gitFS.MkdirAll(pathname, 0o755)
			}
			f, err := gitFS.OpenFile(pathname, os.O_CREATE|os.O_RDWR, 0o755)
			if err != nil {
				return utils.Wrapf(err, `gitfs open %v`, pathname)
			}
			defer f.Close()
			origin, err := originFS.Open(pathname)
			if err != nil {
				return utils.Wrap(err, "origin fs1 open failed")
			}
			origin.Close()
			io.Copy(f, origin)
			return nil
		}))
		if err != nil {
			return nil, nil, utils.Wrap(err, `filesys.Recursive(fs1, ...)`)
		}
		commitHash, err := wt.Commit("add first filesystem", &git.CommitOptions{
			Author: &object.Signature{Name: "yaklang", Email: "yaklang@example.com", When: time.Now()},
		})
		if err != nil {
			retErr = utils.Wrap(err, `wt.Commit(fs1)`)
			return
		}
		commit, err := repo.CommitObject(commitHash)
		if err != nil {
			retErr = utils.Wrap(err, `repo.CommitObject(commitHash)`)
			return
		}
		retCommit = commit
		retTree, retErr = commit.Tree()
		return
	}
	commit1, tree1, err := copyFs(wt, wt.Filesystem, fs1)
	if err != nil {
		return utils.Wrap(err, "create fs1 failed")
	}

	err = wt.RemoveGlob("*")
	if err != nil {
		return utils.Wrap(err, "clean old git fs failed")
	}
	//filesys.Recursive("", filesys.WithFileSystem(fs1), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
	//
	//}))
	commit2, tree2, err := copyFs(wt, wt.Filesystem, fs2)
	if err != nil {
		return utils.Wrap(err, `create fs2 failed`)
	}
	_, _ = commit2, commit1

	changes, err := tree1.DiffContext(ctx, tree2)
	if err != nil {
		return utils.Wrap(err, `tree1.DiffContext(ctx, tree2)`)
	}
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
