package yakgit

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// FetchExactRevision retrieves one immutable commit from the checkout's
// configured origin, without shared repository caching or an unbounded history
// fetch. This Go-only helper is deliberately not exposed to Yak scripts.
func FetchExactRevision(ctx context.Context, local, revision string, opts ...Option) error {
	revision = strings.ToLower(strings.TrimSpace(revision))
	decoded, err := hex.DecodeString(revision)
	if err != nil || len(decoded) != 20 || revision == strings.Repeat("0", 40) {
		return fmt.Errorf("git exact fetch requires a full nonzero commit hash")
	}
	c := NewConfig()
	defer c.Cancel()
	if ctx == nil {
		ctx = context.Background()
	}
	for _, option := range opts {
		if err := option(c); err != nil {
			c.Cancel()
			return err
		}
	}
	defer c.Cancel()
	// The explicit caller context remains authoritative even if an Option has
	// its own context. Bound a caller without a deadline, as Clone does.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCloneTimeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	repository, err := git.PlainOpen(local)
	if err != nil {
		return err
	}
	origin, err := repository.Remote("origin")
	if err != nil {
		return err
	}
	if len(origin.Config().URLs) == 0 {
		return fmt.Errorf("git exact fetch requires a configured origin")
	}
	dialProxy, endpointProxy := gitProxyOptionsForEndpoint(origin.Config().URLs[0], c.Proxy)
	releaseTransport, err := lockGitProtocolTransport(ctx, dialProxy)
	if err != nil {
		return err
	}
	defer releaseTransport()
	return repository.FetchContext(ctx, &git.FetchOptions{
		RemoteName:      "origin",
		RefSpecs:        []gitconfig.RefSpec{gitconfig.RefSpec(revision + ":refs/legion/source-pin")},
		Depth:           1,
		Tags:            git.NoTags,
		Auth:            c.Auth,
		ProxyOptions:    endpointProxy,
		InsecureSkipTLS: !c.VerifyTLS,
	})
}

// HTTP(S) uses yakgit's netx dial transport; SSH/SCP must retain go-git's
// endpoint proxy instead. Applying both doubles the HTTP proxy hop, while
// dropping both silently bypasses an explicitly configured SSH proxy.
func gitProxyOptionsForEndpoint(locator string, configured transport.ProxyOptions) (dial, endpoint transport.ProxyOptions) {
	parsed, err := transport.NewEndpoint(locator)
	if err == nil && (parsed.Protocol == "http" || parsed.Protocol == "https") {
		return configured, transport.ProxyOptions{}
	}
	return transport.ProxyOptions{}, configured
}

// lockGitProtocolTransport protects the full transport lifetime, including
// callers without a per-operation proxy. A cancelled operation must not wait
// for another repository's potentially long transfer before it can clean up.
func lockGitProtocolTransport(ctx context.Context, proxy transport.ProxyOptions) (func(), error) {
	var proxyURL string
	if proxy.URL != "" {
		full, err := proxy.FullURL()
		if err != nil {
			return nil, fmt.Errorf("git transport has an invalid proxy URL")
		}
		proxyURL = full.String()
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if proxyURL == "" {
			if protocolMu.TryRLock() {
				return protocolMu.RUnlock, nil
			}
		} else if protocolMu.TryLock() {
			previousHTTPS, previousHTTP := snapshotProtocolTransports()
			applyProxyTransport(proxyURL)
			return func() {
				restoreProtocolTransports(previousHTTPS, previousHTTP)
				protocolMu.Unlock()
			}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
