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
	raw, _ = f.ReadFile("./file1.txt")
	spew.Dump(raw)
	assert.Contains(t, string(raw), "Modified content of file1")
	raw, _ = f.ReadFile("file2.txt")
	assert.Contains(t, string(raw), "Initial content of file2\n")
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

// TestFromCommitRangeNoDirectChanges 测试当 start 和 end 之间没有直接变更时的场景
// 场景：commit1 修改 file1，commit2 修改 file2，commit3 撤销 file3 的修改
// 从 commit1 到 commit3 可能没有直接变更，但应该返回 commit3 的完整文件系统
func TestFromCommitRangeNoDirectChanges(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "yakgit-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 初始化 git 仓库
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// 获取 worktree
	wt, err := repo.Worktree()
	require.NoError(t, err)

	// 第一次提交：创建 file1.txt
	file1Path := "file1.txt"
	err = os.WriteFile(filepath.Join(tmpDir, file1Path), []byte("Initial content of file1\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add(file1Path)
	require.NoError(t, err)

	commit1Hash, err := wt.Commit("Commit 1: add file1", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	commit1HashStr := commit1Hash.String()
	t.Logf("Commit 1 hash: %s", commit1HashStr)

	// 第二次提交：修改 file1.txt，添加 file2.txt
	err = os.WriteFile(filepath.Join(tmpDir, file1Path), []byte("Modified content of file1\n"), 0644)
	require.NoError(t, err)

	file2Path := "file2.txt"
	err = os.WriteFile(filepath.Join(tmpDir, file2Path), []byte("Content of file2\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add(file1Path)
	require.NoError(t, err)
	_, err = wt.Add(file2Path)
	require.NoError(t, err)

	commit2Hash, err := wt.Commit("Commit 2: modify file1, add file2", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	commit2HashStr := commit2Hash.String()
	t.Logf("Commit 2 hash: %s", commit2HashStr)

	// 第三次提交：添加 file3.txt，然后删除它（撤销）
	file3Path := "file3.txt"
	err = os.WriteFile(filepath.Join(tmpDir, file3Path), []byte("Content of file3\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add(file3Path)
	require.NoError(t, err)

	commit3Hash, err := wt.Commit("Commit 3: add file3", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	commit3HashStr := commit3Hash.String()
	t.Logf("Commit 3 hash: %s", commit3HashStr)

	// 第四次提交：删除 file3.txt（撤销 commit3 的修改）
	_, err = wt.Remove(file3Path)
	require.NoError(t, err)

	commit4Hash, err := wt.Commit("Commit 4: remove file3 (revert commit3)", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	commit4HashStr := commit4Hash.String()
	t.Logf("Commit 4 hash: %s", commit4HashStr)

	// 测试：从 commit1 到 commit4，commit1 和 commit4 之间可能没有直接变更
	// 但应该返回 commit4 的完整文件系统
	t.Run("FromCommitRange-commit1-to-commit4", func(t *testing.T) {
		fs, err := FromCommitRange(tmpDir, commit1HashStr, commit4HashStr)
		require.NoError(t, err, "should return commit4's full file system even if no direct changes")

		// 应该包含 commit4 的所有文件
		raw, err := fs.ReadFile(file1Path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "Modified content of file1")

		raw, err = fs.ReadFile(file2Path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "Content of file2")

		// file3 应该不存在（因为 commit4 删除了它）
		exists, _ := fs.Exists(file3Path)
		assert.False(t, exists, "file3 should not exist in commit4")
	})

	// 测试：从 commit3 到 commit4，commit3 添加了 file3，commit4 删除了 file3
	// 这种情况下，start 和 end 之间没有净变更，但应该返回 commit4 的完整文件系统
	t.Run("FromCommitRange-commit3-to-commit4", func(t *testing.T) {
		fs, err := FromCommitRange(tmpDir, commit3HashStr, commit4HashStr)
		require.NoError(t, err, "should return commit4's full file system even if file3 was added then removed")

		// 应该包含 commit4 的所有文件
		raw, err := fs.ReadFile(file1Path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "Modified content of file1")

		raw, err = fs.ReadFile(file2Path)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "Content of file2")

		// file3 应该不存在
		exists, _ := fs.Exists(file3Path)
		assert.False(t, exists)
	})
}
