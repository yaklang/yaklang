package yakgit

import (
	"context"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitHTTP "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitSSH "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netx"
	gossh "golang.org/x/crypto/ssh"
)

const exactFetchTestRevision = "0123456789abcdef0123456789abcdef01234567"

// These tests mutate go-git's process-wide transport registry and must remain
// sequential. Keep both the transport and the DNS guard local to each test.
func isolateExactFetchTransports(t *testing.T) {
	t.Helper()
	protocolMu.Lock()
	previousHTTPS, previousHTTP := snapshotProtocolTransports()
	installDefaultProxyTransport()
	protocolMu.Unlock()
	t.Cleanup(func() {
		protocolMu.Lock()
		restoreProtocolTransports(previousHTTPS, previousHTTP)
		protocolMu.Unlock()
	})
	previousDNS := netx.GetDefaultOptions()
	netx.SetDefaultDNSOptions(append(previousDNS, netx.WithDNSDisabledDomain("*.invalid"))...)
	t.Cleanup(func() { netx.SetDefaultDNSOptions(previousDNS...) })
}

func newExactFetchRepository(t *testing.T, origin string) string {
	t.Helper()
	local := t.TempDir()
	repository, err := git.PlainInit(local, false)
	require.NoError(t, err)
	_, err = repository.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{origin}})
	require.NoError(t, err)
	return local
}

type exactFetchProxyRequest struct {
	method string
	host   string
}

func TestFetchExactRevisionUsesProxyAndRestoresTransportAfterFailure(t *testing.T) {
	isolateExactFetchTransports(t)
	for _, operation := range []string{"fetch", "clone"} {
		t.Run(operation, func(t *testing.T) {
			requests := make(chan exactFetchProxyRequest, 4)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requests <- exactFetchProxyRequest{method: r.Method, host: r.Host}:
				default:
				}
				http.Error(w, "fixture proxy unavailable", http.StatusServiceUnavailable)
			}))
			defer proxy.Close()
			const origin = "http://syntaxflow-fetch-origin.invalid/source.git"
			protocolMu.RLock()
			beforeHTTPS, beforeHTTP := snapshotProtocolTransports()
			protocolMu.RUnlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var err error
			if operation == "fetch" {
				err = FetchExactRevision(ctx, newExactFetchRepository(t, origin), exactFetchTestRevision,
					WithProxy(proxy.URL, "", ""))
			} else {
				err = Clone(origin, filepath.Join(t.TempDir(), "clone"), WithContext(ctx), WithProxy(proxy.URL, "", ""))
			}
			require.Error(t, err)
			protocolMu.RLock()
			afterHTTPS, afterHTTP := snapshotProtocolTransports()
			protocolMu.RUnlock()
			require.Same(t, beforeHTTPS, afterHTTPS)
			require.Same(t, beforeHTTP, afterHTTP)
			select {
			case request := <-requests:
				require.Equal(t, http.MethodConnect, request.method)
				require.Equal(t, "syntaxflow-fetch-origin.invalid:80", request.host,
					"the proxy must target origin, not tunnel back to the proxy itself")
			default:
				t.Fatal("the configured local proxy received no request")
			}
		})
	}
}

func TestFetchExactRevisionRejectsUntrustedTLSByDefault(t *testing.T) {
	isolateExactFetchTransports(t)
	var requests atomic.Int32
	var receivedUser, receivedPassword, receivedPath string
	fixture := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUser, receivedPassword, _ = r.BasicAuth()
		receivedPath = r.URL.RequestURI()
		requests.Add(1)
		http.Error(w, "fixture repository unavailable", http.StatusServiceUnavailable)
	}))
	fixture.Config.ErrorLog = log.New(io.Discard, "", 0)
	fixture.StartTLS()
	defer fixture.Close()
	local := newExactFetchRepository(t, fixture.URL+"/source.git")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := FetchExactRevision(ctx, local, exactFetchTestRevision)
	var untrusted x509.UnknownAuthorityError
	require.True(t, errors.As(err, &untrusted), "expected a certificate trust failure, got %v", err)
	require.Zero(t, requests.Load(), "an untrusted server must not receive Git HTTP requests")

	// The helper's explicit opt-out is only a test control. The Node caller
	// always supplies WithVerifyTLS(true), including pinned-revision recovery.
	err = FetchExactRevision(ctx, local, exactFetchTestRevision,
		WithVerifyTLS(false), WithUsernamePassword("fixture-user", "fixture-password"))
	require.Error(t, err)
	require.EqualValues(t, 1, requests.Load())
	require.Equal(t, "fixture-user", receivedUser)
	require.Equal(t, "fixture-password", receivedPassword)
	require.Equal(t, "/source.git/info/refs?service=git-upload-pack", receivedPath)

	err = FetchExactRevision(ctx, local, exactFetchTestRevision)
	require.True(t, errors.As(err, &untrusted), "explicit insecure options must not leak to later fetches")
	require.EqualValues(t, 1, requests.Load())
}

