package pcaputil

import (
	"encoding/binary"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReviewTCPDNSObserverParity(t *testing.T) {
	wire := dnsWire(dnsQuery(0x5013, "example.com", 1))
	framed := append(binary.BigEndian.AppendUint16(nil, uint16(len(wire))), wire...)
	for _, chunk := range []int{1, 7, 64} {
		steps := []tcpStep{{seq: 99, syn: true}}
		for off := 0; off < len(framed); off += chunk {
			steps = append(steps, tcpStep{seq: 100 + uint32(off), data: string(framed[off:min(off+chunk, len(framed))])})
		}
		steps = append(steps, tcpStep{seq: 100 + uint32(len(framed)), fin: true})
		capture := binTestPcap(t, steps, 53, false, false)
		for _, workers := range []int{1, 2, 4} {
			for _, observer := range []bool{false, true} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("chunk%d/workers%d/observer%v/deferred%v", chunk, workers, observer, deferred), func(t *testing.T) {
						opts := []CaptureOption{WithProtocolDeferred(deferred)}
						if observer {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
						}
						events, stats, err := binReplay(t, capture, workers, opts...)
						require.NoError(t, err)
						require.Len(t, events, 1)
						e := events[0]
						require.Equal(t, "dns", e.Protocol)
						require.Equal(t, framed, e.Raw)
						require.Contains(t, []string{"decoded", "deferred"}, e.Status)
						require.Empty(t, e.Error)
						_, err = e.GetFields()
						require.NoError(t, err)
						require.Equal(t, "example.com", e.Session["DNS"].(map[string]any)["Questions"].([]map[string]any)[0]["Name"])
						require.Zero(t, stats.BufferedBytes)
						require.Zero(t, stats.Malformed+stats.Unknown+stats.Incomplete)
					})
				}
			}
		}
	}
}
