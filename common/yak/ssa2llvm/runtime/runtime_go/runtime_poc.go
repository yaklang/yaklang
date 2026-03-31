package main

import (
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func runtimePocTimeout(timeout int64) any {
	return poc.WithTimeout(float64(timeout))
}

func runtimePocGet(url string, opt any) any {
	opts := make([]poc.PocConfigOption, 0, 1)
	if actual, ok := opt.(poc.PocConfigOption); ok {
		opts = append(opts, actual)
	}
	rsp, req, err := poc.DoGET(url, opts...)
	return []any{rsp, req, err}
}

func runtimePocGetHTTPPacketBody(packet []byte) []byte {
	return lowhttp.GetHTTPPacketBody(packet)
}
