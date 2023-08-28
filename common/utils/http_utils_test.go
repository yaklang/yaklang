package utils

import (
	"bufio"
	"bytes"
	"github.com/k0kubun/pp"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMarshalHTTPRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`test`))
		pp.Println(r.Header)
	}))
	defer ts.Close()

	_ = ts.URL

	rsp, err := http.Get(ts.URL)
	if err != nil {
		t.Errorf("request: %s failed: %s", ts.URL, err)
		t.FailNow()
	}

	pp.Println(rsp.Request)

	req, err := MarshalHTTPRequest(rsp.Request)
	if err != nil {
		t.Errorf("parse request failed: %s", err)
		t.FailNow()
	}

	t.Logf("data: %s", string(req))

	_, err = ReadHTTPRequestFromReader(bufio.NewReader(bytes.NewBuffer(req)))
	if err != nil {
		t.Errorf("marshal http request for re-building request failed: %s", err)
		t.FailNow()
	}
}
