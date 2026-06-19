package fuzzx

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

type FuzzRequest struct {
	origin   []byte
	requests [][]byte
}

// NewRequest 根据原始请求报文构造一个 fuzzx 请求对象，用于新一代的 HTTP 请求变形与批量发包
// 参数:
//   - raw: 原始 HTTP 请求报文字节数组
//
// 返回值:
//   - 构造好的 fuzzx 请求对象
//   - 错误信息，报文非法时返回非空
//
// Example:
// ```
// raw = []byte(`GET / HTTP/1.1
// Host: www.example.com
//
// `)
// freq = fuzzx.NewRequest(raw)~
// freq.FuzzPath("/a", "/b").Show()
// ```
func NewFuzzHTTPRequest(raw []byte) (*FuzzRequest, error) {
	_, err := utils.ReadHTTPRequestFromBytes(raw)
	if err != nil {
		return nil, utils.Wrap(err, "NewFuzzHTTPRequest error: invalid http request")
	}
	f := &FuzzRequest{
		origin:   raw,
		requests: make([][]byte, 0),
	}
	return f, nil
}

// MustNewRequest 根据原始请求报文构造一个 fuzzx 请求对象，报文非法时直接 panic，便于在确定输入合法时简化调用
// 参数:
//   - raw: 原始 HTTP 请求报文字节数组
//
// 返回值:
//   - 构造好的 fuzzx 请求对象
//
// Example:
// ```
// raw = []byte(`GET / HTTP/1.1
// Host: www.example.com
//
// `)
// freq = fuzzx.MustNewRequest(raw)
// freq.FuzzPath("/a", "/b").Show()
// ```
func MustNewFuzzHTTPRequest(raw []byte) *FuzzRequest {
	f, err := NewFuzzHTTPRequest(raw)
	if err != nil {
		panic(err)
	}
	return f
}

func (f *FuzzRequest) Clone() *FuzzRequest {
	return &FuzzRequest{
		origin:   f.origin,
		requests: slices.Clone(f.requests), // shadow copy
	}
}

func (f *FuzzRequest) Results() [][]byte {
	return f.requests
}

func (f *FuzzRequest) Show() *FuzzRequest {
	for _, req := range f.requests {
		fmt.Println(string(req))
	}
	return f
}

func (f *FuzzRequest) FirstFuzzRequestBytes() []byte {
	if len(f.requests) > 0 {
		return f.requests[0]
	} else {
		return nil
	}
}
