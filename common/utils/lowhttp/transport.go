package lowhttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/netx"
)

// ── Transport interface ──────────────────────────────────────────────────────

// transport executes a single HTTP request over a specific protocol version
// (HTTP/1.1, HTTP/2, or HTTP/3).  Each implementation owns its connection
// management and stream lifecycle. The orchestration layer (HTTPWithoutRetry)
// selects a transport, handles protocol downgrades and H1 stale-connection
// retries, and performs common request/response post-processing.
//
// Transports must NOT handle redirects, retry-on-status-code, or protocol
// downgrades — those are orchestration-layer concerns.
type transport interface {
	// RoundTrip sends the prepared request packet and returns the raw response
	// bytes together with the parsed first http.Response (for cookie processing).
	// An error may accompany a partial result containing connection diagnostics.
	// H2 owns its stream retry budget; H1 signals stale pooled connections to
	// the orchestration layer.
	RoundTrip(ctx context.Context, tr *transportRequest) (*transportResult, error)

	// ShouldDowngrade reports whether the error means the protocol is not
	// available on this server and the caller should fall back to HTTP/1.1.
	// The caller must also decide if replay is safe: a server preface timeout
	// does not prove that a previously sent request was unprocessed.
	ShouldDowngrade(err error) bool
}

// transportRequest carries everything a transport needs from the common
// pre-processing in HTTPWithoutRetry.
type transportRequest struct {
	option         *LowhttpExecConfig
	reqIns         *http.Request      // parsed request instance (for httpctx)
	packet         []byte             // prepared request packet (CRLF fixed, cookie injected)
	dialOpts       []netx.DialXOption // assembled dial options (TLS, proxy, DNS, etc.)
	cacheKey       *connectKey        // pool cache key (scheme set by caller)
	connPool       *LowHttpConnPool   // connection pool for H1 reuse and H2 connections
	usePool        bool               // final H1 connection policy for this attempt
	preserveLength bool               // final NoFixContentLength value after pipeline detection
	h1Conn         net.Conn           // negotiated H1 socket handed off by H2; consumed once
	traceInfo      *LowhttpTraceInfo  // trace info for timing
	originAddr     string             // target host:port
	timeout        time.Duration      // per-request timeout
}

// transportResult carries everything the orchestration layer needs for
// common post-processing (FixHTTPResponse, cookie jar, save flow, etc.).
type transportResult struct {
	rawBytes        []byte           // H1 wire bytes or a parsed H2/H3 response representation
	firstResponse   *http.Response   // parsed first response, when available
	multiResponses  []*http.Response // pipeline/smuggle responses (H1 only)
	isMultiResponse bool             // true if multiple responses were read
	remoteAddr      string           // server address
	portIsOpen      bool             // port was reachable
	h1Conn          net.Conn         // only set with ErrProtocolNotAvailable; caller takes ownership
}

// ── Protocol-downgrade sentinel errors ───────────────────────────────────────

// ErrProtocolNotAvailable indicates the server does not support the requested
// protocol version (e.g. ALPN did not negotiate h2).  The caller should fall
// back to HTTP/1.1.
var ErrProtocolNotAvailable = errors.New("lowhttp: requested protocol not available")

// ErrServerPrefaceTimeout indicates the server negotiated h2 via ALPN but never
// sent its SETTINGS frame (connection preface). An already sent request may
// have been processed; only replay-safe requests may fall back to HTTP/1.1.
var ErrServerPrefaceTimeout = errors.New("lowhttp: server preface timeout")

// shouldDowngradeH2 inspects an H2 transport error and returns true when the
// caller should fall back to HTTP/1.1.
func shouldDowngradeH2(err error) bool {
	return errors.Is(err, ErrProtocolNotAvailable) ||
		errors.Is(err, ErrServerPrefaceTimeout) ||
		errors.Is(err, errH2ServerPrefaceTimeout)
}
