package yakgit

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Checkout 用于指定一个本地仓库，切换其分支或者恢复工作树的文件，这种行为称之为检出(checkout)，它还可以接收零个到多个选项函数，用于影响检出行为
// Example:
// ```
// git.Checkout("C:/Users/xxx/Desktop/yaklang", "feat/new-branch", git.checkoutCreate(true)) // 创建新分支
// git.Checkout("C:/Users/xxx/Desktop/yaklang", "old-branch", git.checkoutForce(true)) // 强制切换
// ```
func checkout(localPath string, ref string, opts ...Option) error {
	c := &config{Remote: "origin"}
	for _, o := range opts {
		if err := o(c); err != nil {
			return err
		}
	}

	repos, err := GitOpenRepositoryWithCache(localPath)
	if err != nil {
		return utils.Errorf("GitOpenRepositoryWithCache failed: %s", err)
	}

	tree, err := repos.Worktree()
	if err != nil {
		return utils.Errorf("git.Worktree failed: %s", err)
	}

	checkoutOpt := &git.CheckoutOptions{
		Create: c.CheckoutCreate,
		Force:  c.CheckoutForce || c.Force,
		Keep:   c.CheckoutKeep,
	}

	if ref != "" {
		branch, err := repos.Branch(ref)
		if err != nil {
			log.Infof("git branch %s not found, try tag", ref)
			tag, err := repos.Tag(ref)
			if err != nil {
				log.Infof("git tag %s not found, try commit", ref)
				commit, err := repos.CommitObject(plumbing.NewHash(ref))
				if err != nil {
					return utils.Errorf("git commit %s not found", ref)
				}
				checkoutOpt.Hash = commit.Hash
			} else {
				checkoutOpt.Hash = tag.Hash()
			}
		} else {
			checkoutOpt.Branch = branch.Merge
		}
	}

	err = tree.Checkout(checkoutOpt)
	if err != nil {
		return utils.Errorf("git fetch failed: %s", err)
	}
	if ref == "" {
		log.Info("git checkout success")
	} else {
		log.Infof("git checkout success: %s", ref)
	}
	return nil
}
