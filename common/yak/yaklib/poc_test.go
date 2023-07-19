package yaklib

import (
	"net/http"
	"net/http/httputil"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestPoC(t *testing.T) {
	_, _, err := pocHTTP(`GET / HTTP/1.1
Host: example.com
	
`)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = pocHTTPEx(req)
	if err != nil {
		t.Fatal(err)
	}
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
