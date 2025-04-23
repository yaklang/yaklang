package yakgit

import (
	"bytes"
	_ "embed"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-rod/rod/lib/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
