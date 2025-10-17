package yakdiff

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// fsStringCollectorHandler 文件系统diff专用的字符串收集处理器
func fsStringCollectorHandler(result *string) DiffHandler {
	return func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch != nil {
			*result += patch.String()
		}
		return nil
	}
}

// copyAndCommitFS 辅助函数，将一个文件系统的内容提交到当前工作树
func copyAndCommitFS(repo *git.Repository, wt *git.Worktree, originFS fi.FileSystem, msg string) (*object.Commit, error) {
	// 1. 完全清理工作区和暂存区
	err := wt.Clean(&git.CleanOptions{Dir: true})
	if err != nil {
		return nil, utils.Wrap(err, "worktree clean failed")
	}

	// 重置工作区到HEAD，这会清理暂存区
	err = wt.Reset(&git.ResetOptions{Mode: git.HardReset})
	if err != nil {
		// 如果没有HEAD（第一次提交），忽略这个错误
		if !strings.Contains(err.Error(), "reference not found") {
			return nil, utils.Wrap(err, "worktree reset failed")
		}
	}

	// 2. 复制文件到工作区
	err = filesys.Recursive(".", filesys.WithFileSystem(originFS), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		// 处理目录
		if isDir {
			return wt.Filesystem.MkdirAll(pathname, 0o755)
		}

		// 处理文件
		// 从源文件系统读取
		originFile, err := originFS.Open(pathname)
		if err != nil {
			return utils.Wrapf(err, "open from origin fs failed: %s", pathname)
		}
		defer originFile.Close()

		// 在 git worktree 中创建文件
		gitFile, err := wt.Filesystem.Create(pathname)
		if err != nil {
			return utils.Wrapf(err, "create in git fs failed: %s", pathname)
		}
		defer gitFile.Close()

		// 复制内容
		if _, err = io.Copy(gitFile, originFile); err != nil {
			return utils.Wrapf(err, "copy file content failed: %s", pathname)
		}

		// 3. 添加到暂存区
		if _, err = wt.Add(pathname); err != nil {
			return utils.Wrapf(err, "worktree add failed: %s", pathname)
		}
		return nil
	}))
	if err != nil {
		return nil, utils.Wrap(err, "recursive copy failed")
	}

	// 4. 提交 - 使用 All: true 来正确处理删除
	commitHash, err := wt.Commit(msg, &git.CommitOptions{
		All:    true, // 这是关键：自动暂存所有修改和删除的文件
		Author: &object.Signature{Name: "yaklang", Email: "yaklang@example.com", When: time.Now()},
	})

	// 如果是因为空提交失败，创建一个空文件然后提交
	if err != nil && strings.Contains(err.Error(), "empty commit") {
		// 创建一个空的.gitkeep文件来避免空提交错误
		gitkeepFile, err2 := wt.Filesystem.Create(".gitkeep")
		if err2 != nil {
			return nil, utils.Wrap(err, "original commit failed and cannot create .gitkeep")
		}
		gitkeepFile.Close()

		_, err2 = wt.Add(".gitkeep")
		if err2 != nil {
			return nil, utils.Wrap(err, "original commit failed and cannot add .gitkeep")
		}

		commitHash, err = wt.Commit(msg, &git.CommitOptions{
			All:    true,
			Author: &object.Signature{Name: "yaklang", Email: "yaklang@example.com", When: time.Now()},
		})
	}
	if err != nil {
		return nil, utils.Wrap(err, "worktree commit failed")
	}

	return repo.CommitObject(commitHash)
}

// FileSystemDiffToString 比较两个文件系统并返回diff字符串
func FileSystemDiffToString(fs1, fs2 fi.FileSystem) (string, error) {
	return FileSystemDiffToStringContext(context.Background(), fs1, fs2)
}

// FileSystemDiffToStringContext 带上下文的文件系统diff比较
func FileSystemDiffToStringContext(ctx context.Context, fs1, fs2 fi.FileSystem) (string, error) {
	var result string
	err := FileSystemDiffContext(ctx, fs1, fs2, fsStringCollectorHandler(&result))
	return result, err
}

// FileSystemDiff 比较两个文件系统并返回diff字符串（为了向后兼容，现在返回字符串）
func FileSystemDiff(fs1, fs2 fi.FileSystem, handler ...DiffHandler) (string, error) {
	if len(handler) > 0 {
		// 如果提供了处理器，保持原有行为但返回空字符串
		err := FileSystemDiffContext(context.Background(), fs1, fs2, handler...)
		return "", err
	}
	// 如果没有提供处理器，返回 diff 字符串
	return FileSystemDiffToString(fs1, fs2)
}

