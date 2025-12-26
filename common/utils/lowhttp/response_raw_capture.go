package lowhttp

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// responseRawCaptureWriter captures response raw bytes into dst and can auto-detect SSE by response headers.
//
// When autoDetectSSE is enabled and response headers include Content-Type: text/event-stream,
// it will:
//  1. set httpctx.NoBodyBuffer = true on the request instance to prevent body buffering in the parser
//  2. stop writing body bytes into dst (dst will contain headers only)
//
// This is required to support SSE auto-detection without requiring request Accept: text/event-stream,
// while avoiding unbounded memory growth from buffering long-lived streams.
type responseRawCaptureWriter struct {
	dst           *bytes.Buffer
	req           *http.Request
	autoDetectSSE bool

	seenHeaderEnd bool
	matchState    int
	headerBuf     bytes.Buffer

	discardBody bool
}

func (w *responseRawCaptureWriter) Write(p []byte) (int, error) {
	if w == nil || w.dst == nil {
		return len(p), nil
	}
	if w.seenHeaderEnd {
		if w.discardBody {
			return len(p), nil
		}
		_, _ = w.dst.Write(p)
		return len(p), nil
	}

	// Scan header until \r\n\r\n appears. Header sizes are typically small, so per-byte scan is ok.
	for i := 0; i < len(p); i++ {
		b := p[i]
		w.headerBuf.WriteByte(b)
		w.dst.WriteByte(b)

		switch w.matchState {
		case 0:
			if b == '\r' {
				w.matchState = 1
			}
		case 1:
			if b == '\n' {
				w.matchState = 2
			} else if b == '\r' {
				w.matchState = 1
			} else {
				w.matchState = 0
			}
		case 2:
			if b == '\r' {
				w.matchState = 3
			} else {
				w.matchState = 0
			}
		case 3:
			if b == '\n' {
				w.seenHeaderEnd = true
				w.matchState = 0

				if w.autoDetectSSE {
					headerLower := strings.ToLower(w.headerBuf.String())
					if strings.Contains(headerLower, "content-type:") && strings.Contains(headerLower, "text/event-stream") {
						w.discardBody = true
						if w.req != nil {
							httpctx.SetNoBodyBuffer(w.req, true)
						}
					}
				}

				rest := p[i+1:]
				if len(rest) > 0 && !w.discardBody {
					_, _ = w.dst.Write(rest)
				}
				return len(p), nil
			} else {
				w.matchState = 0
			}
		}
	}
	return len(p), nil
}
