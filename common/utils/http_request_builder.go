package utils

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// commonHeader interns common header strings.
var commonHeader = map[string]string{
	"Accept":                    "Accept",
	"Accept-Charset":            "Accept-Charset",
	"Accept-Encoding":           "Accept-Encoding",
	"Accept-Language":           "Accept-Language",
	"Accept-Ranges":             "Accept-Ranges",
	"Cache-Control":             "Cache-Control",
	"Cc":                        "Cc",
	"Connection":                "Connection",
	"Content-Id":                "Content-Id",
	"Content-Language":          "Content-Language",
	"Content-Length":            "Content-Length",
	"Content-Transfer-Encoding": "Content-Transfer-Encoding",
	"Content-Type":              "Content-Type",
	"Cookie":                    "Cookie",
	"Date":                      "Date",
	"Etag":                      "Etag",
	"Expires":                   "Expires",
	"From":                      "From",
	"Host":                      "Host",
	"If-Modified-Since":         "If-Modified-Since",
	"If-None-Match":             "If-None-Match",
	"In-Reply-To":               "In-Reply-To",
	"Last-Modified":             "Last-Modified",
	"Location":                  "Location",
	"Message-Id":                "Message-Id",
	"Mime-Version":              "Mime-Version",
	"Pragma":                    "Pragma",
	"Received":                  "Received",
	"Return-Path":               "Return-Path",
	"Server":                    "Server",
	"Set-Cookie":                "Set-Cookie",
	"Subject":                   "Subject",
	"To":                        "To",
	"User-Agent":                "User-Agent",
	"X-Forwarded-For":           "X-Forwarded-For",
	"X-Powered-By":              "X-Powered-By",
}

// ParseHTTPRequestLine parses "GET /foo HTTP/1.1" into its three parts.
func ParseHTTPRequestLine(line string) (method, requestURI, proto string, ok bool) {
	s1 := strings.Index(line, " ")
	s2 := strings.LastIndex(line[s1+1:], " ")
	if s1 < 0 {
		return
	}

	httpVersion := "HTTP/1.1"
	if s2 < 0 {
		return line[:s1], line[s1+1:], httpVersion, true
	}
	s2 += s1 + 1
	return line[:s1], line[s1+1 : s2], line[s2+1:], true
}

func ReadHTTPRequestFromBufioReader(reader *bufio.Reader) (*http.Request, error) {
	return readHTTPRequestFromBufioReader(reader, false, nil)
}

func ReadHTTPRequestFromBufioReaderOnFirstLine(reader *bufio.Reader, h func(string)) (*http.Request, error) {
	return readHTTPRequestFromBufioReader(reader, false, h)
}

func ReadHTTPRequestFromBytes(raw []byte) (*http.Request, error) {
	return readHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewReader(raw)), true, nil)
}

const minIPInteger uint32 = 1 << 24

func ParseStringToUrl(s string) *url.URL {
	// schema://user:password@host:port/path?query#fragment
	// schema://user:password@host:port/path;param?query#fragment
	// schema://host:port/path;param?query#fragment
	// ://host:port/path;param?query#fragment
	// my-app+secure://example.com:80//proxy/https://github.proxy.com
	// baidu.com:443http://example.com
	// baidu.com:443
	// baidu.com
	// 192.168.1.1:
	// 0x01000000
	// http://baidu.com?a=1
	u := new(url.URL)

	// handle #
	s, fragment, fragmentOk := strings.Cut(s, "#")
	if fragmentOk {
		u.RawFragment = fragment
	}

	haveSchemeSplit := false
RETRY:
	if strings.HasPrefix(s, "/") {
		// /path?query#fragment
		// /path;param?query#fragment params
		var after string
		var ok bool
		u.Path, after, ok = strings.Cut(s, "?")
		if ok {
			u.RawQuery, after, ok = strings.Cut(after, "#")
			if ok {
				u.Fragment = after
			}
		}
		return u
	} else if strings.HasPrefix(s, "://") && !haveSchemeSplit {
		s = strings.TrimPrefix(s, "://")
		haveSchemeSplit = true
		goto RETRY
	} else if strings.Contains(s, "://") && !haveSchemeSplit {
		origin := s
		var scheme string
		scheme, s, haveSchemeSplit = strings.Cut(origin, "://")
		u.Scheme = scheme
		if strings.Contains(scheme, ".") {
			log.Warnf("unhealthy schema(%v) found in %v", scheme, origin)
		}
		goto RETRY
	} else {
		// checking /
		if strings.Contains(s, "/") {
			var after string
			var ok bool
			u.Host, after, ok = strings.Cut(s, "/")
			if ok {
				after = "/" + after
			}
			if after != "" {
				u.Path, after, ok = strings.Cut(after, "?")
				if ok {
					u.RawQuery, after, ok = strings.Cut(after, "#")
					if ok {
						u.Fragment = after
					}
				}
			}
		} else if strings.Contains(s, ":") {
			hostname, port, ok := strings.Cut(s, ":")
			if ok && codec.Atoi(port) > 0 && strings.Trim(hostname, ": ") != "" {
				u.Host = HostPort(hostname, port)
			} else if !ok || strings.TrimSpace(port) == "" {
				u.Host = hostname
			} else {
				u.Host = HostPort(hostname, port)
			}
		} else {
			var queryOk bool
			var result string
			result, u.RawQuery, queryOk = strings.Cut(s, "?")
			if u.Host == "" || (!queryOk && haveSchemeSplit) {
				u.Host = result
			} else {
				u.Path = result
			}
		}
	}

	if u.Host != "" {
		var userInfo string
		userInfo, host, ok := strings.Cut(u.Host, "@")
		if ok {
			u.Host = host
			if userInfo != "" && host != "" {
				if strings.Contains(userInfo, ":") {
					username, password, _ := strings.Cut(userInfo, ":")
					u.User = url.UserPassword(username, password)
				} else {
					u.User = url.User(userInfo)
				}
			}
		}

		if strings.Contains(u.Host, "?") {
			u.Host, u.RawQuery, _ = strings.Cut(u.Host, "?")
		}
	}

	return u
}

