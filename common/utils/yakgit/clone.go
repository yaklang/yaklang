package yakgit

import (
	"context"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitClient "github.com/go-git/go-git/v5/plumbing/transport/client"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	gossh "golang.org/x/crypto/ssh"
)

// defaultCloneTimeout bounds a clone whose caller did not supply a context
// with a deadline. It must be large enough that legit large-repo clones
// (measured ~73s for a 200MB repo) never abort, but not unbounded so a hung
// remote does not leak a goroutine and a temp dir forever. 30 minutes is a
// conservative middle ground between the old 30s (which aborted legit large
// clones mid-transfer) and no bound at all.
const defaultCloneTimeout = 30 * time.Minute

// protocolMu guards the global go-git protocol transport swaps done by
// init / SetProxy / applyProxyTransport / installDefaultProxyTransport.
// go-git's gitClient.InstallProtocol mutates a global registry, so concurrent
// clones that swap transports would race without this lock.
var protocolMu sync.Mutex

func init() {
	installDefaultProxyTransport()
}

// newGitHTTPClient builds an http.Client whose Transport uses netx.DialContext
// (so proxy, when given, is honored at the dial layer) but WITHOUT the 30s
// per-request timeout that netx.NewDefaultHTTPClient hardcodes.
//
// Why no client.Timeout: git clone of large repos (hadoop, yaklang, ...) can
// legitimately take minutes to transfer the pack body. netx.NewDefaultHTTPClient
// sets Timeout=30s, which aborts the clone mid-transfer with
// "context deadline exceeded (Client.Timeout ... while reading body)".
// The clone's own context (c.Context) already governs cancellation, so an
// extra client-level deadline only breaks long clones.
func newGitHTTPClient(proxy ...string) *http.Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return netx.DialContext(ctx, addr, proxy...)
		},
	}
	return &http.Client{Transport: tr}
}

// installDefaultProxyTransport registers the no-proxy yakgit HTTP transport
// for the https and http protocols.
//
// Caller must hold protocolMu — EXCEPT init(). Go guarantees package init
// runs single-threaded before any other code in the package (including
// goroutines launched from other init functions), so the very first call
// from yakgit.init() cannot race with anything. Locking there would be
// dead code; all other callers (SetProxy, Clone's restore defer) must hold
// the lock.
func installDefaultProxyTransport() {
	tr := gitHttp.NewClient(newGitHTTPClient())
	gitClient.InstallProtocol("https", tr)
	gitClient.InstallProtocol("http", tr)
}

// applyProxyTransport re-registers the global go-git https/http transports
// with an HTTP client whose custom DialContext dials through the given proxy.
//
// Why this is needed: go-git's built-in ProxyOptions support relies on
// http.Transport.Proxy, but yakgit's init() installs a transport with a
// custom DialContext (netx.DialContext). When DialContext is set, http.Transport
// bypasses the Proxy CONNECT path and dials directly, so go-git's ProxyOptions
// is silently ignored. To make per-clone proxy actually work, we must re-register
// a transport whose DialContext itself dials through the proxy.
//
// The registry is process-global (gitClient.Protocols), so while a proxied
// clone is in flight every other clone in the process observes this transport.
// Callers therefore MUST hold protocolMu for the entire duration the proxied
// transport is active (i.e. across the git.PlainCloneContext call), not just
// around the swap, so concurrent clones/SetProxy observe a consistent transport.
//
// Caller must hold protocolMu.
func applyProxyTransport(proxies ...string) {
	tr := gitHttp.NewClient(newGitHTTPClient(proxies...))
	gitClient.InstallProtocol("https", tr)
	gitClient.InstallProtocol("http", tr)
}

// snapshotProtocolTransports returns the currently-installed go-git https/http
// transports so they can be restored after a per-clone proxy swap. Caller
// must hold protocolMu so the snapshot is consistent with the swap that
// follows it.
func snapshotProtocolTransports() (https, http transport.Transport) {
	return gitClient.Protocols["https"], gitClient.Protocols["http"]
}

