package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var _noContentLengthHeader = map[string]bool{
	"GET":     true,
	"HEAD":    true,
	"DELETE":  true,
	"OPTIONS": true,
	"CONNECT": true,
	"get":     true,
	"head":    true,
	"delete":  true,
	"options": true,
	"connect": true,
}

func ShouldRemoveZeroContentLengthHeader(s string) bool {
	_, ok := _noContentLengthHeader[s]
	return ok
}

const CRLF = "\r\n"

func getHeaderValueAll(header http.Header, key string) string {
	return strings.Join(getHeaderValueList(header, key), ", ")
}

func getHeaderValue(header http.Header, key string) string {
	vals := getHeaderValueList(header, key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func getHeaderValueList(header http.Header, key string) []string {
	if header == nil {
		return nil
	}
	cKey := http.CanonicalHeaderKey(key)
	if key == cKey {
		return []string{header.Get(key)}
	}

	v1, _ := header[key]
	v2, _ := header[cKey]
	vals := make([]string, 0, len(v1)+len(v2))
	var m = map[string]any{}
	for _, v := range [][]string{v1, v2} {
		for _, i := range v {
			if i == "" {
				continue
			}
			if _, ok := m[i]; ok {
				continue
			}
			m[i] = i
			vals = append(vals, i)
		}
	}
	return vals
}

func shrinkHeader(header http.Header, key string) {
	values := getHeaderValueList(header, key)
	delete(header, key)
	delete(header, http.CanonicalHeaderKey(key))
	if len(values) > 0 {
		header[http.CanonicalHeaderKey(key)] = values
	}
}

// DumpHTTPResponse dumps http response to bytes
// if loadBody is true, it will load body to memory
//
// transfer-encoding is a special header
func DumpHTTPResponse(rsp *http.Response, loadBody bool, wr ...io.Writer) ([]byte, error) {
	if rsp == nil {
		return nil, Error("nil response")
	}

	var (
		transferEncodingChunked bool
		contentLengthExisted    bool
		contentLengthInt        int64
	)

	// handle transfer-encoding
	if len(rsp.TransferEncoding) > 0 {
		for _, v := range rsp.TransferEncoding {
			if v == "chunked" {
				transferEncodingChunked = true
				break
			}
		}
	}
	if !transferEncodingChunked {
		if ret := getHeaderValue(rsp.Header, "transfer-encoding"); ret != "" {
			if strings.Contains(ret, "chunked") {
				transferEncodingChunked = true
			}
		}
	}

	// handle content-length
	if rsp.ContentLength > 0 {
		contentLengthExisted = true
		contentLengthInt = rsp.ContentLength
	} else {
		if ret := getHeaderValue(rsp.Header, "content-length"); ret != "" {
			contentLengthExisted = true
			rsp.ContentLength = int64(codec.Atoi(ret))
			contentLengthInt = rsp.ContentLength
		}
	}

	var cacheBuf = new(bytes.Buffer)
	var wrs = make([]io.Writer, 0, len(wr)+1)
	wrs = append(wrs, cacheBuf)
	wrs = append(wrs, wr...)

	var buf = bufio.NewWriter(io.MultiWriter(wrs...))

	// handle proto
	protoRaw := rsp.Proto
	if rsp.ProtoMajor <= 0 && rsp.ProtoMinor <= 0 {
		rsp.ProtoMajor = 1
		rsp.ProtoMinor = 1
	}
	if protoRaw == "" {
		protoRaw = fmt.Sprintf("HTTP/%d.%d", rsp.ProtoMajor, rsp.ProtoMinor)
	}
	buf.WriteString(protoRaw)
	buf.WriteString(" ")
	if rsp.Status == "" {
		if rsp.StatusCode <= 0 {
			rsp.StatusCode = 200
			rsp.Status = "200 OK"
		} else {
			rsp.Status = fmt.Sprintf("%d %s", rsp.StatusCode, http.StatusText(rsp.StatusCode))
		}
	}
	buf.WriteString(rsp.Status)
	buf.WriteString(CRLF)
	buf.Flush()

	// handle server first
	shrinkHeader(rsp.Header, "server")
	if ret := rsp.Header.Get("server"); ret != "" {
		rsp.Header.Set("Server", ret)
		buf.WriteString("Server: ")
		buf.WriteString(ret)
		buf.WriteString(CRLF)
		buf.Flush()
	}

	shrinkHeader(rsp.Header, "content-length")
	for k := range rsp.Header {
		switch strings.ToLower(k) {
		case "transfer-encoding", "content-length", "server":
			continue
		}
		buf.WriteString(k)
		buf.WriteString(": ")
		buf.WriteString(getHeaderValueAll(rsp.Header, k))
		buf.WriteString(CRLF)
	}

	buf.Flush()
	if rsp.Body == nil {
		rsp.Body = http.NoBody
	}

	rawBody, _ := io.ReadAll(rsp.Body)
	var backupBody = io.NopCloser(bytes.NewReader(rawBody))
	defer func() {
		rsp.Body = backupBody
	}()
	haveBody := len(rawBody) > 0
	if transferEncodingChunked {
		rsp.ContentLength = -1 // unknown
		buf.WriteString("Transfer-Encoding: chunked\r\n")
		buf.Flush()
		if haveBody {
			decode, fixed, _ := codec.ReadHTTPChunkedDataWithFixed(rawBody)
			if len(decode) == 0 {
				rawBody = codec.HTTPChunkedEncode(rawBody)
			} else {
				rawBody = fixed
			}
		}
	} else {
		// handle content-length
		if haveBody || contentLengthExisted {
			rsp.ContentLength = int64(len(rawBody))
			contentLengthInt = rsp.ContentLength
			buf.WriteString("Content-Length: ")
			buf.WriteString(strconv.FormatInt(contentLengthInt, 10))
			buf.WriteString(CRLF)
			buf.Flush()
		}
	}

	buf.WriteString(CRLF)
	if loadBody {
		buf.Write(rawBody)
	}
	buf.Flush()
	return cacheBuf.Bytes(), nil
}

// DumpHTTPRequest dumps http request to bytes
// **NO NOT HANDLE SMUGGLE HERE!**
// Transfer-Encoding is handled vai req.TransferEncoding / req.Header["Transfer-Encoding"]
// Content-Length is handled vai req.ContentLength / req.Header["Content-Length"]
// if Transfer-Encoding existed, check body chunked? if not, encode it
// if Transfer-Encoding and Content-Length existed at same time, use transfer-encoding
func DumpHTTPRequest(req *http.Request, loadBody bool) ([]byte, error) {
	if req == nil {
		return nil, Error("nil request")
	}
	var (
		h2                      bool
		transferEncodingChunked bool
		contentLengthExisted    bool
		contentLengthInt        int64
	)
	_ = contentLengthInt
	if len(req.TransferEncoding) > 0 {
		for _, v := range req.TransferEncoding {
			if v == "chunked" {
				transferEncodingChunked = true
				break
			}
		}
	}
	if !transferEncodingChunked {
		if ret := getHeaderValue(req.Header, "transfer-encoding"); ret != "" {
			if strings.Contains(ret, "chunked") {
				transferEncodingChunked = true
			}
		}
	}

	if req.ProtoMajor == 2 || strings.HasPrefix(req.Proto, "HTTP/2") {
		h2 = true
	}

	if ret := getHeaderValue(req.Header, "content-length"); ret != "" || req.ContentLength > 0 {
		contentLengthExisted = true
		if ret != "" {
			contentLengthInt = int64(codec.Atoi(ret))
		} else {
			contentLengthInt = req.ContentLength
		}
	}

	var buf bytes.Buffer
	buf.WriteString(req.Method)
	buf.WriteString(" ")
	if req.RequestURI == "" {
		buf.WriteString(req.URL.RequestURI())
	} else {
		buf.WriteString(req.RequestURI)
	}
	buf.WriteString(" ")
	if h2 {
		req.Proto = "HTTP/2.0"
	} else {
		req.Proto = fmt.Sprint("HTTP/", req.ProtoMajor, ".", req.ProtoMinor)
	}
	buf.WriteString(req.Proto)
	buf.WriteString(CRLF)

	// handle host
	buf.WriteString("Host: ")
	if ret := getHeaderValue(req.Header, "host"); ret == "" {
		if req.Host != "" {
			buf.WriteString(req.Host)
		} else if req.URL.Host != "" {
			buf.WriteString(req.URL.Host)
		}
	} else {
		buf.WriteString(ret)
	}
	buf.WriteString(CRLF)
	shrinkHeader(req.Header, "content-type")

	for k := range req.Header {
		switch strings.ToLower(k) {
		case "host", "content-length", "transfer-encoding":
			continue
		}
		val := getHeaderValueAll(req.Header, k)
		buf.WriteString(k)
		buf.WriteString(": ")
		buf.WriteString(val)
		buf.WriteString(CRLF)
	}

	if req.Body == nil {
		req.Body = http.NoBody
	}
	rawBody, _ := io.ReadAll(req.Body)
	var backupBody = io.NopCloser(bytes.NewReader(rawBody))
	defer func() {
		req.Body = backupBody
	}()

	haveBody := len(rawBody) > 0
	// handle cl / te
	if transferEncodingChunked {
		buf.WriteString("Transfer-Encoding: chunked\r\n")
		// check body is chunked or not
		// if not, encode it
		if haveBody {
			decoded, fixed, _ := codec.ReadHTTPChunkedDataWithFixed(rawBody)
			if len(decoded) == 0 {
				rawBody = codec.HTTPChunkedEncode(rawBody)
			} else {
				rawBody = fixed
			}
		}
	} else {
		if haveBody || !ShouldRemoveZeroContentLengthHeader(req.Method) || contentLengthExisted {
			buf.WriteString("Content-Length: ")
			buf.WriteString(fmt.Sprint(len(rawBody)))
			buf.WriteString(CRLF)
		}
	}

	buf.WriteString(CRLF)
	if loadBody {
		buf.Write(rawBody)
	}
	return buf.Bytes(), nil
}
