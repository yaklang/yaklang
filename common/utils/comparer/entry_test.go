package comparer

import (
	"testing"
	"yaklang/common/mutate"
)

func TestCompareHTTPResponseRaw(t *testing.T) {
	f, err := mutate.NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.baidu.com

`)
	if err != nil {
		panic(err)
	}

	r1, err := f.ExecFirst()
	if err != nil {
		panic(err)
	}

	r2, err := f.FuzzPath("/123123123").FuzzPostRaw("asdfasdfasdfasdf").ExecFirst()
	if err != nil {
		panic(err)
	}

	score := CompareHTTPResponseRaw(r1.ResponseRaw, r2.ResponseRaw)
	println(score)
}

func TestCompareHTTPResponseRaw2(t *testing.T) {
	p1 := []byte(`HTTP/1.1 200 Ok
asdf: 123123
sadfasdf: as
dfasdf: 123123
Content-Type: application/json

{"key": 1, "va": [1,2,3], "asdfasdf": 12311}
`)
	p2 := []byte(`HTTP/1.1 200 Ok
asdf: 123123
sadfasdf: as
dfasdf: 123123
Content-Type: application/json

{"key": 1, "va": [3,                1,"adasdfa",2], "asdfasdf": 12311}
`)
	score := CompareHTTPResponseRaw(p1, p2)
	println(score)
}
