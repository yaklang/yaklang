package pcaputil

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/utils"
	"net"
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
	swg := utils.NewSizedWaitGroup(2)
	swg.Add(2)
	go func() {
		defer swg.Done()
		err := Start(
			WithEmptyDeviceStop(true),
			WithDevice("en1"),
			WithEveryPacket(func(packet gopacket.Packet) {
				count++
			}),
			WithContext(utils.TimeoutContext(2*time.Second)),
			WithEnableCache(true),
			WithMockPcapOperation(&MockPcapOperation{}),
		)
		if err != nil {
			t.Fatal(err)
		}
	}()

	go func() {
		defer swg.Done()
		err := Start(
			WithEmptyDeviceStop(true),
			WithDevice("en1"),
			WithEveryPacket(func(packet gopacket.Packet) {
				count1++
			}),
			WithContext(utils.TimeoutContextSeconds(4)),
			WithEnableCache(true),
			WithMockPcapOperation(&MockPcapOperation{}),
		)
		if err != nil {
			t.Fatal(err)
		}
	}()
	swg.Wait()
	spew.Dump(count, count1)
	if count1-count < 10 {
		t.Fatal("count1-count < 10")
	}
}

func TestWindowsToPcapGuid(t *testing.T) {
	if utils.InGithubActions() {
		t.SkipNow()
	}
	interfaces, err := net.Interfaces()
	require.NoError(t, err)
	for _, nic := range interfaces {

		fmt.Println(nic.Name)
		pcapName, err := deviceNameToPcapGuidWindows(nic.Name)
		//require.NoError(t, err)
		fmt.Println(err)
		fmt.Println(pcapName)
	}
	//
	fmt.Println("--------------------------------")

	pcapics, err := pcap.FindAllDevs()
	require.NoError(t, err)
	for _, pcapic := range pcapics {
		fmt.Println(pcapic.Name)
	}

}
