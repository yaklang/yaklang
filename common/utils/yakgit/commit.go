package yakgit

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func EveryCommit(localRepos string, opt ...Option) error {
	c := NewConfig()
	for _, i := range opt {
		err := i(c)
		if err != nil {
			return err
		}
	}
	r, err := git.PlainOpen(localRepos)
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

			// handle files?
			log.Infof("found: %v", commit.String())
			return nil
		})
		return nil
	})
}
