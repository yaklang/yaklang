package yakgrpc

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/andybalholm/brotli"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const previewResourceLimit = 4 * 1024 * 1024

func previewURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || len(raw) > 8192 {
		return nil, fmt.Errorf("invalid preview URL")
	}
	u.Fragment = ""
	return u, nil
}

func previewOrigin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

func previewPublicIP(raw string) bool {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "2001::/32"} {
		if netip.MustParsePrefix(cidr).Contains(ip) {
			return false
		}
	}
	return true
}

func previewMime(kind, declared string, data []byte) (string, error) {
	declared, _, _ = mime.ParseMediaType(declared)
	switch kind {
	case "css":
		if declared == "text/css" && utf8.Valid(data) {
			return "text/css", nil
		}
	case "font":
		for magic, typ := range map[string]string{"wOFF": "font/woff", "wOF2": "font/woff2", "OTTO": "font/otf", "\x00\x01\x00\x00": "font/ttf"} {
			if bytes.HasPrefix(data, []byte(magic)) {
				return typ, nil
			}
		}
	case "image":
		typ := http.DetectContentType(data)
		switch typ {
		case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/x-icon":
			return typ, nil
		}
		if len(data) >= 16 && string(data[4:8]) == "ftyp" && (string(data[8:12]) == "avif" || string(data[8:12]) == "avis") {
			return "image/avif", nil
		}
	}
	return "", fmt.Errorf("unsupported preview resource content")
}

// FetchHTTPPreviewResource reuses lowhttp transport without invoking a Fuzzer task.
// The caller supplies explicit URLs discovered only after the user's render click.
func (s *Server) FetchHTTPPreviewResource(parent context.Context, req *ypb.HTTPPreviewResourceRequest) (*ypb.HTTPPreviewResourceResponse, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	current, err := previewURL(req.GetURL())
	if err != nil {
		return nil, err
	}
	page, err := previewURL(req.GetPageURL())
	if err != nil {
		return nil, err
	}
	if req.Kind != "image" && req.Kind != "css" && req.Kind != "font" {
		return nil, fmt.Errorf("invalid resource kind")
	}
	n := req.GetNetwork()
	if n == nil {
		n = &ypb.FuzzerRequest{}
	}
	if n.BrowserExtensionDeviceId != "" || n.BrowserTransformProfileId != "" {
		return nil, fmt.Errorf("preview does not support browser transform routing")
	}
	hosts := make(map[string]string)
	for _, pair := range n.GetEtcHosts() {
		hosts[pair.Key] = pair.Value
	}
	for hop := 0; hop < 4; hop++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sameOrigin := previewOrigin(current) == previewOrigin(page)
		host := current.Hostname()
		port := current.Port()
		if port == "" {
			if current.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		if sameOrigin && n.ActualAddr != "" {
			override, overridePort, splitErr := net.SplitHostPort(n.ActualAddr)
			if splitErr == nil {
				host, port = override, overridePort
			} else {
				host = strings.Trim(n.ActualAddr, "[]")
			}
		}
		// Pin the checked address into lowhttp, including when a proxy is used. The
		// original Host and TLS SNI stay intact; redirects undergo the same validation.
		ip := netx.LookupFirstWithContext(ctx, host, netx.WithDNSServers(n.DNSServers...), netx.WithTemporaryHosts(hosts))
		if net.ParseIP(ip) == nil || (!sameOrigin && !previewPublicIP(ip)) {
			return nil, fmt.Errorf("preview target is outside the permitted network scope")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept-Encoding", "identity")
		request.Header.Set("Connection", "close")
		packet, err := httputil.DumpRequestOut(request, false)
		if err != nil {
			return nil, err
		}
		route := &ypb.FuzzerRequest{Proxy: n.Proxy, ProxyRuleId: n.ProxyRuleId, RequestRaw: packet, IsHTTPS: current.Scheme == "https"}
		if sameOrigin {
			route.ActualAddr = n.ActualAddr
		}
		proxies, err := buildFuzzerProxyList(route)
		if err != nil {
			return nil, err
		}
		var header *http.Response
		var body []byte
		var readErr error
		opts := []lowhttp.LowhttpOpt{
			lowhttp.WithRequest(packet), lowhttp.WithHttps(current.Scheme == "https"),
			lowhttp.WithHost(ip), lowhttp.WithContext(ctx),
			lowhttp.WithTimeout(5 * time.Second), lowhttp.WithConnectTimeout(3 * time.Second),
			lowhttp.WithProxy(proxies...), lowhttp.WithEnableSystemProxyFromEnv(!n.NoSystemProxy),
			lowhttp.WithSaveHTTPFlow(false), lowhttp.WithDisableSession(true),
			lowhttp.WithUseMITMRule(false), lowhttp.WithConnPool(false), lowhttp.WithRetryTimes(0),
			lowhttp.WithMaxContentLength(previewResourceLimit + 1), lowhttp.WithNoBodyBuffer(true),
			lowhttp.WithGmTLS(n.IsGmTLS), lowhttp.WithRandomJA3FingerPrint(n.RandomJA3),
			lowhttp.WithBodyStreamReaderHandler(func(rawHeader []byte, reader io.ReadCloser) {
				defer reader.Close()
				header, readErr = http.ReadResponse(bufio.NewReader(bytes.NewReader(rawHeader)), request)
				if readErr != nil {
					return
				}
				var decoded io.Reader = io.LimitReader(reader, previewResourceLimit+1)
				switch strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Encoding"))) {
				case "", "identity":
				case "gzip":
					var zipped *gzip.Reader
					zipped, readErr = gzip.NewReader(decoded)
					if readErr != nil {
						return
					}
					defer zipped.Close()
					decoded = zipped
				case "deflate":
					var zipped io.ReadCloser
					zipped, readErr = zlib.NewReader(decoded)
					if readErr != nil {
						return
					}
					defer zipped.Close()
					decoded = zipped
				case "br":
					decoded = brotli.NewReader(decoded)
				default:
					readErr = fmt.Errorf("unsupported preview content encoding")
					return
				}
				body, readErr = io.ReadAll(io.LimitReader(decoded, previewResourceLimit+1))
			}),
		}
		portNumber, err := net.LookupPort("tcp", port)
		if err != nil {
			return nil, err
		}
		opts = append(opts, lowhttp.WithPort(portNumber))
		sni := current.Hostname()
		if sameOrigin && n.OverwriteSNI {
			sni = n.SNI
		}
		opts = append(opts, lowhttp.WithSNI(sni))
		_, err = lowhttp.HTTPWithoutRedirect(opts...)
		if err != nil {
			return nil, err
		}
		if readErr != nil {
			return nil, readErr
		}
		if header == nil || len(body) > previewResourceLimit {
			return nil, fmt.Errorf("preview response too large or incomplete")
		}
		if header.StatusCode >= 300 && header.StatusCode <= 399 {
			location, err := current.Parse(header.Header.Get("Location"))
			if err != nil || header.Header.Get("Location") == "" {
				return nil, fmt.Errorf("invalid preview redirect")
			}
			current, err = previewURL(location.String())
			if err != nil {
				return nil, err
			}
			continue
		}
		if header.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("preview returned HTTP %d", header.StatusCode)
		}
		typ, err := previewMime(req.Kind, header.Header.Get("Content-Type"), body)
		if err != nil {
			return nil, err
		}
		return &ypb.HTTPPreviewResourceResponse{URL: current.String(), MimeType: typ, Data: body}, nil
	}
	return nil, fmt.Errorf("too many preview redirects")
}
