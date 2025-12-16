package yakgit

import (
	"context"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type config struct {
	Context context.Context
	Cancel  context.CancelFunc

	VerifyTLS          bool
	Auth               transport.AuthMethod
	Depth              int
	RecursiveSubmodule bool

	// SSH 配置
	InsecureIgnoreHostKey bool // 是否跳过 SSH 主机密钥验证

	// remote operation
	Remote string
	Branch string

	Proxy transport.ProxyOptions

	// Force
	Force        bool
	NoFetchTags  bool
	FetchAllTags bool

	CheckoutCreate bool
	CheckoutForce  bool
	CheckoutKeep   bool

	// GitHack
	Threads           int
	UseLocalGitBinary bool
	HTTPOptions       []poc.PocConfigOption

	// handler
	HandleGitReference func(r *plumbing.Reference) error
	FilterGitReference func(r *plumbing.Reference) bool
	HandleGitCommit    func(r *object.Commit) error
	FilterGitCommit    func(r *object.Commit) bool
}

func NewConfig() *config {
	ctx, cancel := context.WithCancel(context.Background())
	return &config{
		Context:            ctx,
		Cancel:             cancel,
		VerifyTLS:          true,
		Depth:              1,
		RecursiveSubmodule: true,
		Remote:             "origin",
	}
}

func (c *config) ToRecursiveSubmodule() git.SubmoduleRescursivity {
	var recursiveSubmodule git.SubmoduleRescursivity
	if c.RecursiveSubmodule {
		recursiveSubmodule = git.SubmoduleRescursivity(10)
	} else {
		recursiveSubmodule = git.NoRecurseSubmodules
	}
	return recursiveSubmodule
}

type Option func(*config) error

// handleReference 是一个选项函数，它接收一个回调函数，这个函数有一个参数，其为引用记录结构体(reference)，每次遍历到过滤后的引用时，就会调用这个回调函数
// Example:
// ```
// // 遍历提交记录，过滤名字中包含ci的引用记录，打印剩余的每个引用记录
// git.IterateCommit("D:/coding/golang/src/yaklang",
// git.filterReference((ref) => {return !ref.Name().Contains("ci")}),
// git.handleReference((ref) => { println(ref.String()) }))
// ```
func WithHandleGitReference(f func(r *plumbing.Reference) error) Option {
	return func(c *config) error {
		c.HandleGitReference = f
		return nil
	}
}

// filterReference 是一个选项函数，它接收一个回调函数，这个函数有一个参数，其为引用记录结构体(reference)，每次遍历到引用时，就会调用这个回调函数，这个函数还有一个返回值，通过这个返回值来决定是否过滤掉这个引用
// Example:
// ```
// // 遍历提交记录，过滤名字中包含ci的引用记录，打印剩余的每个引用记录
// git.IterateCommit("D:/coding/golang/src/yaklang",
// git.filterReference((ref) => {return !ref.Name().Contains("ci")}),
// git.handleReference((ref) => { println(ref.String()) }))
// ```
func WithFilterGitReference(f func(r *plumbing.Reference) bool) Option {
	return func(c *config) error {
		c.FilterGitReference = f
		return nil
	}
}

// handleCommit 是一个选项函数，它接收一个回调函数，这个函数有一个参数，其为提交记录结构体(commit)，每次遍历到一个过滤后的提交记录时，就会调用这个回调函数
// Example:
// ```
// // 遍历提交记录，打印每个提交记录
// git.IterateCommit("D:/coding/golang/src/yaklang", git.handleCommit((c) => { println(c.String()) }))
// ```
func WithHandleGitCommit(f func(r *object.Commit) error) Option {
	return func(c *config) error {
		c.HandleGitCommit = f
		return nil
	}
}

// filterCommit 是一个选项函数，它接收一个回调函数，这个函数有一个参数，其为提交记录结构体(commit)，每次遍历到提交记录时，就会调用这个回调函数，这个函数还有一个返回值，通过这个返回值来决定是否过滤掉这个提交记录
// Example:
// ```
// // 遍历提交记录，过滤作者名字为xxx的提交记录，打印剩余的每个提交记录
// git.IterateCommit("D:/coding/golang/src/yaklang",
// git.filterCommit((c) => { return c.Author.Name != "xxx" }),
// git.handleCommit((c) => { println(c.String()) }))
// ```
func WithFilterGitCommit(f func(r *object.Commit) bool) Option {
	return func(c *config) error {
		c.FilterGitCommit = f
		return nil
	}
}

// verify 是一个选项函数，用于指定其他 Git 操作（例如Clone）时是否验证TLS证书
// Example:
// ```
// git.Clone("https://github.com/yaklang/yaklang", "C:/Users/xxx/Desktop/yaklang", git.recursive(true), git.verify(false))
// ```
func WithVerifyTLS(b bool) Option {
	return func(c *config) error {
		c.VerifyTLS = b
		return nil
	}
}

// noFetchTags 是一个选项函数，用于指定获取(fetch)操作时是否不拉取标签
// Example:
// ```
// git.Fetch("C:/Users/xxx/Desktop/yaklang", git.noFetchTags(true)) // 不拉取标签
// ```
func WithNoFetchTags(b bool) Option {
	return func(c *config) error {
		c.NoFetchTags = b
		return nil
	}
}

// fetchAllTags 是一个选项函数，用于指定获取(fetch)操作时是否拉取所有标签
// Example:
// ```
// git.Fetch("C:/Users/xxx/Desktop/yaklang", git.fetchAllTags(true)) // 拉取所有标签
// ```
func WithFetchAllTags(b bool) Option {
	return func(c *config) error {
		c.FetchAllTags = b
		return nil
	}
}

// fetchAllTags 是一个选项函数，用于指定检出(checkout)操作时是否创建新分支
// Example:
// ```
// git.Checkout("C:/Users/xxx/Desktop/yaklang", "feat/new-branch", git.checkoutCreate(true))
// ```
func WithCheckoutCreate(b bool) Option {
	return func(c *config) error {
		c.CheckoutCreate = b
		return nil
	}
}

// fetchAllTags 是一个选项函数，用于指定检出(checkout)操作时是否强制
// Example:
// ```
// git.Checkout("C:/Users/xxx/Desktop/yaklang", "old-branch", git.checkoutForce(true))
// ```
func WithCheckoutForce(b bool) Option {
	return func(c *config) error {
		c.CheckoutForce = b
		return nil
	}
}

// checkoutKeep 是一个选项函数，用于指定检出(checkout)操作时，本地更改（索引或工作树更改）是否被保留，如果保留，就可以将它们提交到目标分支，默认为false
// Example:
// ```
// git.Checkout("C:/Users/xxx/Desktop/yaklang", "old-branch", git.checkoutKeep(true))
// ```
func WithCheckoutKeep(b bool) Option {
	return func(c *config) error {
		c.CheckoutKeep = b
		return nil
	}
}

// depth 是一个选项函数，用于指定其他 Git 操作（例如Clone）时的最大深度，默认为1
// Example:
// ```
// git.Clone("https://github.com/yaklang/yaklang", "C:/Users/xxx/Desktop/yaklang", git.Depth(1))
// ```
func WithDepth(depth int) Option {
	return func(c *config) error {
		c.Depth = depth
		return nil
	}
}

// force 是一个选项函数，用于指定其他 Git 操作（例如Pull）时是否强制执行，默认为false
// Example:
// ```
// git.Pull("C:/Users/xxx/Desktop/yaklang", git.verify(false), git.force(true))
// ```
func WithForce(b bool) Option {
	return func(c *config) error {
		c.Force = b
		return nil
	}
}

// remote 是一个选项函数，用于指定其他 Git 操作（例如Pull）时的远程仓库名称，默认为origin
// Example:
// ```
// git.Pull("C:/Users/xxx/Desktop/yaklang", git.verify(false), git.remote("origin"))
// ```
func WithRemote(remote string) Option {
	return func(c *config) error {
		c.Remote = remote
		return nil
	}
}

func WithBranch(branch string) Option {
	return func(c *config) error {
		c.Branch = branch
		return nil
	}
}

// recursive 是一个选项函数，用于指定其他 Git 操作（例如Clone）时的是否递归克隆子模块，默认为false
// Example:
// ```
// git.Clone("https://github.com/yaklang/yaklang", "C:/Users/xxx/Desktop/yaklang", git.recursive(true))
// ```
func WithRecuriveSubmodule(b bool) Option {
	return func(c *config) error {
		c.RecursiveSubmodule = b
		return nil
	}
}

// context 是一个选项函数，用于指定其他 Git 操作（例如Clone）时的上下文
// Example:
// ```
// git.Clone("https://github.com/yaklang/yaklang", "C:/Users/xxx/Desktop/yaklang", git.context(context.New()))
// ```
func WithContext(ctx context.Context) Option {
	return func(c *config) error {
		c.Context, c.Cancel = context.WithCancel(ctx)
		return nil
	}
}

// auth 是一个选项函数，用于指定其他 Git 操作（例如Clone）时的认证用户名和密码
// Example:
// ```
// git.Clone("https://github.com/yaklang/yaklang", "C:/Users/xxx/Desktop/yaklang", git.auth("admin", "admin"))
// ```
func WithUsernamePassword(username, password string) Option {
	return func(c *config) error {
		if username != "" && password != "" {
			c.Auth = &gitHttp.BasicAuth{
				Username: username,
				Password: password,
			}
		}
		return nil
	}
}

func WithPrivateKey(userName, keyPath, password string) Option {
	return func(c *config) error {
		auth, err := ssh.NewPublicKeysFromFile(userName, keyPath, password)
		if err != nil {
			return err
		}
		c.Auth = auth
		return nil
	}
}

// WithPrivateKeyContent 使用私钥内容进行认证
// Example:
// ```
// keyContent := `-----BEGIN OPENSSH PRIVATE KEY-----
// b3BlbnNzaC1rZXktdjEAAAAABG5vbmU...
// -----END OPENSSH PRIVATE KEY-----`
// git.Clone("git@github.com:user/repo.git", "/tmp/repo",
//
//	git.WithPrivateKeyContent("git", keyContent, ""),
//	git.WithInsecureIgnoreHostKey(),  // 跳过主机密钥验证
//
// )
// ```
func WithPrivateKeyContent(userName, keyContent, password string) Option {
	return func(c *config) error {
		auth, err := ssh.NewPublicKeys(userName, []byte(keyContent), password)
		if err != nil {
			return err
		}
		c.Auth = auth
		return nil
	}
}

// WithInsecureIgnoreHostKey 跳过 SSH 主机密钥验证
// 适用于自动化工具、测试环境或信任的内网环境
// 警告：跳过主机密钥验证会降低安全性，可能遭受中间人攻击
// Example:
// ```
// git.Clone("git@github.com:user/repo.git", "/tmp/repo",
//
//	git.WithPrivateKeyContent("git", keyContent, ""),
//	git.WithInsecureIgnoreHostKey(),  // 跳过主机密钥验证
//
// )
// ```
func WithInsecureIgnoreHostKey() Option {
	return func(c *config) error {
		c.InsecureIgnoreHostKey = true
		return nil
	}
}

// threads 是一个GitHack选项函数，用于指定并发数，默认为8
// Example:
// ```
// git.GitHack("http://127.0.0.1:8787/git/website", "C:/Users/xxx/Desktop/githack-test", git.threads(8))
// ```
func WithThreads(threads int) Option {
	return func(c *config) error {
		c.Threads = threads
		return nil
	}
}

// useLocalGitBinary 是一个GitHack选项函数，用于指定是否使用本地环境变量的git二进制文件来执行`git fsck`命令，这个命令用于尽可能恢复完整的git仓库，默认为true
// Example:
// ```
// git.GitHack("http://127.0.0.1:8787/git/website", "C:/Users/xxx/Desktop/githack-test", git.useLocalGitBinary(true))
// ```
func WithUseLocalGitBinary(b bool) Option {
	return func(c *config) error {
		c.UseLocalGitBinary = b
		return nil
	}
}

// httpOpts 是一个GitHack选项函数，用于指定GitHack的HTTP选项，其接收零个到多个poc的请求选项函数
// Example:
// ```
// git.GitHack("http://127.0.0.1:8787/git/website", "C:/Users/xxx/Desktop/githack-test", git.httpOpts(poc.timeout(10), poc.https(true)))
// ```
func WithHTTPOptions(opts ...poc.PocConfigOption) Option {
	return func(c *config) error {
		c.HTTPOptions = opts
		return nil
	}
}

func WithProxy(proxyUrl, proxyName, proxyPasswd string) Option {
	return func(c *config) error {
		c.Proxy = transport.ProxyOptions{
			URL:      proxyUrl,
			Username: proxyName,
			Password: proxyPasswd,
		}
		return nil
	}
}
