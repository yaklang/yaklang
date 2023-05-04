package lowhttp

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestUrlToGetRequestPacket(t *testing.T) {
	res := UrlToGetRequestPacket("https://baidu.com/asdfasdfasdf", []byte(`GET / HTTP/1.1
Host: baidu.com
Cookie: test=12;`), false)
	spew.Dump(res)
}
