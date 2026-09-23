package netutil

import (
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestRoute(t *testing.T) {
	test := assert.New(t)
	iface, gw, src, err := Route(3*time.Second, "8.8.8.8")
	if !test.Nil(err) {
		t.FailNow()
	}
	spew.Dump(iface, gw, src)
}

func TestArp(t *testing.T) {
	t.Skip("utils.Arp function not implemented yet")
	// addr, err := utils.Arp("en0", "192.168.3.63")
	// if err != nil {
	// 	panic(err)
	// }
	// spew.Dump(addr)
}
