package netutil

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/tatsushid/go-fastping"
	"github.com/yaklang/yaklang/common/log"
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

func TestPING(t *testing.T) {
	// https://github.com/chenjiandongx/yap/blob/master/yap.go
	p := fastping.NewPinger()
	p.AddIP("192.168.3.63")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var addr string
	p.AddHandler("receive", func(ip *net.IPAddr, duration time.Duration) {
		if ip != nil {
			addr = ip.String()
		}
		cancel()
	})
	go func() {
		select {
		case <-ctx.Done():
			return
		}
	}()
	err := p.Run()
	if err != nil {
		log.Infof("fastping finished: %s", err)
	}
	println(addr)
}
