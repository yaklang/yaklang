package yakgit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"

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
	res, err := GitOpenRepositoryWithCache(repos)
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
	res, err := GitOpenRepositoryWithCache(repos)
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
	log.Infof("start to open plain git repos: %s", repos)
	res, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return nil, err
	}

	log.Infof("start to fetch start & end rev-parse")
	start, err = RevParse(repos, start)
	if err != nil {
		return nil, err
	}
	end, err = RevParse(repos, end)
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

	log.Infof("start to fetch base virtual-fs from start: %v", start)
	basevfs, err := fetchRespos(res, start)
	if err != nil {
		return nil, err
	}

	log.Infof(
		"start to fetch start and end file tree, from: %v to %v",
		utils.ShrinkString(start, 16), utils.ShrinkString(end, 16),
	)
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
	log.Infof("start to calc diff-tree from %v to %v", utils.ShrinkString(start, 16), utils.ShrinkString(end, 16))
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

func GetHeadHash(repos string) string {
	res, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return ""
	}

	ref, err := res.Head()
	if err != nil {
		return ""
	}

	return ref.Hash().String()
}

func GetHeadBranch(repos string) string {
	res, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return ""
	}

	ref, err := res.Head()
	if err != nil {
		return ""
	}

	refname := ref.Name()
	if refname.IsBranch() {
		return refname.String()
	}
	if refname.IsTag() {
		return refname.String()
	}
	if refname.IsRemote() {
		return refname.String()
	}
	return ""
}

func findChildren(res *git.Repository, commitHash plumbing.Hash) ([]*object.Commit, error) {
	var children []*object.Commit
	iter, err := res.CommitObjects()
	if err != nil {
		return nil, err
	}
	err = iter.ForEach(func(commit *object.Commit) error {
		// 检查是否是目标 commit 的子 commit
		for _, parentHash := range commit.ParentHashes {
			if parentHash == commitHash {
				children = append(children, commit)
				break //  找到了一个父 commit 匹配，跳出循环
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return children, nil
}

func GetBranchRange(repos string, branchName string) (start, end string, err error) {
	res, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		err = utils.Errorf("open repos: %v failed: %v", repos, err)
		return
	}

	branchRef, err := res.Reference(plumbing.ReferenceName(branchName), true)
	if err != nil {
		err = utils.Errorf("open reference %v failed: %v", branchName, err)
		return
	}

	commit, err := res.CommitObject(branchRef.Hash())
	if err != nil {
		err = utils.Errorf("get branch end commit failed: %v", err)
		return
	}
	end = commit.Hash.String()

	var branchStart plumbing.Hash
	_ = commit.Parents().ForEach(func(p *object.Commit) error {
		children, err := findChildren(res, p.Hash)
		if err != nil {
			return err
		}
		branchStart = p.Hash
		if len(children) == 2 {
			return utils.Error("stop it")
		} else {
			return nil
		}
	})
	if utils.IsNil(branchStart) || branchStart.IsZero() {
		return "", "", utils.Errorf("get branch start commit failed: %v", err)
	}
	start = branchStart.String()
	return start, end, nil
}

func GetParentCommitHash(repos string, commit string) (string, error) {
	res, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return "", utils.Errorf("open repos: %v failed: %v", repos, err)
	}
	long, err := RevParse(repos, commit)
	if err != nil {
		return "", err
	}

	hash := plumbing.NewHash(long)
	commitObj, err := res.CommitObject(hash)
	if err != nil {
		return "", utils.Errorf("get commit object failed: %v", err)
	}

	parents := commitObj.ParentHashes
	if len(parents) == 0 {
		return "", utils.Errorf("commit has no parent")
	}

	return parents[0].String(), nil
}

func Glance(repos string) string {
	res, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return ""
	}

	ref, err := res.Head()
	if err != nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	hashStr := ref.Hash().String()
	buf.WriteString(fmt.Sprintf("hash: %v\n", hashStr))
	buf.WriteString(fmt.Sprintf("type: %v\n", ref.Type()))
	buf.WriteString(fmt.Sprintf("refname(branch/tag): %v\n", ref.Name()))
	start, end, err := GetBranchRange(repos, ref.Name().String())
	if err == nil {
		buf.WriteString(fmt.Sprintf("branch_start: %v\n", start))
		buf.WriteString(fmt.Sprintf("branch_end: %v\n", end))
		endCommit, _ := res.CommitObject(plumbing.NewHash(end))
		if !utils.IsNil(endCommit) {
			count := 1
			_ = endCommit.Parents().ForEach(func(p *object.Commit) error {
				if p.Hash.String() == start {
					count++
					return utils.Error("stop it")
				}
				return nil
			})
			if count > 1 {
				buf.WriteString(fmt.Sprintf("commits total in this branch: %v\n", count))
			}
		}
	}
	return buf.String()
}

func revParseOptimized(repo *git.Repository, rev string) (string, error) {
	// 核心步骤：直接使用 go-git 内置的、经过优化的 revision 解析器。
	// 这个函数能高效处理完整哈希、短哈希、引用名(HEAD, main, v1.0)、相对引用(HEAD~2)等。
	// 这是最快、最正确的路径。
	hash, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err == nil {
		// 成功解析，直接返回完整哈希字符串。
		return hash.String(), nil
	}

	// 如果 ResolveRevision 返回错误，我们需要判断错误类型。
	// 常见的错误是 ErrReferenceNotFound 或 ErrObjectNotFound。
	// 这意味着 rev 不是一个已知的引用或对象。
	// 在这种情况下，我们可以选择性地实现一些自定义的、更宽松的模糊匹配逻辑。
	// 注意：原始代码中的许多模糊匹配逻辑 `ResolveRevision` 已经覆盖了。
	// 这里的备用逻辑仅用于处理 `ResolveRevision` 无法处理的边缘情况或自定义别名。
	if errors.Is(err, plumbing.ErrReferenceNotFound) || errors.Is(err, plumbing.ErrObjectNotFound) {
		// （可选）如果需要比 ResolveRevision 更宽松的模糊匹配，可以在这里实现。
		// 例如，不带远程前缀的远程分支名等。
		// 但在大多数标准场景下, ResolveRevision 已经足够。
		// 此处保留原始代码中的部分遍历逻辑作为“最后手段”。
		log.Printf("ResolveRevision failed for '%s': %v. Falling back to fuzzy matching...", rev, err)
		return fuzzyMatchReference(repo, rev)
	}

	// 对于其他类型的错误（如仓库损坏等），直接返回。
	return "", fmt.Errorf("failed to parse revision '%s': %w", rev, err)
}

// fuzzyMatchReference 是一种成本较高的备用方案，用于在标准解析失败时进行模糊匹配。
func fuzzyMatchReference(repo *git.Repository, rev string) (string, error) {
	refs, err := repo.References()
	if err != nil {
		return "", fmt.Errorf("failed to get references for fuzzy matching: %w", err)
	}
	defer refs.Close()

	var matchedRef *plumbing.Reference
	errStopIteration := errors.New("reference found")

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		// 检查短名称是否匹配，例如 "main"
		if ref.Name().Short() == rev {
			matchedRef = ref
			return errStopIteration
		}
		// 可以添加更多自定义的模糊匹配规则...
		return nil
	})

	if matchedRef != nil {
		return matchedRef.Hash().String(), nil
	}

	return "", fmt.Errorf("cannot parse revision '%s' with any method", rev)
}

