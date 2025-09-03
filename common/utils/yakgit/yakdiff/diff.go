package yakdiff

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type DiffHandler func(*object.Commit, *object.Change, *object.Patch) error

// stringCollectorHandler 收集所有 patch 内容到字符串
func stringCollectorHandler(result *string) DiffHandler {
	return func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch != nil {
			*result += patch.String()
		}
		return nil
	}
}

// DiffToString 比较两个输入并返回 diff 结果字符串
func DiffToString(raw1, raw2 any) (string, error) {
	return DiffToStringContext(context.Background(), raw1, raw2)
}

// DiffToStringContext 带上下文的字符串 diff 比较
func DiffToStringContext(ctx context.Context, raw1, raw2 any) (string, error) {
	var result string
	err := DiffContext(ctx, raw1, raw2, stringCollectorHandler(&result))
	return result, err
}

// Diff 比较两个输入并返回 diff 结果字符串（为了向后兼容，现在返回字符串）
func Diff(raw1, raw2 any, handler ...DiffHandler) (string, error) {
	if len(handler) > 0 {
		// 如果提供了处理器，保持原有行为但返回空字符串
		err := DiffContext(context.Background(), raw1, raw2, handler...)
		return "", err
	}
	// 如果没有提供处理器，返回 diff 字符串
	return DiffToString(raw1, raw2)
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
