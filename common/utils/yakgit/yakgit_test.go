package yakgit

import (
	"bytes"
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-rod/rod/lib/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed test-repo.zip
var testRepo []byte

func TestGitQuick(t *testing.T) {
	name := getTestGitRepo(t)
	result := GetHeadHash(name)
	require.Equal(t, result, "184d4e3f162cf58da2a4acf4346005a82cf97606")
	result = Glance(name)
	_ = result
	fmt.Println(result)
	parentHashExpect := "745f35e4fd4c1d8cfbc12495f04b989abf9f3437"
	parentHash, _ := GetParentCommitHash(name, "184d4e3f162cf58da2a4acf4346005a82cf97606")
	require.Equal(t, parentHash, parentHashExpect)
	parentHash, _ = GetParentCommitHash(name, "HEAD")
	require.Equal(t, parentHash, parentHashExpect)
	parentHash, _ = GetParentCommitHash(name, "HEAD^")
	require.Equal(t, parentHash, "")
	parentHash, _ = RevParse(name, "HEAD^")
	require.Equal(t, parentHash, "745f35e4fd4c1d8cfbc12495f04b989abf9f3437")
	parentHash, _ = RevParse(name, "HEAD~1")
	require.Equal(t, parentHash, "745f35e4fd4c1d8cfbc12495f04b989abf9f3437")
	parentHash, _ = RevParse(name, "master")
	require.Equal(t, parentHash, "184d4e3f162cf58da2a4acf4346005a82cf97606")
	parentHash, _ = RevParse(name, "refs/heads/master")
	require.Equal(t, parentHash, "184d4e3f162cf58da2a4acf4346005a82cf97606")
}

func TestFSCopy(t *testing.T) {
	zfs, err := filesys.NewZipFSFromString(string(testRepo))
	if err != nil {
		log.Error(err)
		return
	}
	lfs := filesys.CopyToTemporary(zfs)
	result := filesys.DumpTreeView(lfs)
	fmt.Println(lfs.Root())
	fmt.Println(result)
	require.True(t, strings.Contains(result, "c948f70187248df4de62a4ad62576939da349a"))
}

func getTestGitRepo(t *testing.T) string {
	zfs, err := filesys.NewZipFSFromString(string(testRepo))
	if err != nil {
		t.Fatal("cannot release zip fs to git fs")
	}
	lfs := filesys.CopyToTemporary(zfs)
	root := filepath.Join(lfs.Root(), "test-repo")
	log.Infof("release in %v", root)
	return root
}

func TestFSConverter(t *testing.T) {
	zfs, err := filesys.NewZipFSRaw(bytes.NewReader(testRepo), int64(len(testRepo)))
	if err != nil {
		t.Fatal(err)
	}

	baseDir := filepath.Join(consts.GetDefaultYakitBaseTempDir(), utils.RandString(8)) + "/"
	filesys.SimpleRecursive(
		filesys.WithFileSystem(zfs),
		filesys.WithDirStat(func(s string, info fs.FileInfo) error {
			os.MkdirAll(filepath.Join(baseDir, s), 0755)
			return nil
		}),
		filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			raw, err := zfs.ReadFile(s)
			if err != nil {
				return nil
			}
			err = os.WriteFile(filepath.Join(baseDir, s), raw, 0644)
			return nil
		}))
	fmt.Println(baseDir)
	repo := filepath.Join(baseDir, "test-repo")
	f, err := FromCommit(repo, `184d4e3f162cf58da2a4acf4346005a82cf97606`)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	raw, _ = f.ReadFile("./file1.txt")
	assert.Contains(t, string(raw), "Modified content of file1")
	raw, _ = f.ReadFile("file3.txt")
	assert.Contains(t, string(raw), `New file3 content`)
	raw, _ = f.ReadFile("file2.txt")
	assert.Empty(t, raw)

	f, err = FromCommit(repo, `745f35e4fd4c1d8cfbc12495f04b989abf9f3437`)
	if err != nil {
		t.Fatal(err)
	}
	showFS(f)
	raw, _ = f.ReadFile("./file1.txt")
	assert.Contains(t, string(raw), "Initial content of file1\n")
	raw, _ = f.ReadFile("file2.txt")
	assert.Contains(t, string(raw), "Initial content of file2\n")
	raw, _ = f.ReadFile("file3.txt")
	assert.Empty(t, raw)

	f, err = FromCommits(repo, "184d4e3f162cf58da2a4acf4346005a82cf97606", `745f35e4fd4c1d8cfbc12495f04b989abf9f3437`)
	if err != nil {
		t.Fatal(err)
	}
	showFS(f)
	raw, _ = f.ReadFile("./file1.txt")
	assert.Contains(t, string(raw), "Initial content of file1\n")
	raw, _ = f.ReadFile("file2.txt")
	assert.Contains(t, string(raw), "Initial content of file2\n")
	raw, _ = f.ReadFile("file3.txt")
	assert.Contains(t, string(raw), `New file3 content`)

	f, err = FromCommits(repo, `745f35e4fd4c1d8cfbc12495f04b989abf9f3437`, "184d4e3f162cf58da2a4acf4346005a82cf97606")
	if err != nil {
		t.Fatal(err)
	}
	showFS(f)
	raw, _ = f.ReadFile("./file1.txt")
	assert.Contains(t, string(raw), "Modified content of file1")
	raw, _ = f.ReadFile("file2.txt")
	assert.Contains(t, string(raw), "Initial content of file2\n")
	raw, _ = f.ReadFile("file3.txt")
	assert.Contains(t, string(raw), `New file3 content`)

	f, err = FromCommitRange(repo, `745f35e4fd4c1d8cfbc12495f04b989abf9f3437`, "184d4e3f162cf58da2a4acf4346005a82cf97606")
	if err != nil {
		t.Fatal(err)
	}
	showFS(f)
	// FromCommitRange 现在只返回变更的文件，不包含未变更的文件
	raw, _ = f.ReadFile("./file1.txt")
	spew.Dump(raw)
	assert.Contains(t, string(raw), "Modified content of file1")
	// file2.txt 未变更，所以不应该在 diff 结果中
	raw, _ = f.ReadFile("file2.txt")
	assert.Empty(t, raw, "file2.txt should not be in diff (unchanged file)")
	spew.Dump(raw)
	raw, _ = f.ReadFile("file3.txt")
	spew.Dump(raw)
	assert.Contains(t, string(raw), `New file3 content`)

}

