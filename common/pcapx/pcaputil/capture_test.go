package pcaputil

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket"
	"testing"
)

func TestStart(t *testing.T) {
	err := Start(
		WithDebug(false),
		WithDevice("WLAN"),
		WithOutput("./output.pcap"),
	)
	if err != nil {
		t.Error(err)
	}
}

func TestStart1(t *testing.T) {
	err := Start(
		WithBPFFilter("host 93.184.216.34"),
		WithDebug(false),
		WithDevice("en0"),
		WithEveryPacket(func(packet gopacket.Packet) {
			spew.Dump(packet.Data())
		}),
	)
	if err != nil {
		t.Error(err)
	}
}
