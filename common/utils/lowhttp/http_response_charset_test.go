package lowhttp

import (
	"fmt"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixResponse_WithTextFallback(t *testing.T) {
	response := `HTTP/1.1 200 OK
Content-Type: text/aabc

<html><head></head><body>你好，世界！</body></html>`
	rsp, _, err := FixHTTPResponse([]byte(response))
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, string(rsp), "你好，世界！")
	assert.Contains(t, string(rsp), "text/aabc")
	assert.NotContains(t, string(rsp), "charset=utf-8")
}

func TestFixResponse_WithTextFallback2(t *testing.T) {
	sample, _ := codec.Utf8ToGB18030([]byte("你好，世界！"))
	response1 := `HTTP/1.1 200 OK
Content-Type: text/aabc

<html><head><meta charset="gbk"></head><body>` + string(sample) + `</body></html>`
	rsp, _, err := FixHTTPResponse([]byte(response1))
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, string(rsp), "你好，世界！")
	assert.Contains(t, string(rsp), "text/aabc")
	assert.Contains(t, string(rsp), "charset=utf-8")
	assert.Contains(t, string(rsp), "charset=\"utf-8\"")
}

func TestFixResponse_WithTextFallback3(t *testing.T) {
	sample, _ := codec.Utf8ToGB18030([]byte("你好，世界！"))
	response1 := `HTTP/1.1 200 OK
Content-Type: text/aabc; charset=gbk

<html><head><meta charset="gbk"></head><body>` + string(sample) + `</body></html>`
	rsp, _, err := FixHTTPResponse([]byte(response1))
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, string(rsp), "你好，世界！")
	assert.Contains(t, string(rsp), "text/aabc")
	assert.Contains(t, string(rsp), "charset=utf-8")
	assert.NotContains(t, string(rsp), "charset=gbk")
	assert.Contains(t, string(rsp), "charset=\"utf-8\"")
}

func TestFixResponse_CharSet(t *testing.T) {
	t.Run("no content-type charset,body utf-8", func(t *testing.T) {
		test := assert.New(t)
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 18\r\n\r\n你好，世界！")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "Content-Length: 18")
		test.Contains(string(rsp), "你好，世界！")
	})
	t.Run("no content-type charset,body gbk", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 18\r\n\r\n\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		test := assert.New(t)

		test.Nil(err, "FixHTTPResponse error")
		fmt.Println(string(rsp))
		test.Contains(string(rsp), "Content-Type: text/html; charset=utf-8")
		test.Contains(string(rsp), "Content-Length: 18")
		test.Contains(string(rsp), "你好，世界！")
	})

	//t.Run("content-type charset utf-8,body gbk", func(t *testing.T) {
	//	test := assert.New(t)
	//
	//	packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: 18\r\n\r\n\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
	//	rsp, _, err := FixHTTPResponse(packet)
	//	fmt.Println(string(rsp))
	//	test.Nil(err, "FixHTTPResponse error")
	//	test.Contains(string(rsp), "Content-Type: text/html; charset=utf-8")
	//	test.Contains(string(rsp), "Content-Length: 12")
	//	sample := "\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1"
	//	fmt.Println(sample)
	//	test.Contains(string(rsp), sample)
	//})

	t.Run("content-type charset utf-8,body gbk", func(t *testing.T) {
		test := assert.New(t)

		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: 18\r\n\r\n\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		fmt.Println(string(rsp))
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "Content-Type: text/html; charset=utf-8")
		test.Contains(string(rsp), "Content-Length: 18")
		sample := "你好，世界！"
		fmt.Println(sample)
		test.Contains(string(rsp), sample)
	})

	t.Run("content-type charset gbk,body gbk", func(t *testing.T) {
		test := assert.New(t)

		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\nContent-Length: 18\r\n\r\n\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		fmt.Println(string(rsp))
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "Content-Type: text/html; charset=utf-8")
		test.Contains(string(rsp), "Content-Length: 18")
		sample := "你好，世界！"
		fmt.Println(sample)
		test.Contains(string(rsp), sample)
	})

	t.Run("content-type charset gbk,body gbk (limit break)", func(t *testing.T) {
		test := assert.New(t)

		raw, _ := codec.Utf8ToGB18030([]byte(`你好世界`))
		body := strings.Repeat("a", mimetype.GetLimit()-1)
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\nContent-Length: 18\r\n\r\n" + body + string(raw))
		rsp, _, err := FixHTTPResponse(packet)
		fmt.Println(string(rsp))
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "Content-Type: text/html; charset=utf-8")
		test.Contains(string(rsp), "你好世界")
	})

	t.Run("content-type charset gbk,body gbk (limit break 2)", func(t *testing.T) {
		test := assert.New(t)

		raw, _ := codec.Utf8ToGB18030([]byte(`你好世界`))
		body := strings.Repeat("a", mimetype.GetLimit()-len(raw)+1)
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\nContent-Length: 18\r\n\r\n" + body + string(raw))
		rsp, _, err := FixHTTPResponse(packet)
		fmt.Println(string(rsp))
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "Content-Type: text/html; charset=utf-8")
		test.Contains(string(rsp), "你好世界")
	})

	t.Run("no content-type charset,meta gbk", func(t *testing.T) {
		test := assert.New(t)

		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 18\r\n\r\n<html><header><meta charset='gbk' /></header><body>\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1</body></html>")
		rsp, _, err := FixHTTPResponse(packet)
		fmt.Println(string(rsp))
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "meta charset='utf-8'")
		test.Contains(string(rsp), "你好，世界！")
	})

	t.Run("no content-type charset, no meta, but gbk", func(t *testing.T) {
		test := assert.New(t)

		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 18\r\n\r\n<html><header></header><body>\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1</body></html>")
		rsp, _, err := FixHTTPResponse(packet)
		fmt.Println(string(rsp))
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "你好，世界！")
	})

	t.Run("content-type file-type", func(t *testing.T) {
		test := assert.New(t)

		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: image/gif\r\nContent-Length: 18\r\n\r\n\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
	})

	t.Run("no content-type,file body", func(t *testing.T) {
		test := assert.New(t)

		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 18\r\n\r\nGIF89a\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "GIF89a\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
	})

	t.Run("content-type text_html,file body(GIF)", func(t *testing.T) {
		test := assert.New(t)

		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 18\r\n\r\nGI4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "GI4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		test.Contains(string(rsp), "Content-Type: text/html")
	})

	t.Run("content-type text_html,file body(Binary)", func(t *testing.T) {
		test := assert.New(t)
		hexBody := "2c5fc8a643ef334889238c26a41b360daa0156f71b0cca70b8bee7612de7fe4e"
		body, _ := codec.DecodeHex(hexBody)
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 18\r\n\r\n" + string(body))
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), string(body))
		test.Contains(string(rsp), "Content-Type: text/html")
	})
}
