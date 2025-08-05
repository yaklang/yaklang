package yakgit

import (
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Pull 用于指定一个本地仓库，并从其远程仓库中获取代码并合并到本地仓库中，这种行为称之为拉取(pull)，它还可以接收零个到多个选项函数，用于影响拉取行为
// Example:
// ```
// git.Pull("C:/Users/xxx/Desktop/yaklang", git.verify(false), git.remote("origin"))
// ```
func pull(localPath string, opts ...Option) error {
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

	err = tree.PullContext(c.Context, &git.PullOptions{
		RemoteName:        c.Remote,
		Depth:             c.Depth,
		Auth:              c.Auth,
		RecurseSubmodules: c.ToRecursiveSubmodule(),
		Progress:          os.Stdout,
		Force:             c.Force,
		InsecureSkipTLS:   !c.VerifyTLS,
	})
	if err != nil {
		return utils.Errorf("git fetch failed: %s", err)
	}
	ref, err := repos.Head()
	if err != nil {
		log.Errorf("git fetch head failed: %s", err)
	}
	log.Infof("git pull success: %s", ref.String())
	return nil
}