// FileSystemDiffContext 文件系统差异比较的核心实现（重构版本）
func FileSystemDiffContext(ctx context.Context, fs1, fs2 fi.FileSystem, handler ...DiffHandler) error {
	if len(handler) == 0 {
		handler = append(handler, _defaultPatchHandler)
	}

	storage := memory.NewStorage()
	fs := memfs.New()
	repo, err := git.Init(storage, fs)
	if err != nil {
		return utils.Wrap(err, "git.Init")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return utils.Wrap(err, "repo.Worktree()")
	}

	// 提交第一个文件系统
	commit1, err := copyAndCommitFS(repo, wt, fs1, "feat: Initial import of fs1")
	if err != nil {
		return utils.Wrap(err, "failed to commit fs1")
	}

	// 对于第二个文件系统，我们需要完全替换内容以检测删除
	// 1. 清理所有现有文件
	err = wt.RemoveGlob("*")
	if err != nil {
		return utils.Wrap(err, "failed to remove existing files")
	}

	// 2. 添加第二个文件系统的所有文件
	err = filesys.Recursive(".", filesys.WithFileSystem(fs2), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if isDir {
			return wt.Filesystem.MkdirAll(pathname, 0o755)
		}

		originFile, err := fs2.Open(pathname)
		if err != nil {
			return utils.Wrapf(err, "open from fs2 failed: %s", pathname)
		}
		defer originFile.Close()

		gitFile, err := wt.Filesystem.Create(pathname)
		if err != nil {
			return utils.Wrapf(err, "create in git fs failed: %s", pathname)
		}
		defer gitFile.Close()

		if _, err = io.Copy(gitFile, originFile); err != nil {
			return utils.Wrapf(err, "copy file content failed: %s", pathname)
		}

		if _, err = wt.Add(pathname); err != nil {
			return utils.Wrapf(err, "worktree add failed: %s", pathname)
		}
		return nil
	}))
	if err != nil {
		return utils.Wrap(err, "failed to copy fs2 files")
	}

	// 3. 提交所有变更（包括删除）
	commitHash, err := wt.Commit("feat: Update with fs2", &git.CommitOptions{
		All:    true,
		Author: &object.Signature{Name: "yaklang", Email: "yaklang@example.com", When: time.Now()},
	})

	if err != nil && strings.Contains(err.Error(), "empty commit") {
		gitkeepFile, err2 := wt.Filesystem.Create(".gitkeep")
		if err2 != nil {
			return utils.Wrap(err, "original commit failed and cannot create .gitkeep")
		}
		gitkeepFile.Close()

		_, err2 = wt.Add(".gitkeep")
		if err2 != nil {
			return utils.Wrap(err, "original commit failed and cannot add .gitkeep")
		}

		commitHash, err = wt.Commit("feat: Update with fs2", &git.CommitOptions{
			All:    true,
			Author: &object.Signature{Name: "yaklang", Email: "yaklang@example.com", When: time.Now()},
		})
	}
	if err != nil {
		return utils.Wrap(err, "failed to commit fs2")
	}

	commit2, err := repo.CommitObject(commitHash)
	if err != nil {
		return utils.Wrap(err, "failed to get commit2 object")
	}

	// 获取两个 commit 对应的 Tree
	tree1, err := commit1.Tree()
	if err != nil {
		return utils.Wrap(err, "get tree from commit1")
	}
	tree2, err := commit2.Tree()
	if err != nil {
		return utils.Wrap(err, "get tree from commit2")
	}

	// 计算差异
	changes, err := tree1.DiffContext(ctx, tree2)
	if err != nil {
		return utils.Wrap(err, "tree1.DiffContext(ctx, tree2)")
	}

	// 处理每个变更
	for _, change := range changes {
		patch, err := change.Patch()
		if err != nil {
			continue // 跳过无法生成patch的变更
		}

		// 调用所有处理器
		for _, handle := range handler {
			err := handle(commit2, change, patch)
			if err != nil {
				return utils.Wrap(err, "handle change failed")
			}
		}

		// 如果没有处理器，使用默认输出（这个逻辑实际上不会执行，因为上面已经添加了默认处理器）
		if len(handler) <= 0 {
			fmt.Println(change.String())
			fmt.Println(patch.String())
		}
	}
	return nil
}
