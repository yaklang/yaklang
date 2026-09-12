package pcaputil

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var errBinContext = errors.New("protocol context required")

type binHTTPState struct {
	header, total, cursor, next                         int
	chunked, trailers, closeDelimited, response, tunnel bool
	method, summary                                     string
	status                                              int
}

func (h *binHTTPState) config() map[string]any {
	return map[string]any{"httpResponseToMethod": h.method, "httpCloseDelimited": h.closeDelimited}
}

func (f *binFlow) frameDirection(dir int, w []byte) (int, *binSpec, error) {
	if f.protocol != "http" || f.binding != nil {
		return f.frame(w)
	}
	d := &f.directions[dir]
	h := d.http
	if h == nil {
		end := bytes.Index(w, []byte("\r\n\r\n"))
		if end < 0 {
			if len(w) > 32<<10 {
				return 0, nil, fmt.Errorf("HTTP header exceeds 32 KiB")
			}
			return 0, nil, nil
		}
		end += 4
		if end > 32<<10 {
			return 0, nil, fmt.Errorf("HTTP header exceeds 32 KiB")
		}
		lower := bytes.ToLower(w[:end])
		if bytes.Contains(lower, []byte("\r\ncontent-length:")) && bytes.Contains(lower, []byte("\r\ntransfer-encoding:")) {
			return 0, nil, fmt.Errorf("HTTP has conflicting length and transfer encoding")
		}
		h = &binHTTPState{header: end, cursor: end, summary: string(bytes.SplitN(w[:end], []byte("\r\n"), 2)[0])}
		r := bufio.NewReader(bytes.NewReader(w[:end]))
		var length int64
		var transfer []string
		if bytes.HasPrefix(w, []byte("HTTP/")) {
			h.response = true
			if len(f.httpMethods) == 0 {
				return 0, nil, fmt.Errorf("%w: HTTP response has no observed request method", errBinContext)
			}
			h.method = f.httpMethods[0]
			response, err := http.ReadResponse(r, &http.Request{Method: h.method})
			if err != nil {
				return 0, nil, err
			}
			if response.Proto != "HTTP/1.0" && response.Proto != "HTTP/1.1" {
				return 0, nil, fmt.Errorf("unsupported HTTP version")
			}
			h.status = response.StatusCode
			length, transfer = response.ContentLength, response.TransferEncoding
			h.tunnel = h.status == 101 || (h.method == "CONNECT" && h.status >= 200 && h.status < 300)
			if h.method == "HEAD" || h.status < 200 || h.status == 204 || h.status == 304 || h.tunnel {
				length, transfer = 0, nil
			}
		} else {
			request, err := http.ReadRequest(r)
			if err != nil {
				return 0, nil, err
			}
			if request.Proto != "HTTP/1.0" && request.Proto != "HTTP/1.1" {
				return 0, nil, fmt.Errorf("unsupported HTTP version")
			}
			h.method = request.Method
			length, transfer = request.ContentLength, request.TransferEncoding
			if len(f.httpMethods) >= 128 {
				return 0, nil, fmt.Errorf("%w: HTTP request pipeline exceeds 128 entries", errBinContext)
			}
		}
		h.chunked = len(transfer) == 1 && transfer[0] == "chunked"
		if len(transfer) > 0 && !h.chunked {
			return 0, nil, fmt.Errorf("unsupported HTTP transfer encoding")
		}
		h.closeDelimited = h.response && length < 0 && !h.chunked
		if length > int64(f.a.config.MaxMessageBytes-end) {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		if length >= 0 {
			h.total = end + int(length)
		}
		d.http = h
		// Expect: 100-continue can produce a response before the request body.
		// Associate the method as soon as its complete header is validated.
		if !h.response {
			f.httpMethods = append(f.httpMethods, h.method)
		}
	}
	if h.closeDelimited {
		return 0, nil, nil
	} // only a clean TCP FIN can end this body
	if !h.chunked {
		return h.total, f.spec("http", "HTTPExact"), nil
	}
	for {
		if h.trailers {
			if len(w) < h.cursor+2 {
				return 0, nil, nil
			}
			if bytes.Equal(w[h.cursor:h.cursor+2], []byte("\r\n")) {
				return h.cursor + 2, f.spec("http", "HTTPExact"), nil
			}
			end := bytes.Index(w[h.cursor:], []byte("\r\n\r\n"))
			if end < 0 {
				if len(w)-h.cursor > 32<<10 {
					return 0, nil, fmt.Errorf("HTTP trailers exceed limit")
				}
				return 0, nil, nil
			}
			return h.cursor + end + 4, f.spec("http", "HTTPExact"), nil
		}
		if h.next > 0 {
			if len(w) < h.next+2 {
				return 0, nil, nil
			}
			if !bytes.Equal(w[h.next:h.next+2], []byte("\r\n")) {
				return 0, nil, fmt.Errorf("HTTP chunk has no trailing CRLF")
			}
			h.cursor, h.next = h.next+2, 0
		}
		end := bytes.Index(w[h.cursor:], []byte("\r\n"))
		if end < 0 {
			if len(w)-h.cursor > 4096 {
				return 0, nil, fmt.Errorf("HTTP chunk header exceeds limit")
			}
			return 0, nil, nil
		}
		line := string(w[h.cursor : h.cursor+end])
		sizeText, _, _ := strings.Cut(line, ";")
		size, err := strconv.ParseUint(sizeText, 16, 32)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid HTTP chunk size")
		}
		h.cursor += end + 2
		if size == 0 {
			h.trailers = true
			continue
		}
		if size > uint64(f.a.config.MaxMessageBytes-h.cursor-2) {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		h.next = h.cursor + int(size)
	}
}

func (f *binFlow) finishHTTP(dir int) {
	h := f.directions[dir].http
	if h.response {
		if h.status >= 200 || h.status == 101 {
			f.httpMethods[0] = ""
			f.httpMethods = f.httpMethods[1:]
		}
	}
	f.directions[dir].http = nil
	if h.tunnel {
		// A confirmed CONNECT/101 response is an explicit protocol boundary.
		// Re-probe the next ordered bytes once; keep the capture flow identity.
		f.protocol, f.binding, f.level = "", nil, 0
		f.httpMethods = nil
	}
}
