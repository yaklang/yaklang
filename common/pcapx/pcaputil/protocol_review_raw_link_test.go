package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

func TestReviewExplicitIPLinkTypes(t *testing.T) {
	request := "GET /raw-link HTTP/1.1\r\nHost: example.test\r\n\r\n"
	for _, ipv6 := range []bool{false, true} {
		ethernet := binTestPcap(t, []tcpStep{{seq: 99, syn: true}, {seq: 100, data: request}, {seq: 100 + uint32(len(request)), fin: true}}, 80, ipv6, false)
		explicit := layers.LinkTypeIPv4
		if ipv6 {
			explicit = layers.LinkTypeIPv6
		}
		for _, link := range []layers.LinkType{layers.LinkTypeRaw, explicit} {
			var capture bytes.Buffer
			w := pcapgo.NewWriterNanos(&capture)
			require.NoError(t, w.WriteFileHeader(65535, link))
			r, err := pcapgo.NewReader(bytes.NewReader(ethernet))
			require.NoError(t, err)
			for {
				raw, ci, err := r.ReadPacketData()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				ci.CaptureLength -= 14
				ci.Length -= 14
				require.NoError(t, w.WritePacket(ci, raw[14:]))
			}
			for _, workers := range []int{1, 2, 4} {
				for _, observer := range []bool{false, true} {
					for _, deferred := range []bool{false, true} {
						t.Run(fmt.Sprintf("link%d/workers%d/observer%v/deferred%v", link, workers, observer, deferred), func(t *testing.T) {
							opts := []CaptureOption{WithProtocolDeferred(deferred)}
							seen := 0
							if observer {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotNil(t, p.NetworkLayer()); seen++ }))
							}
							events, stats, err := binReplay(t, capture.Bytes(), workers, opts...)
							require.NoError(t, err)
							require.Len(t, events, 1)
							require.Equal(t, "http", events[0].Protocol)
							require.Equal(t, []byte(request), events[0].Raw)
							fields, err := events[0].GetFields()
							require.NoError(t, err)
							require.Equal(t, "/raw-link", fields["Message"].(map[string]any)["HTTP Request"].(map[string]any)["FirstLine"].(map[string]any)["Path"])
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.Unknown+stats.Malformed)
							if observer {
								require.Equal(t, 3, seen)
							}
						})
					}
				}
			}
		}
	}
}
