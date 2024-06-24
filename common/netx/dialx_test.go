package netx

import (
	"strings"
	"testing"
)

func TestDialXProxy(t *testing.T) {
	defaultDialXOptions = []DialXOption{DialX_WithProxy("bbb")}
	_, err := DialX("aaa", DialX_WithProxy())
	if !strings.Contains(err.Error(), "bbb") {
		t.Fatal("set default proxy failed")
	}
}
