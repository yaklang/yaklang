package pcapdb

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

type endpointFixturePacket struct {
	sourceMAC net.HardwareAddr
	sourceIP  net.IP
	vlan      uint16
}

func endpointFixtureBytes(t testing.TB, packet endpointFixturePacket) []byte {
	t.Helper()
	ethernet := &layers.Ethernet{
		SrcMAC:       packet.sourceMAC,
		DstMAC:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: packet.sourceIP, DstIP: net.IP{192, 0, 2, 1}, Protocol: layers.IPProtocolUDP}
	udp := &layers.UDP{SrcPort: 40000, DstPort: 53}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	serializable := []gopacket.SerializableLayer{ethernet}
	if packet.vlan != 0 {
		ethernet.EthernetType = layers.EthernetTypeDot1Q
		serializable = append(serializable, &layers.Dot1Q{VLANIdentifier: packet.vlan, Type: layers.EthernetTypeIPv4})
	}
	serializable = append(serializable, ip, udp, gopacket.Payload(nil))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serializable...))
	return buffer.Bytes()
}

func endpointFixtureCapture(t testing.TB) string {
	t.Helper()
	macA := net.HardwareAddr{0x02, 0, 0, 0, 0, 0x01}
	macB := net.HardwareAddr{0x02, 0, 0, 0, 0, 0x02}
	macC := net.HardwareAddr{0x02, 0, 0, 0, 0, 0x03}
	packets := []endpointFixturePacket{
		{sourceMAC: macA, sourceIP: net.IP{10, 0, 0, 1}},
		{sourceMAC: macA, sourceIP: net.IP{10, 0, 0, 1}},
		{sourceMAC: macB, sourceIP: net.IP{10, 0, 0, 2}, vlan: 10},
		{sourceMAC: macB, sourceIP: net.IP{10, 0, 0, 2}, vlan: 20},
		{sourceMAC: macC, sourceIP: net.IP{8, 8, 8, 8}},
		{sourceMAC: macC, sourceIP: net.IP{1, 1, 1, 1}},
	}
	var capture bytes.Buffer
	writer := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for index, fixture := range packets {
		raw := endpointFixtureBytes(t, fixture)
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{
			Timestamp:     time.Unix(1700000000, int64(index+1)*1000),
			CaptureLength: len(raw),
			Length:        len(raw),
		}, raw))
	}
	return writeCapture(t, capture.Bytes())
}

func TestPCAPDBMACAndSourceIPEndpointSummaries(t *testing.T) {
	input := endpointFixtureCapture(t)
	manager := testManager(t)
	db, err := manager.GetOrCreate(input, WithBatchSize(2))
	require.NoError(t, err)

	packets, err := db.QueryPackets(QueryLimit(10))
	require.NoError(t, err)
	require.Len(t, packets, 6)
	require.Equal(t, "02:00:00:00:00:01", packets[0].SourceMAC)
	require.Equal(t, "aa:bb:cc:dd:ee:ff", packets[0].DestinationMAC)
	require.Empty(t, packets[0].VLANStack)
	require.Equal(t, "10", packets[2].VLANStack)
	require.Equal(t, "20", packets[3].VLANStack)

	macs := make([]MACSummary, 0)
	after := int64(0)
	pages := 0
	for {
		page, err := db.QueryMACSummariesPage(QueryAfter(after), QueryLimit(2))
		require.NoError(t, err)
		pages++
		require.LessOrEqual(t, len(page.Items), 2)
		macs = append(macs, page.Items...)
		if !page.HasMore {
			break
		}
		require.NotZero(t, page.NextCursor)
		after = page.NextCursor
	}
	require.Equal(t, 2, pages)
	require.Len(t, macs, 4)
	byKey := make(map[string]MACSummary, len(macs))
	for _, summary := range macs {
		byKey[fmt.Sprintf("%s|%s", summary.SourceMAC, summary.VLANStack)] = summary
	}
	require.EqualValues(t, 1, byKey["02:00:00:00:00:01|"].SourceIPCount)
	require.EqualValues(t, 2, byKey["02:00:00:00:00:01|"].PacketCount)
	require.EqualValues(t, 1, byKey["02:00:00:00:00:02|10"].SourceIPCount)
	require.EqualValues(t, 1, byKey["02:00:00:00:00:02|20"].SourceIPCount)
	require.EqualValues(t, 2, byKey["02:00:00:00:00:03|"].SourceIPCount)
	require.EqualValues(t, 2, byKey["02:00:00:00:00:03|"].PacketCount)
	require.EqualValues(t, 5, byKey["02:00:00:00:00:03|"].FirstPacketID)
	require.EqualValues(t, 6, byKey["02:00:00:00:00:03|"].LastPacketID)

	bindings, err := db.QueryEndpointBindingsPage(
		QuerySourceMAC("02:00:00:00:00:03"),
		QueryAfter(0),
		QueryLimit(10),
	)
	require.NoError(t, err)
	require.False(t, bindings.HasMore)
	require.Len(t, bindings.Items, 2)
	require.Equal(t, "1.1.1.1", bindings.Items[0].SourceIP)
	require.Equal(t, "8.8.8.8", bindings.Items[1].SourceIP)
	require.EqualValues(t, 6, bindings.Items[0].FirstPacketID)
	require.EqualValues(t, 5, bindings.Items[1].FirstPacketID)

	filtered, err := db.QueryEndpointBindingsPage(
		QuerySourceIP("10.0.0.2"),
		QueryLimit(10),
	)
	require.NoError(t, err)
	require.Len(t, filtered.Items, 2)
	require.Equal(t, "10", filtered.Items[0].VLANStack)
	require.Equal(t, "20", filtered.Items[1].VLANStack)

	var bindingPages []EndpointBindingSummary
	bindingAfter := int64(0)
	for {
		page, err := db.QueryEndpointBindingsPage(QueryAfter(bindingAfter), QueryLimit(2))
		require.NoError(t, err)
		bindingPages = append(bindingPages, page.Items...)
		if !page.HasMore {
			break
		}
		bindingAfter = page.NextCursor
	}
	require.Len(t, bindingPages, 5)
	for index, binding := range bindingPages {
		require.EqualValues(t, index+1, binding.ID)
	}

	windowed, err := db.QueryEndpointBindingsPage(
		QueryTimeRange(
			time.Unix(1700000000, 4000).UnixNano(),
			time.Unix(1700000000, 5000).UnixNano(),
		),
		QueryLimit(10),
	)
	require.NoError(t, err)
	require.Len(t, windowed.Items, 1)
	require.Equal(t, "02:00:00:00:00:02", windowed.Items[0].SourceMAC)
	require.Equal(t, "10.0.0.2", windowed.Items[0].SourceIP)
	require.Equal(t, "20", windowed.Items[0].VLANStack)
	require.EqualValues(t, 1, windowed.Items[0].PacketCount)

	_, err = db.QueryEndpointBindingsPage(QuerySourceMAC("not-a-mac"))
	require.ErrorContains(t, err, "invalid source MAC")
}
