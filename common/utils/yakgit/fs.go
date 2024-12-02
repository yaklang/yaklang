package yakgit

import (
	"os"

	"github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func fetchRespos(res *git.Repository, commitHash string) (*filesys.VirtualFS, error) {
	commit, err := res.CommitObject(plumbing.NewHash(commitHash))
	if err != nil {
		return nil, err
	}

	// 获取父提交
	parentCommits, err := commit.Parents().Next()
	if err != nil {
		return nil, err
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

func FromCommit(repos string, commitHash string) (filesys_interface.FileSystem, error) {
	res, err := git.PlainOpen(repos)
	if err != nil {
		return nil, err
	}
	return fetchRespos(res, commitHash)
}

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

func FromCommitRange(repos string, start, end string) (*filesys.VirtualFS, error) {
	res, err := git.PlainOpen(repos)
	if err != nil {
		return nil, err
	}

	startCommit, err := res.CommitObject(plumbing.NewHash(start))
	if err != nil {
		return nil, utils.Wrap(err, "get start commit")
	}

	endCommit, err := res.CommitObject(plumbing.NewHash(end))
	if err != nil {
		return nil, utils.Wrap(err, "get end commit")
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
	changes, err := endTree.Diff(startTree)
	if err != nil {
		return nil, utils.Wrap(err, "calculate diff")
	}

	// 创建虚拟文件系统
	fs := filesys.NewVirtualFs()

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
