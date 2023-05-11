package mutate

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestGETJSON(t *testing.T) {
	req, err := NewFuzzHTTPRequest(`GET /?a={"c":1} HTTP/1.1
Host: 127.0.0.1:8787

`)
	if err != nil {
		panic(err)
	}

	req.Show()

	checked := false
	for _, r := range req.GetCommonParams() {
		if r.IsGetValueJSON() {
			req, err := r.Fuzz("abc").Show().GetFirstFuzzHTTPRequest()
			if err != nil {
				panic(err)
			}
			spew.Dump(req.GetBytes())
			reqRaw := req.GetBytes()
			_ = reqRaw
		}
	}
	if !checked {
		panic(1)
	}
}
