package fuzzx

import (
	"fmt"

	"golang.org/x/exp/slices"
)

type FuzzRequest struct {
	origin   []byte
	requests [][]byte
}


func NewFuzzHTTPRequest(raw []byte) *FuzzRequest {
	f := &FuzzRequest{
		origin:   raw,
		requests: make([][]byte, 0),
	}
	return f
}

func (f *FuzzRequest) Clone() *FuzzRequest {
	return &FuzzRequest{
		requests:  slices.Clone(f.requests), // shadow copy
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
