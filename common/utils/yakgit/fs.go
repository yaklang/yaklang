package yakgit

import (
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/object"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func fetchRespos(res *git.Repository, commitHash string) (*filesys.VirtualFS, error) {
	commit, err := GetCommitHashEx(res, commitHash)
	if err != nil {
		return nil, err
	}

	// 获取父提交
	parentCommits, err := commit.Parents().Next()
	if err != nil {
		// no parent commit
		// orphan commit
		// just return the commit's tree
		tree, err := commit.Tree()
		if err != nil {
			return nil, err
		}
		vfs := filesys.NewVirtualFs()
		files := tree.Files()
		count := 0
		files.ForEach(func(file *object.File) error {
			raw, err := file.Contents()
			if err != nil {
				log.Warn(utils.Wrapf(err, "read file %s failed", file.Name))
				return nil
			}
			vfs.AddFile(file.Name, raw)
			count++
			return nil
		})
		if count <= 0 {
			return nil, utils.Error("no file changed")
		}
		return vfs, nil
	}

	// 获取父提交的树
	parentTree, err := parentCommits.Tree()
	if err != nil {
		return nil, err
	}

	// 获取当前提交的树
	currentTree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	// 比较两个树的差异
	changes, err := parentTree.Diff(currentTree)
	if err != nil {
		return nil, err
	}

	vfs := filesys.NewVirtualFs()

	// 遍历差异
	count := 0
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			continue
		}

		switch action {
		case merkletrie.Modify:
			fallthrough
		case merkletrie.Insert:
			dst := change.To
			f, err := change.To.Tree.TreeEntryFile(&dst.TreeEntry)
			if err != nil {
				log.Warn(utils.Wrapf(err, "get file %s failed", dst.Name))
				continue
			}
			raw, err := f.Contents()
			if err != nil {
				log.Warn(utils.Wrapf(err, "read file %s failed", dst.Name))
				continue
			}
			count++
			vfs.AddFile(dst.Name, raw)
		}
	}

	if count <= 0 {
		return nil, utils.Error("no file changed")
	}
	return vfs, nil
}

// FileSystemFromCommit 从指定的commit中获取文件系统
//
// Example:
// ```
// fs := git.FileSystemFromCommit("path/to/repo", "2871a988b2ed7ec10a1fd45eca248a96a99a8560")~
// fs, err := git.FileSystemFromCommit("path/to/repo", "2871a988b2ed7ec10a1fd45eca248a96a99a8560")
// ```
func FromCommit(repos string, commitHash string) (filesys_interface.FileSystem, error) {
	res, err := git.PlainOpen(repos)
	if err != nil {
		return nil, err
	}
	return fetchRespos(res, commitHash)
}

// FileSystemFromCommits 从多个commit中获取文件系统
//
// Example:
// ```
// fs := git.FileSystemFromCommits("path/to/repo", "2871a988b2ed7ec10a1fd45eca248a96a99a8560", "54165a396a219d085980dca623ae1ff6582033ad")~
// fs, err := git.FileSystemFromCommits("path/to/repo", "54165a396a219d085980dca623ae1ff6582033ad", "2871a988b2ed7ec10a1fd45eca248a96a99a8560")
// ```
func FromCommits(repos string, commitHashes ...string) (filesys_interface.FileSystem, error) {
	res, err := git.PlainOpen(repos)
	if err != nil {
		return nil, err
	}

	if len(commitHashes) <= 0 {
		return nil, utils.Error("no commit hash")
	}

	if len(commitHashes) == 1 {
		return fetchRespos(res, commitHashes[0])
	}

	base, err := fetchRespos(res, commitHashes[0])
	if err != nil {
		return nil, err
	}

	for _, commitHash := range commitHashes[1:] {
		fs, err := fetchRespos(res, commitHash)
		if err != nil {
			return nil, err
		}
		filesys.SimpleRecursive(filesys.WithFileSystem(fs), filesys.WithFileStat(func(pathname string, info os.FileInfo) error {
			if b, _ := base.Exists(pathname); b {
				err := base.RemoveFileOrDir(pathname)
				if err != nil {
					log.Warn(err)
				}
			}
			raw, err := fs.ReadFile(pathname)
			if err != nil {
				return err
			}
			base.AddFile(pathname, string(raw))
			return nil
		}))
	}

	return base, nil
}

