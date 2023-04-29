package utils

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestFixProxy(t *testing.T) {
	var a = FixProxy("socks://1.2.3.4:111")
	spew.Dump(a)
	if a != "socks://1.2.3.4:111" {
		panic(1)
	}
}