func GetConnectedToHostPortFromHTTPRequest(t *http.Request) (string, error) {
	connectedTo := httpctx.GetContextStringInfoFromRequest(t, httpctx.REQUEST_CONTEXT_KEY_ConnectedTo)
	if connectedTo == "" {
		https, hostport, port, err := generateConnectedToFromHTTPRequest(t)
		if err != nil {
			return "", err
		}
		result := hostport
		//var result string
		//if https {
		//	result = strings.TrimSuffix(hostport, ":443")
		//} else {
		//	result = strings.TrimSuffix(hostport, ":80")
		//}
		httpctx.SetContextValueInfoFromRequest(t, httpctx.REQUEST_CONTEXT_ConnectToHTTPS, https)
		httpctx.SetContextValueInfoFromRequest(t, httpctx.REQUEST_CONTEXT_KEY_ConnectedTo, result)
		httpctx.SetContextValueInfoFromRequest(t, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost, ExtractHost(result))
		httpctx.SetContextValueInfoFromRequest(t, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort, port)
		return result, nil
	}
	return connectedTo, nil
}

func generateConnectedToFromHTTPRequest(t *http.Request) (bool, string, int, error) {
	if t == nil {
		return false, "", 0, Error("nil http request")
	}
	host := t.Host
	if host == "" {
		host = t.URL.Host
	}
	var port int
	var hostname string

	if ret := strings.LastIndex(host, ":"); ret > 0 {
		hostname, port = host[:ret], codec.Atoi(host[ret+1:])
	} else {
		hostname = host
	}

	var https = port == 443
	if t.URL.Scheme != "" {
		if t.URL.Scheme == "https" || t.URL.Scheme == "wss" {
			https = true
		} else {
			https = false
		}
	}

	if port <= 0 {
		if https {
			port = 443
		} else {
			port = 80
		}
	}

	if ret := HostPort(hostname, port); strings.HasPrefix(ret, ":") {
		return false, "", 0, Errorf("invalid host:port(%v) from %v", ret, host)
	} else {
		return https, ret, port, nil
	}
}

