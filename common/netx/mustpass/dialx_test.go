package mustpass

import (
	"github.com/yaklang/yaklang/common/netx"
	"strings"
	"testing"
)

func TestDialXProxy(t *testing.T) {
	netx.SetDefaultDialXConfig(netx.DialX_WithProxy("bbb"))
	_, err := netx.DialX("aaa", netx.DialX_WithProxy())
	if !strings.Contains(err.Error(), "bbb") {
		t.Fatal("set default proxy failed")
	}
}
