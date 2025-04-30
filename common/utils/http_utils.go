package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/log"

	"github.com/pkg/errors"
)

func GetHTTPHeader(headers http.Header, key string) string {
	if v := headers.Get(key); len(v) > 0 {
		return v
	}
	if values := headers[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func MarshalHTTPRequest(req *http.Request) ([]byte, error) {
	if req == nil {
		return nil, errors.New("request is empty")
	}
	var (
		raw  []byte
		path string
	)

	if !strings.HasPrefix(req.URL.Path, "/") {
		path = "/" + path
	}

	raw = append(raw, []byte(fmt.Sprintf("%s %s %s\r\n", req.Method, path, req.Proto))...)

	for key, values := range req.Header {
		for _, value := range values {
			raw = append(raw, []byte(fmt.Sprintf("%s: %s\r\n", key, value))...)
		}
	}

	req.BasicAuth()

	raw = append(raw, []byte("\r\n")...)
	if req.Body == nil {
		return raw, nil
	}

	data, err := ioutil.ReadAll(req.Body)
	if err != nil || len(data) == 0 {
		return raw, nil
	}

	return append(raw, data...), nil
}

func HttpDumpWithBody(i interface{}, body bool) ([]byte, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("HttpDumpWithBody panic: %v", err)
			PrintCurrentGoroutineRuntimeStack()
		}
	}()
	switch ret := i.(type) {
	case *http.Request:
		ret.Close = false
		return DumpHTTPRequest(ret, body)
	case http.Request:
		return HttpDumpWithBody(&ret, body)
	case *http.Response:
		return DumpHTTPResponse(ret, body)
	case http.Response:
		return HttpDumpWithBody(&ret, body)
	default:
		return nil, Errorf("error type for http.dump, Type: [%v]", reflect.TypeOf(i))
	}
}

func HttpShow(i interface{}) []byte {
	rsp, err := HttpDumpWithBody(i, true)
	if err != nil {
		log.Errorf("show failed: %s", err)
		return nil
	}
	fmt.Println(string(rsp))
	return rsp
}

func Url2UnEscapeString(u *url.URL) string {
	buffer := bytes.NewBuffer(nil)
	if u.Scheme != "" {
		buffer.WriteString(u.Scheme)
		buffer.WriteByte(':')
	}
	if u.Scheme != "" || u.Host != "" || u.User != nil {
		if u.OmitHost && u.Host == "" && u.User == nil {
			// omit empty host
		} else {
			if u.Host != "" || u.Path != "" || u.User != nil {
				buffer.WriteString("//")
			}
			if ui := u.User; ui != nil {
				buffer.WriteString(ui.String())
				buffer.WriteByte('@')
			}
			if h := u.Host; h != "" {
				buffer.WriteString(h)
			}
		}
	}
	path := u.Path
	if path != "" && path[0] != '/' && u.Host != "" {
		buffer.WriteByte('/')
	}
	if buffer.Len() == 0 {
		// RFC 3986 §4.2
		// A path segment that contains a colon character (e.g., "this:that")
		// cannot be used as the first segment of a relative-path reference, as
		// it would be mistaken for a scheme name. Such a segment must be
		// preceded by a dot-segment (e.g., "./this:that") to make a relative-
		// path reference.
		if segment, _, _ := strings.Cut(path, "/"); strings.Contains(segment, ":") {
			buffer.WriteString("./")
		}
	}
	buffer.WriteString(path)
	if u.ForceQuery || u.RawQuery != "" {
		buffer.WriteByte('?')
		buffer.WriteString(u.RawQuery)
	}
	if u.Fragment != "" {
		buffer.WriteByte('#')
		buffer.WriteString(u.Fragment)
	}
	return buffer.String()
}
