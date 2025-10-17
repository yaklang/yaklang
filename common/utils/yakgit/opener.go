package yakgit

import (
	"github.com/go-git/go-git/v5"
	"github.com/jellydator/ttlcache/v3"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"path/filepath"
	"time"
)

var reposCache = ttlcache.New[string, *git.Repository]()

func GitOpenRepositoryWithCache(localPath string) (*git.Repository, error) {
	if stat, err := os.Stat(localPath); err != nil {
		return nil, utils.Errorf("os.Stat failed: %s", err)
	} else if !stat.IsDir() {
		return nil, utils.Errorf("%s is not a directory", localPath)
	}

	if !filepath.IsAbs(localPath) {
		var err error
		localPath, err = filepath.Abs(localPath)
		if err != nil {
			return nil, err
		}
	}

	result := reposCache.Get(localPath)
	if !utils.IsNil(result) {
		if ret := result.Value(); !utils.IsNil(ret) {
			return ret, nil
		}
	}

	log.Infof("missed cache for %s, opening repository...", localPath)
	start := time.Now()
	repos, err := git.PlainOpen(localPath)
	cost := time.Now().Sub(start)
	log.Infof("GitOpenRepositoryWithCache took %s to open repository at %s", cost, localPath)
	if err != nil {
		return nil, utils.Errorf("GitOpenRepositoryWithCache failed: %s", err)
	}
	defer func() {
		reposCache.Set(localPath, repos, time.Minute)
	}()
	return repos, nil
}
