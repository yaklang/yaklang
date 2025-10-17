package yakgit

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yaklang/yaklang/common/utils"
)

func GetAllBranches(repos string) ([]string, error) {
	rep, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return nil, err
	}
	iter, err := rep.References()
	if err != nil {
		return nil, utils.Errorf("fetch refs iters failed: %s", err)
	}
	var i []string
	iter.ForEach(func(ref *plumbing.Reference) error {
		n := ref.Name()
		if n.IsTag() || n.IsBranch() {
			return nil
		}
		shortName := n.Short()
		i = append(i, shortName)
		return nil
	})
	return i, nil
}