func TestFetchExactRevisionRejectsInvalidHashesBeforeNetwork(t *testing.T) {
	isolateExactFetchTransports(t)
	var requests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected network request", http.StatusServiceUnavailable)
	}))
	defer fixture.Close()
	local := newExactFetchRepository(t, fixture.URL+"/source.git")
	for _, revision := range []string{
		"", "main", "HEAD", "refs/heads/main", strings.Repeat("a", 39),
		strings.Repeat("a", 41), strings.Repeat("g", 40), strings.Repeat("0", 40),
		exactFetchTestRevision + "^", exactFetchTestRevision + ":refs/heads/main",
	} {
		t.Run(revision, func(t *testing.T) {
			err := FetchExactRevision(context.Background(), local, revision)
			require.ErrorContains(t, err, "full nonzero commit hash")
		})
	}
	require.Zero(t, requests.Load())
}

func TestFetchExactRevisionCancellationWhileWaitingForTransport(t *testing.T) {
	isolateExactFetchTransports(t)
	var requests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected network request", http.StatusServiceUnavailable)
	}))
	defer fixture.Close()
	local := newExactFetchRepository(t, fixture.URL+"/source.git")
	for _, tc := range []struct {
		name     string
		proxy    bool
		deadline bool
	}{
		{name: "read_lock_cancelled"},
		{name: "write_lock_cancelled", proxy: true},
		{name: "read_lock_deadline", deadline: true},
		{name: "write_lock_deadline", proxy: true, deadline: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			protocolMu.Lock()
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(protocolMu.Unlock) }
			defer release()
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if tc.deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 75*time.Millisecond)
				want = context.DeadlineExceeded
			}
			defer cancel()
			// A separate option context must never replace the authoritative
			// caller context and make a cancelled task wait indefinitely.
			opts := []Option{WithContext(context.Background())}
			if tc.proxy {
				opts = append(opts, WithProxy(fixture.URL, "", ""))
			}
			done := make(chan error, 1)
			started := time.Now()
			go func() { done <- FetchExactRevision(ctx, local, exactFetchTestRevision, opts...) }()
			if !tc.deadline {
				timer := time.AfterFunc(25*time.Millisecond, cancel)
				defer timer.Stop()
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, want)
				require.Less(t, time.Since(started), 600*time.Millisecond)
			case <-time.After(time.Second):
				release()
				cancel()
				select {
				case <-done:
				case <-time.After(time.Second):
				}
				t.Fatal("fetch did not stop while the protocol transport lock remained held")
			}
		})
	}
	require.Zero(t, requests.Load())
}

type exactFetchRoundTripper func(*http.Request) (*http.Response, error)

func (fn exactFetchRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestFetchExactRevisionAddsDeadlineToActualRequest(t *testing.T) {
	isolateExactFetchTransports(t)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fixture repository unavailable", http.StatusServiceUnavailable)
	}))
	defer fixture.Close()
	local := newExactFetchRepository(t, fixture.URL+"/source.git")
	baseTransport := &http.Transport{}
	defer baseTransport.CloseIdleConnections()
	var deadline time.Time
	var hasDeadline bool
	client := gitHTTP.NewClient(&http.Client{Transport: exactFetchRoundTripper(func(r *http.Request) (*http.Response, error) {
		deadline, hasDeadline = r.Context().Deadline()
		return baseTransport.RoundTrip(r)
	})})
	protocolMu.Lock()
	restoreProtocolTransports(client, client)
	protocolMu.Unlock()
	started := time.Now()
	err := FetchExactRevision(context.Background(), local, exactFetchTestRevision)
	require.Error(t, err)
	require.True(t, hasDeadline, "the real HTTP request must inherit a bounded operation context")
	require.WithinDuration(t, started.Add(defaultCloneTimeout), deadline, time.Second)
}

