package yakdns

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"testing"
)

func TestNativeLookupHost(t *testing.T) {
	var a = nativeLookupHost(utils.TimeoutContextSeconds(5), "baidu.com")
	spew.Dump(a)
}

func TestPlayground(t *testing.T) {
	conn, err := net.Dial("udp", "8.8.8.7:53")
	if err != nil {
		panic(err)
	}
	_ = conn.Close()
}
