package mutate

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func execMutateViaMethods(ins *ypb.MutateMethod, req []byte) [][]byte {
	freq, err := NewFuzzHTTPRequest(req)
	if err != nil {
		log.Errorf("NewFuzzHTTPRequest failed: %s", err)
		return [][]byte{req}
	}
	var results [][]byte
	switch strings.TrimSpace(strings.ToLower(ins.Type)) {
	case "cookie":
		for _, value := range ins.Value {
			freq.FuzzCookie(value.Key, value.Value).RequestMap(func(bytes []byte) {
				results = append(results, bytes)
			})
		}
	case "header":
		for _, value := range ins.Value {
			freq.FuzzHTTPHeader(value.Key, value.Value).RequestMap(func(bytes []byte) {
				results = append(results, bytes)
			})
		}
	case "get":
		for _, value := range ins.Value {
			freq.FuzzGetParams(value.Key, value.Value).RequestMap(func(bytes []byte) {
				results = append(results, bytes)
			})
		}
	case "post":
		for _, value := range ins.Value {
			freq.FuzzPostParams(value.Key, value.Value).RequestMap(func(bytes []byte) {
				results = append(results, bytes)
			})
		}
	}
	return results
}

func RequestMutateViaMethods(methods ...*ypb.MutateMethod) func([]byte) [][]byte {
	return func(origin []byte) [][]byte {
		var results [][]byte
		for _, method := range methods {
			results = append(results, execMutateViaMethods(method, origin)...)
		}
		if len(results) > 0 {
			return results
		}
		return [][]byte{origin}
	}
}

func WithPoolOpt_MutateViaMethods(methods ...*ypb.MutateMethod) HttpPoolConfigOption {
	return func(c *httpPoolConfig) {
		if c.MutateHook == nil {
			c.MutateHook = RequestMutateViaMethods(methods...)
		} else {
			origin := c.MutateHook
			c.MutateHook = func(bytes []byte) [][]byte {
				var results [][]byte
				for _, result := range origin(bytes) {
					results = append(results, result)
				}
				for _, result := range RequestMutateViaMethods(methods...)(bytes) {
					results = append(results, result)
				}
				return results
			}
		}
	}
}