func showFS(fi filesys_interface.FileSystem) {
	fmt.Println(filesys.DumpTreeView(fi))
}

func TestFSConverter_Short(t *testing.T) {
	zfs, err := filesys.NewZipFSRaw(bytes.NewReader(testRepo), int64(len(testRepo)))
	if err != nil {
		t.Fatal(err)
	}

	baseDir := filepath.Join(consts.GetDefaultYakitBaseTempDir(), utils.RandString(8)) + "/"
	filesys.SimpleRecursive(
		filesys.WithFileSystem(zfs),
		filesys.WithDirStat(func(s string, info fs.FileInfo) error {
			os.MkdirAll(filepath.Join(baseDir, s), 0755)
			return nil
		}),
		filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			raw, err := zfs.ReadFile(s)
			if err != nil {
				return nil
			}
			err = os.WriteFile(filepath.Join(baseDir, s), raw, 0644)
			return nil
		}))
	fmt.Println(baseDir)
	repo := filepath.Join(baseDir, "test-repo")
	f, err := FromCommit(repo, `184d4e3f162`)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	raw, _ = f.ReadFile("./file1.txt")
	assert.Contains(t, string(raw), "Modified content of file1")
	raw, _ = f.ReadFile("file3.txt")
	assert.Contains(t, string(raw), `New file3 content`)
	raw, _ = f.ReadFile("file2.txt")
	assert.Empty(t, raw)
}

