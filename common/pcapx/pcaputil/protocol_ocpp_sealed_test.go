package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testOCPP16SealedMatrix(t *testing.T) {
	var answers struct {
		Profiles map[string]struct {
			Events []struct {
				Record             string
				Direction          int
				Session            map[string]any `json:"session"`
				ResponseTo         *int           `json:"response_to_event"`
				RequestTransaction bool           `json:"request_transaction"`
			}
		}
	}
	raw, err := trafficfixture.ReadFile("testdata/ocpp16-rpc/answers.json")
	require.NoError(t, err)
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&answers))
	require.Len(t, answers.Profiles, 21)
	for name, p := range answers.Profiles {
		t.Run(name, func(t *testing.T) {
			check := func(t *testing.T, events []*ProtocolEvent) {
				t.Helper()
				require.Len(t, events, len(p.Events))
				offsets := [2]uint64{}
				for i, e := range events {
					want := p.Events[i]
					wire, err := trafficfixture.ReadFile(strings.TrimPrefix(want.Record, "common/pcapx/pcaputil/"))
					require.NoError(t, err)
					protocol := "websocket"
					if i < 2 {
						protocol = "http"
					}
					require.Equal(t, protocol, e.Protocol)
					require.Equal(t, want.Direction, e.Direction)
					require.Equal(t, wire, e.Raw)
					require.Equal(t, len(wire), e.Length)
					require.Equal(t, offsets[want.Direction], e.Offset)
					offsets[want.Direction] += uint64(len(wire))
					require.Empty(t, e.Error)
					spAssertTree(t, want.Session, e.Session)
					if want.ResponseTo != nil {
						require.Equal(t, events[*want.ResponseTo].ID, e.ResponseTo)
						require.Equal(t, events[*want.ResponseTo].ID, e.TransactionID)
					} else {
						require.Zero(t, e.ResponseTo)
						if want.RequestTransaction {
							require.Equal(t, e.ID, e.TransactionID)
						} else {
							require.Zero(t, e.TransactionID)
						}
					}
					tree, err := e.Decode()
					require.NoError(t, err)
					if want.Session != nil {
						spAssertTree(t, want.Session, tree["session"])
					}
					fields, err := e.GetFields()
					require.NoError(t, err)
					if app, ok := want.Session["OCPP"]; ok {
						spAssertTree(t, app, fields["OCPP"])
						original := cloneSession(fields)
						e.Raw[0] ^= 255
						e.Session = map[string]any{"forged": true}
						e.Fields = map[string]any{"forged": true}
						e.Structured = map[string]any{"fields": e.Fields}
						for k := range fields {
							delete(fields, k)
						}
						fresh, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, original, fresh)
						decoded, err := e.Decode()
						require.NoError(t, err)
						spAssertTree(t, want.Session, decoded["session"])
					} else {
						require.NotContains(t, fields, "OCPP")
					}
				}
			}
			capture, err := trafficfixture.ReadFile("testdata/ocpp16-rpc/captures/" + name + ".pcap")
			require.NoError(t, err)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("capture/w%d/d%v/o%v", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Uint64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(capture), opts...))
							for _, e := range events {
								require.NotEmpty(t, e.SourceBytes.PacketRefs)
								sum := sha256.Sum256(e.Raw)
								require.Equal(t, fmt.Sprintf("%x", sum), e.SourceBytes.SHA256)
							}
							check(t, events)
							require.Zero(t, stats.BufferedBytes)
							if observe {
								require.Positive(t, seen.Load())
							} else {
								require.Zero(t, seen.Load())
							}
						})
					}
				}
			}
			for _, chunk := range []int{0, 1, 7} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("session/chunk%d/d%v", chunk, deferred), func(t *testing.T) {
						s, err := NewProtocolSession(DefaultParserBudget())
						require.NoError(t, err)
						defer s.Close("done")
						s.(*captureSession).f.a.config.Deferred = deferred
						var events []*ProtocolEvent
						for i, w := range p.Events {
							wire, err := trafficfixture.ReadFile(strings.TrimPrefix(w.Record, "common/pcapx/pcaputil/"))
							require.NoError(t, err)
							step := chunk
							if step == 0 {
								step = len(wire)
							}
							for at := 0; at < len(wire); at += step {
								b := bytes.Clone(wire[at:min(at+step, len(wire))])
								r := s.Feed(w.Direction, time.Unix(int64(i+1), 0), b)
								if r.Err != nil {
									require.Equal(t, ErrNeedMore, r.Err.Kind)
									require.True(t, r.NeedMore)
								}
								events = append(events, r.Events...)
								for j := range b {
									b[j] ^= 255
								}
							}
						}
						require.Empty(t, s.Close("complete"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.Nil(t, s.Close("again"))
						check(t, events)
					})
				}
			}
		})
	}
}
