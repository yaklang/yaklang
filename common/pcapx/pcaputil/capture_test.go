package pcaputil

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	t.SkipNow()

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
	t.SkipNow()

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

func TestBackgroundHandler(t *testing.T) {
	var count = 0
	var count1 = 0
	go func() {
		err := Start(
			WithEmptyDeviceStop(true),
			WithDevice("en1"),
			WithEveryPacket(func(packet gopacket.Packet) {
				count++
			}),
			WithContext(utils.TimeoutContext(3*time.Second)),
			WithEnableCache(true),
		)
		if err != nil {
			t.Fatal(err)
		}
	}()

	go func() {
		err := Start(
			WithEmptyDeviceStop(true),
			WithDevice("en1"),
			WithEveryPacket(func(packet gopacket.Packet) {
				count1++
			}),
			WithContext(utils.TimeoutContextSeconds(8)),
			WithEnableCache(true),
		)
		if err != nil {
			t.Fatal(err)
		}
	}()

	for {
		time.Sleep(time.Second)
		spew.Dump(count, count1)
	}
}
