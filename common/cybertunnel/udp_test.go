package cybertunnel

import (
	"net"
	"testing"
)

func TestUDP(t *testing.T) {
	addr := &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: 53,
	}
	println(addr.String())
}
