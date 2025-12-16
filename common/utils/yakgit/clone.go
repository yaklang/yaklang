package yakgit

import (
	"context"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitClient "github.com/go-git/go-git/v5/plumbing/transport/client"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	gossh "golang.org/x/crypto/ssh"
)

func init() {
	tr := gitHttp.NewClient(netx.NewDefaultHTTPClient())
	gitClient.InstallProtocol("https", tr)
	gitClient.InstallProtocol("http", tr)
}

// SetProxy 是一个辅助函数，用于指定其他 Git 操作（例如Clone）的代理
// Example:
// ```
// git.SetProxy("http://127.0.0.1:1080")
// ```
func SetProxy(proxies ...string) {
	tr := gitHttp.NewClient(netx.NewDefaultHTTPClient(proxies...))
	gitClient.InstallProtocol("https", tr)
	gitClient.InstallProtocol("http", tr)
}

// Clone 用于克隆远程仓库并存储到本地路径中，它还可以接收零个到多个选项函数，用于影响克隆行为
// Example:
// ```
// git.Clone("https://github.com/yaklang/yaklang", "C:/Users/xxx/Desktop/yaklang", git.recursive(true), git.verify(false))
// ```
func Clone(u string, localPath string, opt ...Option) error {
	c := &config{}
	for _, o := range opt {
		if err := o(c); err != nil {
			return err
		}
	}
	if c.Context == nil {
		c.Context, c.Cancel = context.WithCancel(context.Background())
	}

	if c.InsecureIgnoreHostKey && c.Auth != nil {
		if sshAuth, ok := c.Auth.(*ssh.PublicKeys); ok {
			sshAuth.HostKeyCallback = gossh.InsecureIgnoreHostKey()
		}
	}

	respos, err := git.PlainCloneContext(c.Context, localPath, false, &git.CloneOptions{
		URL:               u,
		Auth:              c.Auth,
		Depth:             c.Depth,
		RecurseSubmodules: c.ToRecursiveSubmodule(),
		InsecureSkipTLS:   !c.VerifyTLS,
		Progress:          os.Stdout,
		ProxyOptions:      c.Proxy,
		ReferenceName:     plumbing.ReferenceName(c.Branch),
	})
	if err != nil {
		return utils.Wrapf(err, "git clone: %v to %v failed", u, localPath)
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
