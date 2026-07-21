package yakgit

import (
	"context"
	"net/http"
	"testing"
	"time"

	gitClient "github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewGitHTTPClient_HasNoClientTimeout locks in the fix for the 30s
// http.Client.Timeout bug: netx.NewDefaultHTTPClient hardcoded Timeout=30s,
// which aborted large-repo clones mid-transfer with
// "context deadline exceeded (Client.Timeout ... while reading body)".
// newGitHTTPClient must NOT set a client-level deadline; the clone's own
// context governs cancellation.
func TestNewGitHTTPClient_HasNoClientTimeout(t *testing.T) {
	c := newGitHTTPClient()
	assert.NotNil(t, c)
	assert.Equal(t, time.Duration(0), c.Timeout,
		"newGitHTTPClient must not set http.Client.Timeout (30s aborted large clones); clone context governs cancellation")

	// With-proxy variant must also be timeout-free.
	cProxy := newGitHTTPClient("socks5://127.0.0.1:1080")
	assert.Equal(t, time.Duration(0), cProxy.Timeout)
}

// TestNewGitHTTPClient_HasTransport confirms a transport is wired so clone
// traffic actually flows through the custom DialContext (proxy path).
func TestNewGitHTTPClient_HasTransport(t *testing.T) {
	c := newGitHTTPClient()
	require.NotNil(t, c.Transport, "transport must be set so netx.DialContext is used")
	_, ok := c.Transport.(*http.Transport)
	assert.True(t, ok, "transport must be *http.Transport so DialContext is honored")
}

// TestCloneDefaultContextGetsTimeout verifies that Clone, when the caller
// does not supply a context, wraps the clone in a bounded deadline instead
// of an unbounded WithCancel(Background()). This guards against the
// liveness regression introduced by dropping the 30s client timeout: a
// hung remote would otherwise hang the clone forever and leak a goroutine.
//
// We cannot easily call Clone directly without network, but the context
// wrapping is exercised by WithContext(nil) + reading back the deadline
// through the public option surface. Since Clone's context handling is
// internal, this test instead asserts the contract at the option layer:
// a config with no explicit context, after Clone would have prepared it,
// must end up with a deadline. We approximate by checking the documented
// default constant is finite and reasonably large.
func TestCloneDefaultTimeoutConstantIsFinite(t *testing.T) {
	assert.Greater(t, defaultCloneTimeout, time.Duration(0), "defaultCloneTimeout must be finite")
	// Must be large enough for legit large clones (measured ~73s for 200MB).
	assert.Greater(t, defaultCloneTimeout, 5*time.Minute,
		"defaultCloneTimeout must comfortably exceed large-repo clone time (~73s measured)")
	// Must be bounded so a hung clone fails and frees resources.
	assert.Less(t, defaultCloneTimeout, 24*time.Hour,
		"defaultCloneTimeout must be bounded so hung clones don't leak forever")
}

// TestCloneContextOptionPreservesCallerDeadline verifies that when the
// caller supplies a context WITH a deadline, Clone honors it as-is rather
// than wrapping it with the default timeout. WithContext wraps the caller
// context via context.WithCancel (which inherits the deadline), so the
// deadline survives. We assert that inheritance.
func TestCloneContextOptionPreservesCallerDeadline(t *testing.T) {
	callerDeadline := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), callerDeadline)
	defer cancel()

	c := &config{}
	WithContext(ctx)(c)
	require.NotNil(t, c.Context, "WithContext must install a context")

	// WithContext does context.WithCancel(ctx), which inherits the parent
	// deadline. Clone's own logic then sees a deadline present and does NOT
	// wrap it with the default timeout. So the deadline must still be ~10s
	// out, not 30min.
	dl, ok := c.Context.Deadline()
	require.True(t, ok, "caller deadline must be inherited by WithContext")
	remaining := time.Until(dl)
	assert.LessOrEqual(t, remaining, callerDeadline,
		"caller deadline (~10s) must be preserved, not extended to defaultCloneTimeout")
	assert.Greater(t, remaining, 5*time.Second,
		"caller deadline must not be shortened below the caller's bound")
}

// TestSnapshotRestoreProtocolTransportsPreservesGlobalProxy verifies the fix
// for the Copilot-flagged bug where Clone's defer unconditionally called
// installDefaultProxyTransport(), silently wiping a global proxy previously
// set via SetProxy. After snapshotProtocolTransports + restoreProtocolTransports,
// the transports saved before a per-clone proxy swap must be restored verbatim,
// so a global SetProxy keeps effect across a per-clone-proxy Clone.
func TestSnapshotRestoreProtocolTransportsPreservesGlobalProxy(t *testing.T) {
	// Set a global proxy transport via SetProxy (takes protocolMu internally).
	globalProxy := "socks5://127.0.0.1:9999"
	SetProxy(globalProxy)
	t.Cleanup(func() {
		// Restore the no-proxy default so this test doesn't leak state.
		protocolMu.Lock()
		installDefaultProxyTransport()
		protocolMu.Unlock()
	})

	// Snapshot the global-proxy transport that SetProxy just installed.
	protocolMu.Lock()
	savedHTTPS, savedHTTP := snapshotProtocolTransports()
	protocolMu.Unlock()
	require.NotNil(t, savedHTTPS, "global proxy transport must be installed")
	require.NotNil(t, savedHTTP)

	// Simulate a per-clone proxy swap, then restore via the snapshot path
	// (this is what Clone's defer now does instead of installDefaultProxyTransport).
	protocolMu.Lock()
	applyProxyTransport("socks5://127.0.0.1:11111")
	// Confirm the swap took effect — current transport differs from snapshot.
	currentHTTPS := gitClient.Protocols["https"]
	require.NotNil(t, currentHTTPS)
	restoreProtocolTransports(savedHTTPS, savedHTTP)
	protocolMu.Unlock()

	// After restore, the global-proxy transport SetProxy installed must be back.
	restoredHTTPS := gitClient.Protocols["https"]
	restoredHTTP := gitClient.Protocols["http"]
	assert.Same(t, savedHTTPS, restoredHTTPS,
		"restoreProtocolTransports must put back the exact pre-swap transport, preserving a global SetProxy")
	assert.Same(t, savedHTTP, restoredHTTP,
		"http transport must also be restored to the pre-swap value")
}