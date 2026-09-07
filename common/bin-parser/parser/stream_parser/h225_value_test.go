package stream_parser

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func h225OriginalPackets(t *testing.T) []gopacket.Packet {
	t.Helper()
	f, e := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-h323.pcap")
	require.NoError(t, e)
	defer f.Close()
	r, e := pcapgo.NewReader(f)
	require.NoError(t, e)
	var packets []gopacket.Packet
	for {
		b, _, e := r.ReadPacketData()
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		packets = append(packets, gopacket.NewPacket(b, layers.LayerTypeEthernet, gopacket.Default))
	}
	require.Len(t, packets, 75)
	return packets
}

func h225AssertFields(t *testing.T, wire []byte, fs []h225Field) {
	t.Helper()
	var walk func([]h225Field, int) int
	walk = func(fs []h225Field, start int) int {
		for _, f := range fs {
			require.Equal(t, start, f.Start, f.Name)
			require.GreaterOrEqual(t, f.End, f.Start, f.Name)
			require.LessOrEqual(t, f.End, len(wire)*8, f.Name)
			if f.Type == "" {
				require.Equal(t, f.End, walk(f.Children, f.Start), f.Name)
			} else {
				require.Empty(t, f.Children, f.Name)
			}
			start = f.End
		}
		return start
	}
	require.Equal(t, len(wire)*8, walk(fs, 0))
}

func TestH225OriginalSchemaFields(t *testing.T) {
	packets := h225OriginalPackets(t)
	validCall := map[int]bool{6: true, 7: true, 10: true, 11: true, 14: true, 15: true, 18: true, 19: true, 47: true, 66: true}
	ras := map[int]uint64{60: 1, 61: 2, 62: 2, 63: 3, 64: 3, 67: 4180, 68: 4180, 69: 4181, 70: 4181, 71: 18067, 72: 18067, 73: 18068, 74: 18068, 75: 18069}
	for index, packet := range packets {
		frame := index + 1
		if l := packet.Layer(layers.LayerTypeUDP); l != nil {
			wire := l.(*layers.UDP).Payload
			t.Run(fmt.Sprintf("ras-%d", frame), func(t *testing.T) {
				f, _, e := decodeH225Message(wire, "ras")
				if frame == 59 {
					require.Error(t, e)
					return
				}
				require.NoError(t, e)
				h225AssertFields(t, wire, f)
				for cut := 0; cut < len(wire); cut++ {
					_, _, e := decodeH225Message(wire[:cut], "ras")
					require.Error(t, e, "prefix %d", cut)
				}
				var seq uint64
				var walk func([]h225Field)
				walk = func(fs []h225Field) {
					for _, f := range fs {
						if f.Name == "requestSeqNum" {
							seq = f.Value.(uint64)
						}
						walk(f.Children)
					}
				}
				walk(f)
				require.Equal(t, ras[frame], seq)
			})
			continue
		}
		l := packet.Layer(layers.LayerTypeTCP)
		if l == nil {
			continue
		}
		tcp := l.(*layers.TCP)
		if len(tcp.Payload) == 0 {
			continue
		}
		if frame == 48 || frame == 50 {
			continue
		}
		t.Run(fmt.Sprintf("tcp-%d", frame), func(t *testing.T) {
			f, _, e := decodeH225Message(tcp.Payload, "call")
			if validCall[frame] {
				require.NoError(t, e)
				h225AssertFields(t, tcp.Payload, f)
				for cut := 0; cut < len(tcp.Payload); cut++ {
					_, _, e := decodeH225Message(tcp.Payload[:cut], "call")
					require.Error(t, e, "prefix %d", cut)
				}
			} else {
				require.Error(t, e)
			}
		})
	}
	a := packets[47].Layer(layers.LayerTypeTCP).(*layers.TCP)
	b := packets[49].Layer(layers.LayerTypeTCP).(*layers.TCP)
	require.Equal(t, a.Seq+uint32(len(a.Payload)), b.Seq)
	wire := append(append([]byte{}, a.Payload...), b.Payload...)
	require.Equal(t, len(wire), int(binary.BigEndian.Uint16(wire[2:4])))
	f, _, e := decodeH225Message(wire, "call")
	require.NoError(t, e)
	h225AssertFields(t, wire, f)
}
