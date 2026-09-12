package bin_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

const t38CorpusCapture = "testdata/protocol-corpus/captures/ndpi/ndpi-t38.pcap"

// The pinned material proves a T.38 SDP advertisement, but it does not contain
// a valid UDPTL message that bin-parser can honestly present as T.38 fields.
// The packets immediately after the final advertisement continue the prior
// RTP/PCMA sequence. Keep this distinction executable so a heuristic decoder
// label cannot silently promote the sample to field-level T.38 coverage.
func TestProtocolCorpusT38AdvertisementKeepsMediaEvidenceSeparate(t *testing.T) {
	captureData := readProtocolCorpusFile(t, ".", t38CorpusCapture)
	reader, err := pcapgo.NewReader(bytes.NewReader(captureData))
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())

	packetCount := 0
	advertisements := 0
	media := make(map[int][]byte)
	for {
		frame, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		packetCount++
		lowerFrame := bytes.ToLower(frame)
		if bytes.Contains(lowerFrame, []byte("m=image")) && bytes.Contains(lowerFrame, []byte("udptl")) {
			advertisements++
		}

		if packetCount != 1429 && packetCount != 1436 && packetCount != 1438 {
			continue
		}
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		require.NotNilf(t, udpLayer, "frame %d has no UDP layer", packetCount)
		udp := udpLayer.(*layers.UDP)
		require.Equalf(t, layers.UDPPort(15580), udp.SrcPort, "frame %d source port", packetCount)
		require.Equalf(t, layers.UDPPort(16756), udp.DstPort, "frame %d destination port", packetCount)
		media[packetCount] = append([]byte(nil), udp.Payload...)
	}

	require.Equal(t, 1552, packetCount)
	require.Equal(t, 16, advertisements)
	require.Len(t, media, 3)

	previousSequence := uint16(1843)
	previousTimestamp := uint32(1741919976)
	for _, frameNumber := range []int{1429, 1436, 1438} {
		payload := media[frameNumber]
		require.Lenf(t, payload, 172, "frame %d media payload", frameNumber)
		require.Equalf(t, uint8(2), payload[0]>>6, "frame %d RTP version", frameNumber)
		require.Equalf(t, uint8(8), payload[1]&0x7f, "frame %d RTP payload type", frameNumber)
		require.Equalf(t, previousSequence, binary.BigEndian.Uint16(payload[2:4]), "frame %d RTP sequence", frameNumber)
		require.Equalf(t, previousTimestamp, binary.BigEndian.Uint32(payload[4:8]), "frame %d RTP timestamp", frameNumber)
		require.Equalf(t, uint32(0x0eaf0eaf), binary.BigEndian.Uint32(payload[8:12]), "frame %d RTP SSRC", frameNumber)
		previousSequence++
		previousTimestamp += 160
	}
}
