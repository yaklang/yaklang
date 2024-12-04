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
