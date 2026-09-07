package stream_parser

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestRMIOriginalSyntaxAndEveryByte(t *testing.T) {
	f, e := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-rmi.pcap")
	require.NoError(t, e)
	defer f.Close()
	reader, e := pcapgo.NewReader(f)
	require.NoError(t, e)
	frame, data := 0, 0
	for {
		wire, _, e := reader.ReadPacketData()
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		frame++
		packet := gopacket.NewPacket(wire, layers.LayerTypeEthernet, gopacket.Default)
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if len(tcp.Payload) == 0 {
			continue
		}
		data++
		mode := "message"
		switch frame {
		case 4:
			mode = "header"
		case 6:
			mode = "server-handshake"
		case 8:
			mode = "client-endpoint"
		}
		t.Run(fmt.Sprintf("frame-%d", frame), func(t *testing.T) {
			fs, _, e := decodeRMIRecord(tcp.Payload, mode)
			require.NoError(t, e)
			var walk func([]rmiField, int) int
			walk = func(fs []rmiField, start int) int {
				for _, f := range fs {
					require.Equal(t, start, f.Start, f.Name)
					require.LessOrEqual(t, f.End, len(tcp.Payload), f.Name)
					if f.Type == "" {
						require.Equal(t, f.End, walk(f.Children, f.Start), f.Name)
					}
					start = f.End
				}
				return start
			}
			require.Equal(t, len(tcp.Payload), walk(fs, 0))
			for cut := 0; cut < len(tcp.Payload); cut++ {
				_, _, e := decodeRMIRecord(tcp.Payload[:cut], mode)
				// Call with no arguments and a normal void return are valid smaller
				// records; the method signature is deliberately not guessed.
				if mode == "message" && (tcp.Payload[0] == 0x50 || tcp.Payload[0] == 0x51) && e == nil {
					minimum := 41
					if tcp.Payload[0] == 0x51 {
						minimum = 22
					}
					require.Equal(t, minimum, cut, "only a signature-independent empty value record is a valid prefix")
					f, _, e := decodeRMIRecord(tcp.Payload[:cut], mode)
					require.NoError(t, e)
					require.NotEmpty(t, f)
					continue
				}
				require.Error(t, e, "prefix %d", cut)
			}
		})
	}
	require.Equal(t, 19, frame)
	require.Equal(t, 8, data)
}
