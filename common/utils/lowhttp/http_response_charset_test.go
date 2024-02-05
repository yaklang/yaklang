package lowhttp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixResponse_CharSet(t *testing.T) {
	test := assert.New(t)
	t.Run("no content-type charset,body utf-8", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 18\r\n\r\n你好，世界！")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "Content-Length: 18")
		test.Contains(string(rsp), "你好，世界！")
	})
	t.Run("no content-type charset,body gbk", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 18\r\n\r\n\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "Content-Type: text/html; charset=utf-8")
		test.Contains(string(rsp), "Content-Length: 18")
		test.Contains(string(rsp), "你好，世界！")
	})

	t.Run("content-type charset utf-8,body gbk", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: 18\r\n\r\n\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "Content-Type: text/html; charset=utf-8")
		test.Contains(string(rsp), "Content-Length: 12")
		test.Contains(string(rsp), "\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
	})

	t.Run("no content-type charset,meta gbk", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 18\r\n\r\n<html><header><meta charset='gbk' /></header><body>\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1</body></html>")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "meta charset='utf-8'")
		test.Contains(string(rsp), "你好，世界！")
	})

	t.Run("content-type file-type", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: image/gif\r\nContent-Length: 18\r\n\r\n\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
	})

	t.Run("no content-type,file body", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 18\r\n\r\nGIF89a\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
		rsp, _, err := FixHTTPResponse(packet)
		test.Nil(err, "FixHTTPResponse error")
		test.Contains(string(rsp), "GIF89a\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7\xa3\xa1")
	})
}
