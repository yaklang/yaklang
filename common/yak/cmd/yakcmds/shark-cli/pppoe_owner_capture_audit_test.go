package sharkcli

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func n14CookedOwnerCaptureWire(t *testing.T, ethernet []byte, link layers.LinkType, source byte) []byte {
	t.Helper()
	body := ethernet[14:]
	ppp := uint16(0x21)
	if body[0]>>4 == 6 {
		ppp = 0x57
	}
	poe := []byte{0x11, 0, 0, 30, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(poe[4:6], uint16(2+len(body)))
	binary.BigEndian.PutUint16(poe[6:8], ppp)
	var h []byte
	if link == layers.LinkTypeLinuxSLL {
		h = make([]byte, 16)
		binary.BigEndian.PutUint16(h[2:4], 1)
		binary.BigEndian.PutUint16(h[4:6], 6)
		copy(h[6:12], []byte{2, 0, 0, source, 0, 1})
		binary.BigEndian.PutUint16(h[14:16], 0x8864)
	} else {
		h = make([]byte, 20)
		binary.BigEndian.PutUint16(h[:2], 0x8864)
		binary.BigEndian.PutUint32(h[4:8], 1)
		binary.BigEndian.PutUint16(h[8:10], 1)
		h[11] = 6
		copy(h[12:18], []byte{2, 0, 0, source, 0, 1})
	}
	return append(append(h, poe...), body...)
}

func TestN14PPPoECookedStartCaptureNeverFeedsNativeOrFallbackTCPPreview(t *testing.T) {
	request := "GET /owner-a HTTP/1.1\r\nHost: audit.invalid\r\n\r\n"
	forbidden := "GET /owner-b-forbidden HTTP/1.1\r\nHost: audit.invalid\r\n\r\n"
	for _, link := range []layers.LinkType{layers.LinkTypeLinuxSLL, layers.LinkTypeLinuxSLL2} {
		for _, freeze := range []bool{false, true} {
			t.Run(fmt.Sprintf("link%d/freeze%t", link, freeze), func(t *testing.T) {
				authored := [][]byte{
					nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
					nativeOpeningPacket(t, 2, 499, 100, "", true, true, false),
					nativeOpeningPacket(t, 3, 100, 500, request, false, false, false),
					nativeOpeningPacket(t, 4, 100+uint32(len(request)), 500, forbidden, false, false, false),
				}
				var capture bytes.Buffer
				writer := pcapgo.NewWriter(&capture)
				require.NoError(t, writer.WriteFileHeader(65535, link))
				var wires [][]byte
				for i, packet := range authored {
					source := byte(1)
					if i == 1 {
						source = 2
					}
					if i == 3 {
						source = 3
					}
					wire := n14CookedOwnerCaptureWire(t, packet, link, source)
					wires = append(wires, wire)
					ci := gopacket.CaptureInfo{Timestamp: time.Unix(100, int64(i)*1000000), CaptureLength: len(wire), Length: len(wire)}
					require.NoError(t, writer.WritePacket(ci, wire))
				}
				reader, err := pcaputil.NewCaptureReader(bytes.NewReader(capture.Bytes()))
				require.NoError(t, err)
				src := &captureSource{reader: reader, link: link, snaplen: 65535, offline: true, name: "authored-n14-cooked-pppoe"}
				session := startCapture(context.Background(), src, captureConfig{freezePacketFields: freeze}, false)
				defer session.Stop()
				var packets []*capturedPacket
				for packet := range session.packets {
					packets = append(packets, packet)
				}
				require.NoError(t, session.result())
				require.Len(t, packets, len(wires))
				require.Empty(t, session.streams.records, "cooked PPPoE cannot allocate a native or fallback preview")
				require.Empty(t, session.streams.nativeRecords)
				require.Empty(t, session.streams.active)
				require.Empty(t, session.streams.protocolConversations)
				require.Zero(t, session.streams.retained)
				require.Len(t, session.streams.assemblers, 1, "only the unexercised initial standalone assembler exists")
				for i, packet := range packets {
					require.NotNil(t, packet.history, "capture binds native history before analyzer.Feed; no fallback assembly")
					require.Equal(t, wires[i], packet.data, "the complete original physical frame remains inspectable")
					require.Zero(t, packet.tcpFlowID)
					require.Zero(t, packet.streamID)
					require.Zero(t, packet.conversationID)
					require.Empty(t, packet.connectionStack)
					require.Empty(t, packet.application)
					require.Nil(t, packet.transportView, "owner refusal precedes the beforeTransport callback")
					events := packet.protocolEvents()
					require.Len(t, events, 1)
					require.Equal(t, "PPPoEOwnerUnconfirmed", events[0].ExpertCode)
					require.Zero(t, events[0].FlowID)
					require.Zero(t, events[0].TCPFlowID)
					row := summarize(packet)
					require.Equal(t, "PPPoE", row.Protocol)
					require.Equal(t, "context-required", row.ProtocolStatus)
					require.Empty(t, row.ConnectionProtocol)
					require.Empty(t, row.Source)
					require.Empty(t, row.Destination)
					require.Zero(t, row.SourcePort)
					require.Zero(t, row.DestinationPort)
					require.NotContains(t, row.ProtocolStack, "TCP")
					require.NotContains(t, row.ProtocolStack, "IPv4")
					require.Contains(t, row.Info, "PPPoE header observed")
					require.Contains(t, row.Info, "owner unconfirmed")
					sidStart := 18
					if link == layers.LinkTypeLinuxSLL2 {
						sidStart = 22
					}
					for _, got := range []packetDetail{detail(packet), quickDetail(packet), workerMetadataDetailForPacket(t, packet)} {
						require.Equal(t, "PPPoE", got.protocol)
						require.NotEmpty(t, got.hex)
						require.Empty(t, got.sources, "raw physical bytes are not an admitted inner TCP/PDU byte source")
						text := detailLines(got.fields)
						require.NotContains(t, text, "YAML dissector")
						require.NotContains(t, text, "owner-b-forbidden")
						found := 0
						for _, f := range got.fields {
							if f.text == "Observed PPPoE Session ID: 30" {
								found++
								require.True(t, f.hasRange)
								require.Zero(t, f.source)
								require.Equal(t, sidStart, f.start)
								require.Equal(t, sidStart+2, f.end)
								require.Equal(t, []byte{0, 30}, packet.data[f.start:f.end])
							}
						}
						require.Equal(t, 1, found)
					}
					for _, snapshot := range packet.observedFields {
						for _, field := range snapshot.Values {
							for _, prefix := range []string{"tcp.", "http.", "udp.", "dns."} {
								require.NotContains(t, field.Name, prefix)
							}
						}
					}
				}
			})
		}
	}
}

func TestN14PPPoEOwnerRefusalAlsoStopsStandalonePreview(t *testing.T) {
	raw := n14CookedOwnerPresentationPacket(t, layers.LinkTypeLinuxSLL)
	require.Nil(t, raw.history)
	streams := newStreamStore(16, 65536)
	streams.add(raw)
	streams.finish()
	require.Empty(t, streams.records)
	require.Empty(t, streams.active)
	require.Zero(t, streams.retained)
	require.Zero(t, raw.streamID)
}
