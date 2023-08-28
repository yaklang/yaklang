package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
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

	var httpVersion = "HTTP/1.1"
	if s2 < 0 {
		return line[:s1], line[s1+1:], httpVersion, true
	}
	s2 += s1 + 1
	return line[:s1], line[s1+1 : s2], line[s2+1:], true
}

func ReadHTTPRequestFromReader(reader *bufio.Reader) (*http.Request, error) {
	var rawPacket = new(bytes.Buffer)

	var req = &http.Request{
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
	rawPacket.Write(firstLine)
	rawPacket.WriteString(CRLF)

	// handle proto
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

	// uri is very complex
	// utf8 valid or not
	if strings.Contains(uri, "://") && method == "CONNECT" {
		fmt.Println("DEBUG")
	}
	before, fragment, _ := strings.Cut(req.RequestURI, "#")
	urlIns, _ := url.ParseRequestURI(before)
	if urlIns == nil {
		// remove : begin
		// utf8 invalid
		urlIns = new(url.URL)
		if method == "CONNECT" {
			urlIns.Host = before
		} else {
			var after = req.RequestURI
			if IsHttpOrHttpsUrl(req.RequestURI) {
				var schemaRaw, rest, ok = strings.Cut(req.RequestURI, "://")
				if ok {
					if strings.Contains(schemaRaw, ".") {
						fmt.Println("DEBUG")
					}
					urlIns.Scheme = schemaRaw
					after = rest
				}
			}
			if strings.HasPrefix(after, "/") {
				urlIns.Path, urlIns.RawQuery, _ = strings.Cut(after, "?")
			} else if strings.Contains(after, "/") {
				var hostraw, after, _ = strings.Cut(after, "/")
				after = "/" + after
				if strings.Contains(hostraw, "@") {
					var userinfo, hostport string
					userinfo, hostport, _ = strings.Cut(hostraw, "@")
					urlIns.User = url.UserPassword(userinfo, "")
					urlIns.Host = hostport
				} else {
					urlIns.Host = hostraw
				}
				urlIns.Path, urlIns.RawQuery, _ = strings.Cut(after, "?")
			} else {
				urlIns.Path, urlIns.RawQuery, _ = strings.Cut(after, "?")
			}
		}
	}
	if urlIns != nil {
		urlIns.Fragment = fragment
	}
	req.URL = urlIns

	/*
		handle headers
			1. keep gzip
			2. keep chunked if have
		    3. smuggle use max(chunked, contentLength)

		if smuggle { keep cl and te }
		if not smuggle { if te keep te }
	*/
	// close is default in 0.9 or 1.0
	var defaultClose = (req.ProtoMajor == 1 && req.ProtoMinor == 0) || req.ProtoMajor < 1
	var header = make(http.Header)
	var useContentLength = false
	var contentLengthInt = 0
	var useTransferEncodingChunked = false
	for {
		lineBytes, err := BufioReadLine(reader)
		if err != nil {
			return nil, Errorf(`Read Request Header Failed: %s`, err)
		}
		rawPacket.Write(lineBytes)
		rawPacket.WriteString(CRLF)

		if len(bytes.TrimSpace(lineBytes)) == 0 {
			rawPacket.WriteString(CRLF)
			break
		}

		before, after, _ := bytes.Cut(lineBytes, []byte{':'})
		keyStr := string(before)
		valStr := strings.TrimLeftFunc(string(after), unicode.IsSpace)

		if _, isCommonHeader := commonHeader[keyStr]; isCommonHeader {
			keyStr = http.CanonicalHeaderKey(keyStr)
		}

		var isSingletonHeader = false
		switch strings.ToLower(keyStr) {
		case "content-length":
			useContentLength = true
			contentLengthInt = codec.Atoi(valStr)
			if contentLengthInt != 0 || !ShouldRemoveZeroContentLengthHeader(method) {
				header.Set(keyStr, valStr)
			}
		case "content-type":
			isSingletonHeader = true
		case `transfer-encoding`:
			req.TransferEncoding = []string{valStr}
			if strings.EqualFold(valStr, "chunked") {
				useTransferEncodingChunked = true
			}
		case "host":
			req.Host = valStr
		case "connection":
			if strings.EqualFold(valStr, "close") {
				defaultClose = true
			} else if strings.EqualFold(valStr, "keep-alive") {
				defaultClose = false
			}
		}

		// add header
		if keyStr == "" {
			continue
		}
		if isSingletonHeader {
			header.Set(keyStr, valStr)
			continue
		}
		if firstCap := keyStr[0]; 'A' <= firstCap && firstCap <= 'Z' {
			header.Add(keyStr, valStr)
		} else {
			header[keyStr] = append(header[keyStr], valStr)
		}
	}
	req.Close = defaultClose
	req.Header = header

	// handle body
	var bodyRawBuf = new(bytes.Buffer)
	if useContentLength && useTransferEncodingChunked {
		log.Debug("content-length and transfer-encoding chunked both exist, try smuggle? use content-length first!")
		if contentLengthInt > 0 {
			// smuggle
			bodyRaw, _ := io.ReadAll(io.NopCloser(io.LimitReader(reader, int64(contentLengthInt))))
			bodyRawBuf.Write(bodyRaw)
		} else {
			// chunked
			_, fixed, _, err := codec.HTTPChunkedDecoderWithRestBytes(reader)
			if err != nil {
				return nil, Errorf("chunked decoder error: %v", err)
			}
			bodyRawBuf.Write(fixed)
		}
	} else if !useContentLength && useTransferEncodingChunked {
		// handle chunked
		_, fixed, _, err := codec.HTTPChunkedDecoderWithRestBytes(reader)
		if err != nil {
			return nil, Errorf("chunked decoder error: %v", err)
		}
		if len(fixed) > 0 {
			bodyRawBuf.Write(fixed)
		}
	} else {
		// handle content-length as default
		bodyRaw, _ := io.ReadAll(io.NopCloser(io.LimitReader(reader, int64(contentLengthInt))))
		bodyRawBuf.Write(bodyRaw)
	}

	rawPacket.Write(bodyRawBuf.Bytes())

	if bodyRawBuf.Len() == 0 {
		req.Body = http.NoBody
	} else {
		req.Body = io.NopCloser(bodyRawBuf)
	}
	if req.URL != nil && req.URL.Host != "" {
		req.Host = req.URL.Host
	}
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
		result, rest := codec.HTTPChunkedDecodeWithRestBytes(body)
		if len(result) > 0 {
			ins.Body = io.NopCloser(bytes.NewReader(result))
		} else {
			ins.Body = io.NopCloser(bytes.NewReader(rest))
		}
	}
}
