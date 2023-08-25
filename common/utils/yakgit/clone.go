package yakgit

import (
	"context"
	"github.com/go-git/go-git/v5"
	gitClient "github.com/go-git/go-git/v5/plumbing/transport/client"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"os"
)

func init() {
	tr := gitHttp.NewClient(netx.NewDefaultHTTPClient())
	gitClient.InstallProtocol("https", tr)
	gitClient.InstallProtocol("http", tr)
}

func SetProxy(proxies ...string) {
	tr := gitHttp.NewClient(netx.NewDefaultHTTPClient(proxies...))
	gitClient.InstallProtocol("https", tr)
	gitClient.InstallProtocol("http", tr)
}

func clone(u string, localPath string, opt ...Option) error {
	c := &config{}
	for _, o := range opt {
		if err := o(c); err != nil {
			return err
		}
	}
	if c.Context == nil {
		c.Context, c.Cancel = context.WithCancel(context.Background())
	}

	respos, err := git.PlainCloneContext(c.Context, localPath, false, &git.CloneOptions{
		URL:               u,
		Auth:              c.ToAuth(),
		Depth:             c.Depth,
		RecurseSubmodules: c.ToRecursiveSubmodule(),
		InsecureSkipTLS:   !c.VerifyTLS,
		Progress:          os.Stdout,
	})
	if err != nil {
		return utils.Errorf("git clone: %v to %v failed: %s", u, localPath)
	}
	_ = respos
	h, err := respos.Head()
	if h != nil {
		log.Infof("git clone: %v to %v success: %v", u, localPath, h.String())
	} else {
		log.Infof("git clone: %v to %v success", u, localPath)
	}
	return nil
}
