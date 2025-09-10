package yakdiff

import (
	"bufio"
	"bytes"
	"context"
	"os"
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
			bufline := bufio.NewReader(bytes.NewBufferString(patch.String()))
			var buf bytes.Buffer
			for i := 0; i < 2; i++ {
				firstline, err := utils.BufioReadLine(bufline)
				if err != nil {
					*result += patch.String()
					return nil
				}
				if bytes.HasPrefix(firstline, []byte("diff --git")) {
					continue
				}
				if bytes.HasPrefix(firstline, []byte("index ")) {
					continue
				}
				buf.Write(firstline)
				buf.WriteByte('\n')
			}
			bufline.WriteTo(&buf)
			*result += buf.String()
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

	// 如果内容相同，直接返回，没有差异
	if bytes.Equal(r1, r2) {
		return nil
	}

	storage := memory.NewStorage()
	repo, err := git.Init(storage, memfs.New())
	if err != nil {
		return utils.Wrap(err, "init git repos")
	}
	wt, err := repo.Worktree()
	if err != nil {
		return utils.Wrap(err, "get worktree")
	}

	filename := "content"

	// 第一次提交
	fp, err := wt.Filesystem.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return utils.Wrap(err, "open file")
	}
	fp.Write(r1)
	fp.Close()

	_, err = wt.Add(filename)
	if err != nil {
		return utils.Wrap(err, "add file")
	}

	commit1, err := wt.Commit("first version", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Yaklang",
			Email: "yaklang@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return utils.Wrap(err, "commit")
	}

	// 修改文件内容
	fp, err = wt.Filesystem.OpenFile(filename, os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return utils.Wrap(err, "reopen file")
	}
	fp.Write(r2)
	fp.Close()

	_, err = wt.Add(filename)
	if err != nil {
		return utils.Wrap(err, "add modified file")
	}

	commit2, err := wt.Commit("second version", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Yaklang",
			Email: "yaklang@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return utils.Wrap(err, "commit")
	}

	// 获取两个 commit 对象
	commit1Obj, err := repo.CommitObject(commit1)
	if err != nil {
		return utils.Wrap(err, "get commit1 object")
	}
	commit2Obj, err := repo.CommitObject(commit2)
	if err != nil {
		return utils.Wrap(err, "get commit2 object")
	}

	// 获取两个 tree
	tree1, err := commit1Obj.Tree()
	if err != nil {
		return utils.Wrap(err, "get tree1")
	}
	tree2, err := commit2Obj.Tree()
	if err != nil {
		return utils.Wrap(err, "get tree2")
	}

	// 进行 diff
	changes, err := tree1.DiffContext(ctx, tree2)
	if err != nil {
		return utils.Wrap(err, "diff")
	}

	for _, change := range changes {
		patch, _ := change.Patch()
		for _, handle := range handler {
			err := handle(commit2Obj, change, patch)
			if err != nil {
				return utils.Wrap(err, "handle change failed")
			}
		}
	}

	return nil
}
