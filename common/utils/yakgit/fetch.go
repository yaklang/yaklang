package yakgit

import (
	"github.com/go-git/go-git/v5"
	"github.com/yaklang/yaklang/common/utils"
	"os"
)

func fetch(localPath string, opts ...Option) error {
	c := &config{Remote: "origin"}
	for _, o := range opts {
		if err := o(c); err != nil {
			return err
		}
	}

	repos, err := git.PlainOpen(localPath)
	if err != nil {
		return utils.Errorf("git.PlainOpen failed: %s", err)
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
		Auth:            c.ToAuth(),
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