// FileSystemFromCommitRange 从commit范围中获取文件系统
//
// Example:
// ```
// fs := git.FileSystemFromCommitRange("path/to/repo", "2871a988b2ed7ec10a1fd45eca248a96a99a8560", "54165a396a219d085980dca623ae1ff6582033ad")~
// ```
func FromCommitRange(repos string, start, end string) (*filesys.VirtualFS, error) {
	res, err := git.PlainOpen(repos)
	if err != nil {
		return nil, err
	}

	startCommit, err := GetCommitHashEx(res, start)
	if err != nil {
		return nil, utils.Wrap(err, "get start commit")
	}

	endCommit, err := GetCommitHashEx(res, end)
	if err != nil {
		return nil, utils.Wrap(err, "get end commit")
	}

	basevfs, err := fetchRespos(res, start)
	if err != nil {
		return nil, err
	}

	// 获取两个commit的tree
	startTree, err := startCommit.Tree()
	if err != nil {
		return nil, utils.Wrap(err, "get start tree")
	}
	endTree, err := endCommit.Tree()
	if err != nil {
		return nil, utils.Wrap(err, "get end tree")
	}

	// 计算diff
	changes, err := startTree.Diff(endTree)
	if err != nil {
		return nil, utils.Wrap(err, "calculate diff")
	}

	// 创建虚拟文件系统
	fs := basevfs

	count := 0
	// 遍历所有变更
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			return nil, utils.Wrap(err, "get change action")
		}

		switch action {
		case merkletrie.Insert, merkletrie.Modify:
			// 对于新增和修改的文件,从新commit中读取内容
			dst := change.To
			f, err := change.To.Tree.TreeEntryFile(&dst.TreeEntry)
			if err != nil {
				log.Warnf("read file %s content failed: %s", dst.Name, err)
				continue
			}
			content, err := f.Contents()
			if err != nil {
				log.Warnf("read file %s content failed: %s", dst.Name, err)
				continue
			}

			if a, _ := fs.Exists(dst.Name); a {
				err := fs.RemoveFileOrDir(dst.Name)
				if err != nil {
					log.Warn(err)
				}
			}

			count++
			fs.AddFile(dst.Name, content)
		case merkletrie.Delete:
			// 对于删除的文件,不需要特殊处理,因为新的fs中本来就没有
			continue
		}
	}

	if count <= 0 {
		return nil, utils.Error("no file changed")
	}

	return fs, nil
}

func GetHeadCommitRange(repos string) (*object.Commit, *object.Commit, error) {
	repo, err := git.PlainOpen(repos)
	if err != nil {
		return nil, nil, err
	}
	currentRef, err := repo.Head()
	if err != nil {
		return nil, nil, err
	}
	currentCommit, err := repo.CommitObject(currentRef.Hash())
	if err != nil {
		return nil, nil, err
	}
	upstreamRef, err := repo.Reference("refs/heads/main", true)
	if err != nil {
		return nil, nil, err
	}
	upstreamCommit, err := repo.CommitObject(upstreamRef.Hash())
	if err != nil {
		return nil, nil, err
	}

	base, err := currentCommit.MergeBase(upstreamCommit)
	if err != nil {
		return nil, nil, err
	}

	if len(base) <= 0 {
		return nil, nil, utils.Error("no merge base")
	}
	return base[0], currentCommit, nil
}

func GetHeadHash(repos string) string {
	res, err := git.PlainOpen(repos)
	if err != nil {
		return ""
	}

	ref, err := res.Head()
	if err != nil {
		return ""
	}

	return ref.Hash().String()
}

func ShortHashToFullHash(repo *git.Repository, hash string) (string, error) {
	// 获取对象数据库
	objDB, err := repo.Objects()
	if err != nil {
		return "", fmt.Errorf("failed to get object database: %w", err)
	}

	var foundHash string
	var matchCount int

	// 遍历所有对象
	err = objDB.ForEach(func(obj object.Object) error {
		fullHash := obj.ID().String()
		if strings.HasPrefix(fullHash, hash) {
			foundHash = fullHash
			matchCount++
			if matchCount > 1 {
				return fmt.Errorf("ambiguous hash prefix: %s matches multiple objects", hash)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if matchCount == 0 {
		return "", fmt.Errorf("no matching hash found for %s", hash)
	}

	return foundHash, nil
}

// GetCommitHashEx 获取完整的commit hash
// 如果hash不是完整的commit hash,则尝试查找匹配的commit hash
func GetCommitHashEx(repo *git.Repository, hash string) (*object.Commit, error) {
	if len(hash) < 7 {
		return nil, fmt.Errorf("invalid commit hash: %s (too short)", hash)
	}
	if len(hash) > 40 {
		return nil, fmt.Errorf("invalid commit hash: %s (too long)", hash)
	}
	commit, err := repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		fullHash, err := ShortHashToFullHash(repo, hash)
		if err != nil {
			return nil, err
		}
		commit, err = repo.CommitObject(plumbing.NewHash(fullHash))
		if err != nil {
			return nil, err
		}
	}
	return commit, nil
}
