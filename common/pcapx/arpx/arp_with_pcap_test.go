package arpx

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"os"
	"testing"
)

func TestArpWithPcap(t *testing.T) {
	iface, target := os.Getenv("YAK_PCAP_LIVE_INTERFACE"), os.Getenv("YAK_PCAP_LIVE_TARGET")
	if os.Getenv("YAK_PCAP_LIVE_TEST") != "1" || iface == "" || target == "" {
		t.Skip("live ARP requires explicit interface, target and YAK_PCAP_LIVE_TEST=1")
	}
	a, err := ArpWithPcap(context.Background(), iface, target)
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}