// restoreProtocolTransports puts back the previously-snapshotted transports.
// Caller must hold protocolMu.
func restoreProtocolTransports(https, http transport.Transport) {
	gitClient.InstallProtocol("https", https)
	gitClient.InstallProtocol("http", http)
}

// SetProxy 是一个辅助函数，用于指定其他 Git 操作（例如Clone）的代理
// 参数:
//   - proxies: 一个或多个代理地址
//
// 返回值:
//   - 无
//
// Example:
// ```
// git.SetProxy("http://127.0.0.1:1080")
// ```
func SetProxy(proxies ...string) {
	protocolMu.Lock()
	defer protocolMu.Unlock()
	applyProxyTransport(proxies...)
}

// Clone 用于克隆远程仓库并存储到本地路径中，它还可以接收零个到多个选项函数，用于影响克隆行为
// 参数:
//   - u: 远程仓库地址
//   - localPath: 本地存储路径
//   - opt: 可选项，如 git.recursive、git.verify、git.depth、git.auth 等
//
// 返回值:
//   - 错误信息
//
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
	// Bound the clone's lifetime when the caller did not supply a deadline.
	// The previous implementation relied on netx.NewDefaultHTTPClient's 30s
	// http.Client.Timeout as the only liveness bound; now that we use a
	// timeout-free client (see newGitHTTPClient), a hung remote would hang
	// forever. Wrap with a generous deadline (30min — large legit clones
	// measured ~73s for 200MB) so a stalled clone still fails and frees
	// its goroutine + temp dir. If the caller's context already has a
	// deadline, honor it as-is.
	if c.Context == nil {
		c.Context, c.Cancel = context.WithTimeout(context.Background(), defaultCloneTimeout)
	} else if _, ok := c.Context.Deadline(); !ok {
		c.Context, c.Cancel = context.WithTimeout(c.Context, defaultCloneTimeout)
	}
	defer c.Cancel()

	if c.InsecureIgnoreHostKey && c.Auth != nil {
		if sshAuth, ok := c.Auth.(*ssh.PublicKeys); ok {
			sshAuth.HostKeyCallback = gossh.InsecureIgnoreHostKey()
		}
	}

	// If a per-clone proxy is configured, re-register the global go-git
	// transport with a DialContext that dials through that proxy. go-git's
	// own ProxyOptions is silently ignored because yakgit's custom DialContext
	// bypasses http.Transport.Proxy, so we must swap the transport ourselves.
	//
	// Hold protocolMu for the ENTIRE clone, not just the swap: gitClient.Protocols
	// is a process-global map, and PlainCloneContext reads it at clone time, so
	// releasing the lock before the clone finishes would let a concurrent clone
	// (or SetProxy) overwrite the transport mid-flight, silently routing this
	// clone's requests through the wrong proxy. Proxied clones are rare
	// (scan-time, one per target) so serializing them is acceptable; no-proxy
	// clones never touch the registry and stay fully concurrent.
	//
	// Restore the PREVIOUS transport (snapshotted before the swap) on exit,
	// NOT the no-proxy default — a caller may have set a global proxy via
	// SetProxy that should remain in effect after this per-clone proxy clone
	// returns. Unconditionally calling installDefaultProxyTransport() here
	// would silently wipe that global setting.
	if c.Proxy.URL != "" {
		full, err := c.Proxy.FullURL()
		if err != nil {
			return utils.Wrapf(err, "git clone: %v to %v failed: invalid proxy url", u, localPath)
		}
		protocolMu.Lock()
		prevHTTPS, prevHTTP := snapshotProtocolTransports()
		applyProxyTransport(full.String())
		defer func() {
			restoreProtocolTransports(prevHTTPS, prevHTTP)
			protocolMu.Unlock()
		}()
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
