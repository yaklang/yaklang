package yakgit

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// IterateCommit 用于指定一个本地仓库，遍历其所有的提交记录(commit)，并对过滤后的每个提交记录执行指定的操作，它还可以接收零个到多个选项函数，用于配置回调函数
// Example:
// ```
// // 遍历提交记录，过滤名字中包含ci的引用记录，过滤作者名字为xxx的提交记录，打印剩余的每个提交记录
// git.IterateCommit("D:/coding/golang/src/yaklang",
// git.filterReference((ref) => {return !ref.Name().Contains("ci")}),
// git.filterCommit((c) => { return c.Author.Name != "xxx" }),
// git.handleCommit((c) => { println(c.String()) }))
// ```
func EveryCommit(localRepos string, opt ...Option) error {
	c := NewConfig()
	for _, i := range opt {
		err := i(c)
		if err != nil {
			return err
		}
	}
	r, err := GitOpenRepositoryWithCache(localRepos)
	if err != nil {
		return utils.Errorf("open repository failed: %s", err)
	}
	refs, err := r.References()
	if err != nil {
		return utils.Errorf("get references failed: %s", err)
	}
	return refs.ForEach(func(ref *plumbing.Reference) error {
		if c.FilterGitReference != nil && !c.FilterGitReference(ref) {
			return nil
		}
		if c.HandleGitReference != nil {
			err := c.HandleGitReference(ref)
			if err != nil {
				return err
			}
		}
		commitIter, err := r.Log(&git.LogOptions{
			From: ref.Hash(),
		})
		if err != nil {
			log.Errorf("fetch %v's logs failed: %s", ref.Hash(), err)
			return nil
		}
		commitIter.ForEach(func(commit *object.Commit) error {
			if c.FilterGitCommit != nil && !c.FilterGitCommit(commit) {
				return nil
			}

			if c.HandleGitCommit != nil {
				err := c.HandleGitCommit(commit)
				if err != nil {
					return err
				}
			}

			return nil
		})
		return nil
	})
}