// TestFromCommitRangeWithBranch 测试使用分支名获取文件系统的功能
// 场景：创建 main 分支和 feature 分支，测试从 main 到 feature 分支的 diff
func TestFromCommitRangeWithBranch(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "yakgit-test-branch-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 初始化 git 仓库
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// 获取 worktree
	wt, err := repo.Worktree()
	require.NoError(t, err)

	// 在 main 分支上：第一次提交：创建 file1.txt（必须先有提交才能获取 HEAD）
	file1Path := "file1.txt"
	err = os.WriteFile(filepath.Join(tmpDir, file1Path), []byte("Initial content of file1\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add(file1Path)
	require.NoError(t, err)

	commit1Hash, err := wt.Commit("Commit 1: add file1 on main", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	t.Logf("Commit 1 hash: %s", commit1Hash.String())

	// 获取当前分支名（可能是 master 或 main），现在 HEAD 已经存在
	headRef, err := repo.Head()
	require.NoError(t, err)
	mainBranchName := headRef.Name().Short()
	t.Logf("Default branch name: %s", mainBranchName)

	// 在 main 分支上：第二次提交：添加 file2.txt
	file2Path := "file2.txt"
	err = os.WriteFile(filepath.Join(tmpDir, file2Path), []byte("Content of file2 on main\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add(file2Path)
	require.NoError(t, err)

	commit2Hash, err := wt.Commit("Commit 2: add file2 on main", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	t.Logf("Commit 2 hash: %s", commit2Hash.String())

	// 创建 feature 分支
	featureBranchName := "feature/test-branch"
	headRef, err = repo.Head()
	require.NoError(t, err)

	// 创建新分支引用
	featureRef := plumbing.NewBranchReferenceName(featureBranchName)
	err = repo.Storer.SetReference(plumbing.NewHashReference(featureRef, headRef.Hash()))
	require.NoError(t, err)

	// 切换到 feature 分支
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: featureRef,
		Create: false,
	})
	require.NoError(t, err)

	// 在 feature 分支上：修改 file1.txt
	err = os.WriteFile(filepath.Join(tmpDir, file1Path), []byte("Modified content of file1 on feature branch\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add(file1Path)
	require.NoError(t, err)

	commit3Hash, err := wt.Commit("Commit 3: modify file1 on feature", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	t.Logf("Commit 3 hash: %s", commit3Hash.String())

	// 在 feature 分支上：添加 file3.txt
	file3Path := "file3.txt"
	err = os.WriteFile(filepath.Join(tmpDir, file3Path), []byte("Content of file3 on feature branch\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add(file3Path)
	require.NoError(t, err)

	commit4Hash, err := wt.Commit("Commit 4: add file3 on feature", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	t.Logf("Commit 4 hash: %s", commit4Hash.String())

	// 辅助函数：获取分支的完整文件树
	getBranchFileTree := func(branchName string) (*filesys.VirtualFS, error) {
		res, err := GitOpenRepositoryWithCache(tmpDir)
		if err != nil {
			return nil, err
		}
		branchRef, err := res.Reference(plumbing.ReferenceName("refs/heads/"+branchName), true)
		if err != nil {
			return nil, err
		}
		commit, err := res.CommitObject(branchRef.Hash())
		if err != nil {
			return nil, err
		}
		files, err := commit.Files()
		if err != nil {
			return nil, err
		}
		vfs := filesys.NewVirtualFs()
		err = files.ForEach(func(file *object.File) error {
			raw, err := file.Contents()
			if err != nil {
				return err
			}
			vfs.AddFile(file.Name, raw)
			return nil
		})
		return vfs, err
	}

	// 测试：使用分支名从 main 到 feature 分支
	t.Run("FromCommitRange-main-to-feature-branch", func(t *testing.T) {
		// 打印 main 分支的文件树
		mainFs, err := getBranchFileTree(mainBranchName)
		require.NoError(t, err)
		t.Logf("=== %s branch file tree ===", mainBranchName)
		filesys.TreeView(mainFs)

		// 打印 feature 分支的文件树
		featureFs, err := getBranchFileTree(featureBranchName)
		require.NoError(t, err)
		t.Logf("=== %s branch file tree ===", featureBranchName)
		filesys.TreeView(featureFs)

		// 说明修改情况
		t.Logf("=== Changes from %s to %s ===", mainBranchName, featureBranchName)
		t.Logf("  - file1.txt: MODIFIED (content changed)")
		t.Logf("  - file2.txt: UNCHANGED (same content)")
		t.Logf("  - file3.txt: ADDED (new file)")

		// 使用分支名而不是 commit hash
		fs, err := FromCommitRange(tmpDir, mainBranchName, featureBranchName)
		require.NoError(t, err, "should return feature branch's file system diff from main")

		// 打印 diff 文件树（使用与 gitefs 命令相同的功能）
		t.Logf("=== Diff file tree (from %s to %s) ===", mainBranchName, featureBranchName)
		filesys.TreeView(fs)

		// file1 应该被修改（变更的文件）
		raw, err := fs.ReadFile(file1Path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "Modified content of file1 on feature branch")

		// file2 不应该存在（未变更的文件）
		exists, _ := fs.Exists(file2Path)
		assert.False(t, exists, "file2 should not exist in diff (unchanged file)")

		// file3 应该存在（feature 分支新增，变更的文件）
		raw, err = fs.ReadFile(file3Path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "Content of file3 on feature branch")
	})

	// 测试：使用分支名从 feature 到 main（反向）
	// 应该只返回变更的文件：file1.txt（修改）和 file3.txt（删除）
	t.Run("FromCommitRange-feature-to-main-branch", func(t *testing.T) {
		// 打印 feature 分支的文件树
		featureFs, err := getBranchFileTree(featureBranchName)
		require.NoError(t, err)
		t.Logf("=== %s branch file tree ===", featureBranchName)
		filesys.TreeView(featureFs)

		// 打印 main 分支的文件树
		mainFs, err := getBranchFileTree(mainBranchName)
		require.NoError(t, err)
		t.Logf("=== %s branch file tree ===", mainBranchName)
		filesys.TreeView(mainFs)

		// 说明修改情况
		t.Logf("=== Changes from %s to %s ===", featureBranchName, mainBranchName)
		t.Logf("  - file1.txt: MODIFIED (revert to main content)")
		t.Logf("  - file2.txt: UNCHANGED (not in diff)")
		t.Logf("  - file3.txt: DELETED (removed in main)")

		fs, err := FromCommitRange(tmpDir, featureBranchName, mainBranchName)
		require.NoError(t, err, "should return changed files diff from feature to main")

		// 打印 diff 文件树（使用与 gitefs 命令相同的功能）
		t.Logf("=== Diff file tree (from %s to %s) ===", featureBranchName, mainBranchName)
		filesys.TreeView(fs)

		// file1 应该恢复为 main 的内容（变更的文件）
		raw, err := fs.ReadFile(file1Path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "Initial content of file1")

		// file2 不应该存在（未变更的文件）
		exists, _ := fs.Exists(file2Path)
		assert.False(t, exists, "file2 should not exist in diff (unchanged file)")

		// file3 应该不存在（main 分支没有这个文件，diff 中会删除）
		exists, _ = fs.Exists(file3Path)
		assert.False(t, exists, "file3 should not exist in main branch")
	})

	// 测试：使用 refs/heads/ 前缀的分支名
	t.Run("FromCommitRange-with-refs-prefix", func(t *testing.T) {
		fs, err := FromCommitRange(tmpDir, "refs/heads/"+mainBranchName, "refs/heads/"+featureBranchName)
		require.NoError(t, err, "should work with refs/heads/ prefix")

		raw, err := fs.ReadFile(file1Path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "Modified content of file1 on feature branch")
	})
}