func revParse(repo *git.Repository, rev string) (string, error) {
	long, _ := ShortHashToFullHash(repo, rev)
	if len(long) > 0 {
		return long, nil
	}
	if rev == "HEAD" {
		head, err := repo.Head()
		if err != nil {
			return "", utils.Errorf("get head failed: %v", err)
		}
		return head.Hash().String(), nil
	} else if strings.HasPrefix(rev, "HEAD") {
		if rev == "HEAD^" {
			head, err := repo.Head()
			if err != nil {
				return "", utils.Errorf("get head failed: %v", err)
			}
			commit, err := repo.CommitObject(head.Hash())
			if err != nil {
				return "", err
			}
			parents := commit.ParentHashes
			if len(parents) == 0 {
				return "", utils.Errorf("HEAD commit has no parent")
			}
			return parents[0].String(), nil
		} else if strings.HasPrefix(rev, "HEAD~") {
			// 处理 HEAD~n 格式
			n := 0
			_, err := fmt.Sscanf(rev, "HEAD~%d", &n)
			if err != nil || n <= 0 {
				return "", utils.Errorf("invalid HEAD~n format: %s", rev)
			}

			// 获取 HEAD
			head, err := repo.Head()
			if err != nil {
				return "", utils.Errorf("get head failed: %v", err)
			}

			// 获取当前 commit
			commit, err := repo.CommitObject(head.Hash())
			if err != nil {
				return "", err
			}

			// 沿着第一个父提交向上遍历 n 次
			for i := 0; i < n; i++ {
				parents := commit.ParentHashes
				if len(parents) == 0 {
					return "", utils.Errorf("reached root commit before finding HEAD~%d", n)
				}

				commit, err = repo.CommitObject(parents[0])
				if err != nil {
					return "", utils.Errorf("failed to get parent commit: %v", err)
				}
			}

			return commit.Hash.String(), nil
		}
	}

	// 处理完整引用名
	// 尝试将输入解析为引用名
	referenceNames := []plumbing.ReferenceName{
		// 尝试直接使用给定的引用名
		plumbing.ReferenceName(rev),
		// 尝试解析为分支
		plumbing.NewBranchReferenceName(rev),
		// 尝试解析为标签
		plumbing.NewTagReferenceName(rev),
		// 尝试解析为远程分支
		plumbing.NewRemoteReferenceName("origin", rev),
	}

	// 遍历所有可能的引用名称并尝试解析
	for _, refName := range referenceNames {
		ref, err := repo.Reference(refName, true)
		if err == nil {
			return ref.Hash().String(), nil
		}
	}

	// 尝试模糊匹配分支和标签名
	refs, err := repo.References()
	if err != nil {
		return "", utils.Errorf("failed to get references: %v", err)
	}

	var matchedRef *plumbing.Reference

	// 定义异常终止的错误
	var errStopIteration = errors.New("reference_found")

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		shortName := ref.Name().Short()

		// 如果引用名完全匹配或短名称完全匹配
		if name == rev || shortName == rev {
			matchedRef = ref
			return errStopIteration
		}

		// 检查分支名（不带refs/heads/前缀）
		if strings.HasPrefix(name, "refs/heads/") && strings.TrimPrefix(name, "refs/heads/") == rev {
			matchedRef = ref
			return errStopIteration
		}

		// 检查标签名（不带refs/tags/前缀）
		if strings.HasPrefix(name, "refs/tags/") && strings.TrimPrefix(name, "refs/tags/") == rev {
			matchedRef = ref
			return errStopIteration
		}

		return nil
	})

	// 如果找到了匹配的引用，则返回其哈希
	if matchedRef != nil {
		return matchedRef.Hash().String(), nil
	}

	return "", utils.Errorf("cannot parse revision: %s", rev)
}

func RevParse(repos string, rev string) (string, error) {
	repo, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return "", utils.Errorf("open: %v failed: %v", repos, err)
	}
	return revParseOptimized(repo, rev)
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
	hash, err := revParseOptimized(repo, hash)
	if err != nil {
		return nil, utils.Errorf("rev-parse err: %v", err)
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
