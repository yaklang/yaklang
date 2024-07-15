package utils

import (
	"fmt"
	"io/ioutil"
	"net/http"
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
