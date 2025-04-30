package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	pp "github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	_, err = ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewBuffer(req)))
	if err != nil {
		t.Errorf("marshal http request for re-building request failed: %s", err)
		t.FailNow()
	}
}

func TestCtxEffectReader(t *testing.T) {
	req := new(http.Request)
	var body bytes.Buffer
	httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
		return io.TeeReader(bodyReader, &body), nil
	})
	rsp, err := ReadHTTPResponseFromBufioReader(bytes.NewReader([]byte(`HTTP/1.1 200 OK
Content-Length: 11

aaaaaaaaaaa`)), req)
	if err != nil {
		t.Errorf("read http response failed: %s", err)
		t.FailNow()
	}
	t.Logf("rsp: %p", rsp)
	if body.Len() != 11 {
		t.Errorf("invalid body length: %d", body.Len())
		t.FailNow()
	}
	if body.String() == "aaaaaaaaaaa" {
		spew.Dump(body.Bytes())
	}
}

func TestCtxEffectReader_KeyValue(t *testing.T) {
	req := new(http.Request)
	var body bytes.Buffer
	var clInt int
	httpctx.SetResponseHeaderParsed(req, func(key string, value string) {
		spew.Dump(key, value)
		if key == "content-length" {
			clInt = codec.Atoi(value)
		}
	})
	httpctx.SetResponseHeaderCallback(req, func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error) {
		if clInt == 11 {
			return io.TeeReader(bodyReader, &body), nil
		}
		return bodyReader, nil
	})
	rsp, err := ReadHTTPResponseFromBufioReader(bytes.NewReader([]byte(`HTTP/1.1 200 OK
Content-Length: 11

aaaaaaaaaaa`)), req)
	if err != nil {
		t.Errorf("read http response failed: %s", err)
		t.FailNow()
	}
	t.Logf("rsp: %p", rsp)
	if body.Len() != 11 {
		t.Errorf("invalid body length: %d", body.Len())
		t.FailNow()
	}
	if clInt != 11 {
		t.Errorf("invalid content-length: %d", clInt)
		t.FailNow()
	}
}

func TestHttpUtilsUrl2String(t *testing.T) {
	u, err := url.Parse("http://127.0.0.1:7788/${eval(danger)}/#${eval(danger)}")
	require.NoError(t, err)
	fmt.Println(Url2UnEscapeString(u))
	require.True(t, Url2UnEscapeString(u) == "http://127.0.0.1:7788/${eval(danger)}/#${eval(danger)}")
}
