package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestIPv4ZeroTotalLengthRetainsLiteralFieldThroughWorker(t *testing.T) {
	base := tcpFailureRaw(tcpFailureBody(20, nil, []byte{0x80, 1, 2, 3, 4}))
	binary.BigEndian.PutUint16(base.data[16:18], 0)
	ip := bytes.Clone(base.data[14:])
	for _, c := range []struct {
		name string
		link layers.LinkType
		wire []byte
		at   int
	}{
		{"EthernetOptions", layers.LinkTypeEthernet, base.data, 14},
		{"Raw", layers.LinkTypeRaw, ip, 0},
		{"Loopback", layers.LinkTypeNull, append([]byte{2, 0, 0, 0}, ip...), 4},
		{"VLAN", layers.LinkTypeEthernet, append(append(bytes.Clone(base.data[:12]), 0x81, 0, 0, 77, 8, 0), ip...), 18},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw := &capturedPacket{number: 1, data: bytes.Clone(c.wire), link: c.link, ci: gopacket.CaptureInfo{CaptureLength: len(c.wire), Length: len(c.wire)}}
			require.Nil(t, tcpFailureDecoded(raw).ErrorLayer(), "complete captured TCP header is the legacy compatibility case")
			before := bytes.Clone(raw.data)
			for name, d := range map[string]packetDetail{"detail": detail(raw), "quick": quickDetail(raw), "worker": workerMetadataDetailForPacket(t, raw)} {
				t.Run(name, func(t *testing.T) {
					text := detailLines(d.fields)
					require.Contains(t, text, "TSO origin unconfirmed")
					require.Contains(t, text, "captured-prefix observation")
					require.NotContains(t, text, "YAML dissector")
					require.NotContains(t, text, fmt.Sprintf("Length: %d", len(ip)))
					found := 0
					for _, f := range d.fields {
						if f.text != "Total Length: 0" {
							continue
						}
						found++
						require.True(t, f.hasRange)
						require.Zero(t, f.source)
						require.Equal(t, c.at+2, f.start)
						require.Equal(t, c.at+4, f.end)
						require.Equal(t, []byte{0, 0}, raw.data[f.start:f.end])
					}
					require.Equal(t, 1, found, "one literal field, independent of the dependency's effective length")
					require.NotContains(t, strings.ToLower(text), "tso verified")
				})
			}
			require.Equal(t, before, raw.data)
		})
	}
}

func TestIPv4ZeroTotalLengthDoesNotSelectSpeculativeInnerParent(t *testing.T) {
	inner := tcpFailureRaw(tcpFailureBody(20, nil, []byte{1, 2, 3}))
	binary.BigEndian.PutUint16(inner.data[16:18], 0)
	ip := bytes.Clone(inner.data[14:])
	outer := make([]byte, 20)
	outer[0], outer[8], outer[9] = 0x45, 64, 4
	binary.BigEndian.PutUint16(outer[2:4], uint16(len(outer)+len(ip)))
	copy(outer[12:20], []byte{192, 0, 2, 1, 192, 0, 2, 2})
	wire := append(append(bytes.Clone(inner.data[:14]), outer...), ip...)
	raw := &capturedPacket{number: 1, data: wire, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{CaptureLength: len(wire), Length: len(wire)}}
	require.True(t, hasCapturedZeroLengthIPv4(raw, nil))
	require.False(t, hasCapturedZeroLengthIPv4(raw, &packetTransportView{Layers: []string{"Ethernet", "IPv4"}}))
	require.False(t, hasCapturedZeroLengthIPv4(raw, &packetTransportView{Layers: []string{"Ethernet", "IPv6"}}))
	for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		found := false
		for _, f := range d.fields {
			if f.text == "Total Length: 0" {
				found = true
				require.True(t, f.hasRange)
				require.Zero(t, f.source)
				require.Equal(t, 36, f.start)
				require.Equal(t, 38, f.end)
			}
		}
		require.True(t, found)
	}
	decoded := tcpFailureDecoded(inner)
	observed := decoded.NetworkLayer().(*layers.IPv4)
	for _, at := range []int{-1, 0, len(inner.data)} {
		var d packetDetail
		require.False(t, appendZeroLengthIPv4Fields(&d, inner.data, observed, at))
		require.Empty(t, d.fields, "a header without its exact captured offset cannot create ranges")
	}
	changed := bytes.Clone(inner.data)
	changed[16] = 1
	var d packetDetail
	require.False(t, appendZeroLengthIPv4Fields(&d, changed, observed, 14))
	require.Empty(t, d.fields, "bytes decoded from another packet do not qualify this source")
}
