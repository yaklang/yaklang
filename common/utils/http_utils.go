package utils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

func HttpGetWithRetry(retry int, url string) ([]byte, error) {
	var e error
	for ; retry > 0; retry-- {
		b, err := HttpGet(url)
		if err == nil {
			return b, nil
		} else {
			e = err
			continue
		}
	}
	return nil, e
}

func HttpGet(url string) ([]byte, error) {

	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Errorf("HTTP GET %s error: %s", url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Errorf("read response body error: %s", body)
	}
	return body, nil
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
		if retBytes := httpctx.GetBareRequestBytes(ret); len(retBytes) > 0 {
			return retBytes, nil
		}
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
