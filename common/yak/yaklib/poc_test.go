package yaklib

import (
	"net/http/httputil"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestPoC(t *testing.T) {
	rsp, req, err := pocHTTP(`GET / HTTP/1.1
Host: dppt98.guangdong.chinatax.gov.cn:8443

`)
	spew.Dump(rsp, req, err)
}

func TestPoCmethod(t *testing.T) {
	// remove useragent
	_, req, err := do("GET", "https://baidu.com/", _pocOptDeleteHeader("User-Agent"))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := httputil.DumpRequest(req, true)
	if err != nil {
		t.Fatal(err)
	}
	wanted := lowhttp.FixHTTPPacketCRLF([]byte(`GET / HTTP/1.1
Host: baidu.com
`), false)

	if string(actual) != string(wanted) {
		t.Fatalf("actual: %s, wanted: %s", actual, wanted)
	}
}