func TestCloneCannotUseAnotherFetchProxy(t *testing.T) {
	isolateExactFetchTransports(t)
	var originRequests, proxyRequests atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		originRequests.Add(1)
		http.Error(w, "fixture repository unavailable", http.StatusServiceUnavailable)
	}))
	defer origin.Close()
	entered := make(chan exactFetchProxyRequest, 4)
	unblock := make(chan struct{})
	var unblockOnce sync.Once
	releaseProxy := func() { unblockOnce.Do(func() { close(unblock) }) }
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests.Add(1)
		select {
		case entered <- exactFetchProxyRequest{method: r.Method, host: r.Host}:
		default:
		}
		select {
		case <-unblock:
		case <-r.Context().Done():
		}
		http.Error(w, "fixture proxy unavailable", http.StatusServiceUnavailable)
	}))
	defer proxy.Close()
	defer releaseProxy()
	local := newExactFetchRepository(t, "http://syntaxflow-fetch-origin.invalid/source.git")
	fetchCtx, cancelFetch := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelFetch()
	fetchDone := make(chan error, 1)
	go func() {
		fetchDone <- FetchExactRevision(fetchCtx, local, exactFetchTestRevision, WithProxy(proxy.URL, "", ""))
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		releaseProxy()
		<-fetchDone
		t.Fatal("fetch never entered the local proxy")
	}
	cloneCtx, cancelClone := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancelClone()
	cloneDone := make(chan error, 1)
	cloneLocal := filepath.Join(t.TempDir(), "blocked-clone")
	go func() {
		cloneDone <- Clone(origin.URL+"/source.git", cloneLocal,
			WithContext(cloneCtx), WithVerifyTLS(true))
	}()
	var cloneErr error
	select {
	case cloneErr = <-cloneDone:
	case <-time.After(time.Second):
		releaseProxy()
		<-fetchDone
		<-cloneDone
		t.Fatal("an ordinary clone failed to cancel behind a proxied fetch")
	}
	releaseProxy()
	require.Error(t, <-fetchDone)
	require.ErrorIs(t, cloneErr, context.DeadlineExceeded)
	require.Zero(t, originRequests.Load(), "the cancelled clone must not start an HTTP request")
	require.EqualValues(t, 1, proxyRequests.Load(), "another operation must not borrow the fetch proxy")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := Clone(origin.URL+"/source.git", filepath.Join(t.TempDir(), "direct-clone"),
		WithContext(ctx), WithVerifyTLS(true))
	require.Error(t, err)
	require.EqualValues(t, 1, originRequests.Load(), "after restoration an ordinary clone must contact only its own origin")
	require.EqualValues(t, 1, proxyRequests.Load())
}

// go-git consults known_hosts even with a HostKeyCallback when algorithms are
// empty. Set them in the synthetic auth so this fixture never reads user SSH
// files. The five-second dial timeout also bounds go-git's pre-session dial,
// which does not yet inherit the operation's context.
type exactFetchSSHPassword struct{ gitSSH.Password }

func (a *exactFetchSSHPassword) ClientConfig() (*gossh.ClientConfig, error) {
	cfg, err := a.Password.ClientConfig()
	if err != nil {
		return nil, err
	}
	cfg.HostKeyAlgorithms = []string{gossh.KeyAlgoED25519}
	cfg.Timeout = 5 * time.Second
	return cfg, nil
}

type exactFetchSOCKSObservation struct {
	host              string
	port              uint16
	registryUntouched bool
	sharedLockHeld    bool
	err               error
}

func readExactFetchSOCKSRequest(conn net.Conn, beforeHTTPS, beforeHTTP transport.Transport) (result exactFetchSOCKSObservation) {
	if result.err = conn.SetDeadline(time.Now().Add(5 * time.Second)); result.err != nil {
		return
	}
	var greeting [2]byte
	if _, result.err = io.ReadFull(conn, greeting[:]); result.err != nil {
		return
	}
	if greeting[0] != 5 || greeting[1] == 0 {
		result.err = fmt.Errorf("expected SOCKS5 method negotiation")
		return
	}
	methods := make([]byte, int(greeting[1]))
	if _, result.err = io.ReadFull(conn, methods); result.err != nil {
		return
	}
	if _, result.err = conn.Write([]byte{5, 0}); result.err != nil {
		return
	}
	var request [5]byte
	if _, result.err = io.ReadFull(conn, request[:]); result.err != nil {
		return
	}
	if request[0] != 5 || request[1] != 1 || request[2] != 0 || request[3] != 3 || request[4] == 0 {
		result.err = fmt.Errorf("expected SOCKS5 CONNECT with a domain target")
		return
	}
	target := make([]byte, int(request[4])+2)
	if _, result.err = io.ReadFull(conn, target); result.err != nil {
		return
	}
	result.host = string(target[:len(target)-2])
	result.port = binary.BigEndian.Uint16(target[len(target)-2:])
	if protocolMu.TryRLock() {
		duringHTTPS, duringHTTP := snapshotProtocolTransports()
		result.registryUntouched = beforeHTTPS == duringHTTPS && beforeHTTP == duringHTTP
		protocolMu.RUnlock()
		if protocolMu.TryLock() {
			protocolMu.Unlock()
		} else {
			result.sharedLockHeld = true
		}
	}
	return
}