func readHTTPRequestFromBufioReader(reader *bufio.Reader, fixContentLength bool, onFirstLine func(string)) (*http.Request, error) {
	rawPacket := new(bytes.Buffer)

	req := &http.Request{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Form:       nil,
		Body:       http.NoBody,
		RequestURI: "", // do not handle it as client
		TLS:        nil,
	}

	defer func() {
		if req != nil && req.URL != nil {
			req.URL.Opaque = ""
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}

		if err := recover(); err != nil {
			log.Errorf("ReadHTTPRequestEx panic: %v", err)
			PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// parse first line
	firstLine, err := BufioReadLine(reader)
	if err != nil {
		return nil, Errorf(`Read Request FirstLine Failed: %s`, err)
	}
	if onFirstLine != nil {
		onFirstLine(string(firstLine))
	}
	rawPacket.Write(firstLine)
	rawPacket.WriteString(CRLF)

	// handle proto
	perfix, firstLine, _ := CutBytesPrefixFunc(firstLine, NotSpaceRune)
	method, uri, proto, ok := ParseHTTPRequestLine(string(firstLine))
	if ok {
		req.Method = method
		req.RequestURI = uri
		req.Proto = proto
		_, after, ok := strings.Cut(proto, "/")
		if ok {
			major, minor, _ := strings.Cut(after, ".")
			req.ProtoMajor, _ = strconv.Atoi(major)
			req.ProtoMinor, _ = strconv.Atoi(minor)
		}
	} else {
		return nil, Errorf(`Parse Request FirstLine(%v) Failed: %s`, strconv.Quote(string(firstLine)), firstLine)
	}

	var (
		// RequestURI > URL > Host in header
		hostInURL    string
		hostInHeader string
	)

	/*
		handle headers
			1. keep gzip
			2. keep chunked if have
		    3. smuggle use max(chunked, contentLength)

		if smuggle { keep cl and te }
		if not smuggle { if te keep te }
	*/
	// close is default in 0.9 or 1.0
	defaultClose := (req.ProtoMajor == 1 && req.ProtoMinor == 0) || req.ProtoMajor < 1
	header := make(http.Header)
	useContentLength := false
	contentLengthInt := 0
	useTransferEncodingChunked := false

	_ = ScanHTTPHeader(reader, func(lineBytes []byte) {
		if len(lineBytes) == 0 {
			rawPacket.WriteString(CRLF)
			return
		}
		rawPacket.Write(lineBytes)
		rawPacket.WriteString(CRLF)

		before, after, _ := bytes.Cut(lineBytes, []byte{':'})
		keyStr := string(before)
		valStr := strings.TrimLeftFunc(string(after), unicode.IsSpace)

		if _, isCommonHeader := commonHeader[keyStr]; isCommonHeader {
			keyStr = http.CanonicalHeaderKey(keyStr)
		}

		isSingletonHeader := false
		switch strings.ToLower(keyStr) {
		case "content-length":
			useContentLength = true
			contentLengthInt = codec.Atoi(valStr)
			if contentLengthInt != 0 || !ShouldRemoveZeroContentLengthHeader(method) {
				header[keyStr] = []string{valStr}
				req.ContentLength = int64(contentLengthInt)
			}
		case "host":
			hostInHeader = valStr
		case "content-type":
			isSingletonHeader = true
		case `transfer-encoding`:
			req.TransferEncoding = []string{valStr}
			if IContains(valStr, "chunked") {
				useTransferEncodingChunked = true
			}
		case "connection":
			if strings.EqualFold(valStr, "close") {
				defaultClose = true
			} else if strings.EqualFold(valStr, "keep-alive") {
				defaultClose = false
			}
		}

		// add header
		if keyStr == "" {
			return
		}
		if isSingletonHeader {
			header[keyStr] = append(header[keyStr], valStr)
			return
		}
		header[keyStr] = append(header[keyStr], valStr)
	}, perfix, false)

	// uri is very complex
	// utf8 valid or not
	before, fragment, haveFragment := strings.Cut(req.RequestURI, "#")
	var urlIns *url.URL
	if method == "CONNECT" {
		urlIns = new(url.URL)
		// if connect, the uri should be host:port
		host, port, _ := ParseStringToHostPort(before)
		if port > 0 {
			urlIns.Host = HostPort(host, port)
		} else {
			if strings.HasPrefix(hostInHeader, ":") {
				port := codec.Atoi(hostInHeader[1:])
				if port > 0 && port <= 65535 {
					urlIns.Host = HostPort(host, port)
				} else {
					urlIns.Host = strings.Trim(host, "/")
				}
			} else {
				urlIns.Host = strings.Trim(host, "/")
			}
		}
	} else if urlIns, _ = url.ParseRequestURI(before); urlIns == nil {
		// remove : begin
		// utf8 invalid
		urlIns = new(url.URL)
		if IsHttpOrHttpsUrl(req.RequestURI) {
			urlIns, err = url.Parse(req.RequestURI)
			if err != nil {
				return nil, Errorf("parse uri-url (%v) failed: %s", req.RequestURI, err)
			}
		} else {
			urlIns.Path, urlIns.RawQuery, _ = strings.Cut(req.RequestURI, "?")
		}
	}

	if urlIns != nil && haveFragment {
		urlIns.Fragment = fragment
	}
	req.URL = urlIns

	// handle host
	hostInURL = req.URL.Host
	if ret := strings.LastIndex(hostInURL, ":"); ret >= 0 {
		hostname, portStr := strings.TrimSpace(hostInURL[:ret]), codec.Atoi(hostInURL[ret+1:])
		if hostname == "" || portStr == 0 {
			req.URL.Host = ""
			hostInURL = ""
		}
	}

	req.Close = defaultClose
	req.Header = header

	// handling host
	if hostInHeader == "" && hostInURL == "" && method == "CONNECT" {
		return nil, Error(`Host(inHeader/inURL) is empty in CONNECT method`)
	}

	var host string
	if hostInURL != "" {
		host = hostInURL
	} else {
		host = hostInHeader
	}
	req.Host = host
	if req.URL.Host == "" {
		req.URL.Host = hostInHeader
	}
	bodyRawBuf := new(bytes.Buffer)
	if fixContentLength {
		// by reader
		raw, _ := io.ReadAll(reader)
		rawPacket.Write(raw)
		if useContentLength && !useTransferEncodingChunked {
			req.ContentLength = int64(len(raw))
			shrinkHeader(req.Header, "content-length")
			req.Header.Set("Content-Length", strconv.Itoa(len(raw)))
		}
		bodyRawBuf.Write(raw)
	} else {
		// by header
		if useContentLength && useTransferEncodingChunked {
			log.Warn("content-length and transfer-encoding chunked both exist, try smuggle? use content-length first!")
			if contentLengthInt > 0 {
				// smuggle
				bodyRaw, _ := io.ReadAll(io.NopCloser(io.LimitReader(reader, int64(contentLengthInt))))
				rawPacket.Write(bodyRaw)
				bodyRawBuf.Write(bodyRaw)
				if ret := contentLengthInt - len(bodyRaw); ret > 0 {
					bodyRawBuf.WriteString(strings.Repeat("\n", ret))
				}
			} else {
				// chunked
				_, fixed, _, err := codec.HTTPChunkedDecoderWithRestBytes(reader)
				rawPacket.Write(fixed)
				if err != nil {
					return nil, Errorf("chunked decoder error: %v", err)
				}
				bodyRawBuf.Write(fixed)
			}
		} else if !useContentLength && useTransferEncodingChunked {
			// handle chunked
			_, fixed, _, err := codec.HTTPChunkedDecoderWithRestBytes(reader)
			rawPacket.Write(fixed)
			if err != nil {
				return nil, Errorf("chunked decoder error: %v", err)
			}
			if len(fixed) > 0 {
				bodyRawBuf.Write(fixed)
			}
		} else {
			// handle content-length as default
			bodyRaw, err := io.ReadAll(io.NopCloser(io.LimitReader(reader, int64(contentLengthInt))))
			rawPacket.Write(bodyRaw)
			if err != nil && err != io.EOF {
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					return nil, Errorf("read body error: %v", err)
				}
				log.Warnf("read body error: %v", err)
			}
			bodyLen := len(bodyRaw)
			bodyRawBuf.Write(bodyRaw)
			bodyRawBuf.WriteString(strings.Repeat("\n", contentLengthInt-bodyLen))
		}
	}
	if bodyRawBuf.Len() == 0 {
		req.Body = http.NoBody
	} else {
		req.Body = io.NopCloser(bodyRawBuf)
	}
	if req.URL != nil && req.URL.Host != "" {
		req.Host = req.URL.Host
	}
	httpctx.SetBareRequestBytes(req, rawPacket.Bytes())
	return req, nil
}

// FixHTTPRequestForGolangNativeHTTPClient
// utils.Read/DumpRequest is working as pair...
// if u want to use transport(golang native)
// do this `FixHTTPRequestForGolangNativeHTTPClient` helps
// because golang native transport will encode chunked body again
func FixHTTPRequestForGolangNativeHTTPClient(req *http.Request) {
	if req == nil {
		return
	}
	if StringArrayContains(req.TransferEncoding, "chunked") && req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		result, rest := codec.HTTPChunkedDecodeWithRestBytes(body)
		if len(result) > 0 {
			req.Body = io.NopCloser(bytes.NewReader(result))
		} else {
			req.Body = io.NopCloser(bytes.NewReader(rest))
		}
	}
}

func FixHTTPResponseForGolangNativeHTTPClient(ins *http.Response) {
	if ins == nil {
		return
	}
	if StringArrayContains(ins.TransferEncoding, "chunked") && ins.Body != nil {
		body, _ := io.ReadAll(ins.Body)
		ins.Body = io.NopCloser(bytes.NewReader(body))
	}
}
