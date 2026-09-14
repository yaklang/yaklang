package lowhttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/netx"
)

// ── Transport interface ──────────────────────────────────────────────────────

// Transport executes a single HTTP request over a specific protocol version
// (HTTP/1.1, HTTP/2, or HTTP/3).  Each implementation owns its connection
// management, stale-connection retries, and stream lifecycle.  The orchestration
// layer (HTTPWithoutRetry) selects a transport, handles protocol downgrades,
// and performs common request/response post-processing.
//
// Transports must NOT handle redirects, retry-on-status-code, or protocol
// downgrades — those are orchestration-layer concerns.
type Transport interface {
	// RoundTrip sends the prepared request packet and returns the raw response
	// bytes together with the parsed first http.Response (for cookie processing).
	// The transport handles stale-connection retries internally (bounded by
	// option.RetryTimes) so that a dead pooled connection surfaces as an error
	// rather than a silent hang.
	RoundTrip(ctx context.Context, tr *transportRequest) (*transportResult, error)

	// CanRetry reports whether a failed request can be safely retried at the
	// orchestration layer (e.g. REFUSED_STREAM, connection-closed errors for
	// idempotent methods).  Non-idempotent methods (POST, PATCH) return false.
	CanRetry(req *http.Request, err error) bool

	// ShouldDowngrade reports whether the error means the protocol is not
	// available on this server and the caller should fall back to HTTP/1.1.
	// Examples: ALPN did not negotiate h2, server preface timeout.
	ShouldDowngrade(err error) bool
}

// transportRequest carries everything a Transport needs from the common
// pre-processing in HTTPWithoutRetry.
type transportRequest struct {
	option     *LowhttpExecConfig
	reqIns     *http.Request          // parsed request instance (for httpctx)
	packet     []byte                 // prepared request packet (CRLF fixed, cookie injected)
	dialOpts   []netx.DialXOption     // assembled dial options (TLS, proxy, DNS, etc.)
	cacheKey   *connectKey            // pool cache key (scheme set by caller)
	connPool   *LowHttpConnPool       // connection pool (nil if WithConnPool is false)
	traceInfo  *LowhttpTraceInfo      // trace info for timing
	originAddr string                 // target host:port
	timeout    time.Duration          // per-request timeout
}

// transportResult carries everything the orchestration layer needs for
// common post-processing (FixHTTPResponse, cookie jar, save flow, etc.).
type transportResult struct {
	rawBytes        []byte           // wire response bytes (headers + body)
	firstResponse   *http.Response   // parsed first response (nil for H2/H3)
	multiResponses  []*http.Response // pipeline/smuggle responses (H1 only)
	isMultiResponse bool             // true if multiple responses were read
	remoteAddr      string           // server address
	portIsOpen      bool             // port was reachable
}

// ── Protocol-downgrade sentinel errors ───────────────────────────────────────

// ErrProtocolNotAvailable indicates the server does not support the requested
// protocol version (e.g. ALPN did not negotiate h2).  The caller should fall
// back to HTTP/1.1.
var ErrProtocolNotAvailable = errors.New("lowhttp: requested protocol not available")

// ErrServerPrefaceTimeout indicates the server negotiated h2 via ALPN but never
// sent its SETTINGS frame (connection preface).  Retrying would rebuild the same
// dead connection, so the caller should fall back to HTTP/1.1.
var ErrServerPrefaceTimeout = errors.New("lowhttp: server preface timeout")

// shouldDowngradeH2 inspects an H2 transport error and returns true when the
// caller should fall back to HTTP/1.1.
func shouldDowngradeH2(err error) bool {
	return errors.Is(err, ErrProtocolNotAvailable) ||
		errors.Is(err, ErrServerPrefaceTimeout) ||
		errors.Is(err, errH2ServerPrefaceTimeout)
}
