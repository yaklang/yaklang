package yakgit

import (
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/yaklang/yaklang/common/utils"
)

// Fetch 用于指定一个本地仓库，并从其远程仓库中获取代码，它还可以接收零个到多个选项函数，用于影响获取行为
// Example:
// ```
// git.Fetch("C:/Users/xxx/Desktop/yaklang", git.verify(false), git.remote("origin"), git.fetchAllTags(true))
// ```
func fetch(localPath string, opts ...Option) error {
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

	var tag git.TagMode
	if c.NoFetchTags {
		tag = git.NoTags
	} else if c.FetchAllTags {
		tag = git.AllTags
	}
	err = repos.FetchContext(c.Context, &git.FetchOptions{
		RemoteName:      c.Remote,
		Depth:           c.Depth,
		Auth:            c.Auth,
		Progress:        os.Stdout,
		Tags:            tag,
		Force:           c.Force,
		InsecureSkipTLS: !c.VerifyTLS,
	})
	if err != nil {
		return utils.Errorf("git fetch failed: %s", err)
	}
	return nil
}