func newExactFetchSOCKSFixture(t *testing.T, beforeHTTPS, beforeHTTP transport.Transport) (string, <-chan exactFetchSOCKSObservation) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	observed := make(chan exactFetchSOCKSObservation, 1)
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		conn, err := listener.Accept()
		if err != nil {
			observed <- exactFetchSOCKSObservation{err: err}
			return
		}
		defer conn.Close()
		result := readExactFetchSOCKSRequest(conn, beforeHTTPS, beforeHTTP)
		observed <- result
		if result.err == nil {
			// SOCKS5 "connection not allowed". Never dial or forward the target.
			_, _ = conn.Write([]byte{5, 2, 0, 1, 0, 0, 0, 0, 0, 0})
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-closed
	})
	return "socks5://" + listener.Addr().String(), observed
}

func TestFetchExactRevisionSSHAndSCPUseExplicitSOCKSProxy(t *testing.T) {
	isolateExactFetchTransports(t)
	previousSSHConfig := gitSSH.DefaultSSHConfig
	gitSSH.DefaultSSHConfig = nil
	t.Cleanup(func() { gitSSH.DefaultSSHConfig = previousSSHConfig })
	// A regression that drops ProxyOptions must fail without external DNS or
	// environment-proxy traffic, and it must not count as a successful test.
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("external DNS is disabled by the SOCKS fixture")
	}}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })
	for _, name := range []string{"ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(name, "")
	}
	for _, locator := range []struct{ name, value string }{
		{name: "ssh_url", value: "ssh://git@syntaxflow-fetch-origin.invalid/repo"},
		{name: "scp", value: "git@syntaxflow-fetch-origin.invalid:repo"},
	} {
		for _, operation := range []string{"fetch", "clone"} {
			t.Run(operation+"_"+locator.name, func(t *testing.T) {
				protocolMu.RLock()
				beforeHTTPS, beforeHTTP := snapshotProtocolTransports()
				protocolMu.RUnlock()
				proxyURL, observed := newExactFetchSOCKSFixture(t, beforeHTTPS, beforeHTTP)
				auth := &exactFetchSSHPassword{Password: gitSSH.Password{
					User: "git", Password: "fixture-password",
					HostKeyCallbackHelper: gitSSH.HostKeyCallbackHelper{HostKeyCallback: gossh.InsecureIgnoreHostKey()},
				}}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				opts := []Option{WithContext(ctx), WithProxy(proxyURL, "", ""), func(c *config) error {
					c.Auth = auth
					return nil
				}}
				var err error
				if operation == "fetch" {
					err = FetchExactRevision(ctx, newExactFetchRepository(t, locator.value), exactFetchTestRevision, opts...)
				} else {
					err = Clone(locator.value, filepath.Join(t.TempDir(), "clone"), opts...)
				}
				require.Error(t, err, "the fixture always rejects CONNECT")
				select {
				case request := <-observed:
					require.NoError(t, request.err)
					require.Equal(t, "syntaxflow-fetch-origin.invalid", request.host)
					require.EqualValues(t, 22, request.port)
					require.True(t, request.registryUntouched, "SSH must not swap the HTTP(S) registry")
					require.True(t, request.sharedLockHeld, "SSH must share the transport read lock while dialing")
				default:
					t.Fatalf("the SOCKS5 fixture received no request; a direct-connect error is not proxy evidence: %v", err)
				}
				protocolMu.RLock()
				afterHTTPS, afterHTTP := snapshotProtocolTransports()
				protocolMu.RUnlock()
				require.Same(t, beforeHTTPS, afterHTTPS)
				require.Same(t, beforeHTTP, afterHTTP)
			})
		}
	}
}
