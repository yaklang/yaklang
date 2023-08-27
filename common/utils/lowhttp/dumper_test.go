package lowhttp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"testing"
)

func TestNativeDumper(t *testing.T) {
	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		panic(err)
	}
	req.Header["content-type"] = []string{"application/json"}
	req.Body = io.NopCloser(bytes.NewBuffer([]byte(`{"test": "test"}`)))
	raw, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(raw))
	fmt.Println(string(FixHTTPRequestOut(raw)))
}
