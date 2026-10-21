package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func radiusNASPortControls(t *testing.T) []radiusEAPControl {
	var x struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-accounting-byte", "RADIUS_ACCOUNTING_BYTE_TEST_ROOT", "nas-port-controls.json"), &x))
	require.Len(t, x.Cases, 3)
	return x.Cases
}
func TestRADIUSNASPortMatrix(t *testing.T) {
	for _, c := range radiusNASPortControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := radiusEAPCapture(t, c)
			if root := os.Getenv("RADIUS_ACCOUNTING_BYTE_EXPORT"); root != "" {
				p := filepath.Join(root, "radius-accounting-byte")
				require.NoError(t, os.MkdirAll(p, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(p, c.Name+".pcap"), raw, 0600))
			} else {
				require.Equal(t, raw, finsReadBatch(t, "radius-accounting-byte", "RADIUS_ACCOUNTING_BYTE_TEST_ROOT", c.Name+".pcap"))
			}
			for _, workers := range []int{1, 2, 4} {
				for _, d := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, d, observe), func(t *testing.T) {
							var events, originals []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(d), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
								snap := *e
								snap.Raw = bytes.Clone(e.Raw)
								snap.Session = cloneSession(e.Session)
								snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
								events = append(events, &snap)
								originals = append(originals, e)
								if observe {
									tlsDTLSPoison(e, "http")
									e.ID, e.FlowID = 0, 0
								}
							})}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							a, err := NewPacketAnalyzer(opts...)
							require.NoError(t, err)
							a.conf.binParser.budget = radiusEAPBudget(c)
							if c.Budget.MaxMessageBytes > 0 {
								a.conf.binParser.config.MaxMessageBytes = c.Budget.MaxMessageBytes
							}
							r, err := NewCaptureReader(bytes.NewReader(raw))
							require.NoError(t, err)
							for {
								b, ci, err := r.ReadPacketData()
								if err == io.EOF {
									break
								}
								require.NoError(t, err)
								require.NoError(t, a.Feed(b, ci, r.LinkType()))
								clear(b)
							}
							require.NoError(t, a.Close())
							require.NoError(t, a.Close())
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.Equal(t, len(c.Steps), seen)
							}
							radiusEAPCheck(t, c, events, true, d)
							for i := range c.Steps {
								f, err := originals[i].GetFields()
								rocTypedError(t, c.Steps[i].Error, err)
								if c.Steps[i].Error == "" {
									require.Equal(t, radiusEAPWant(t, c, i), f)
								} else {
									require.Nil(t, f)
								}
							}
						})
					}
				}
			}
		})
	}
}
