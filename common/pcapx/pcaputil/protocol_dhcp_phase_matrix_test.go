package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type dhcpPhaseStep struct {
	wire     []byte
	fields   map[string]any
	err      string
	tx, resp int
	ts       int64
	reverse  bool
}
type dhcpPhaseCase struct {
	name  string
	v6    bool
	steps []dhcpPhaseStep
}

func dhcpPhaseControls(t *testing.T) []dhcpPhaseCase {
	_, v4 := dhcpOwnedInputs(t)
	_, v6 := dhcp6OwnedInputs(t)
	var out []dhcpPhaseCase
	for _, six := range []bool{false, true} {
		wire := func(i int) []byte {
			if six {
				return bytes.Clone(v6[i])
			}
			return bytes.Clone(v4[i])
		}
		fields := func(i int) map[string]any {
			if six {
				return dhcp6OwnedFields(t, i)
			}
			return dhcpOwnedFields(i)
		}
		makeStep := func(i int, ts int64, tx, resp int) dhcpPhaseStep {
			return dhcpPhaseStep{wire(i), fields(i), "", tx, resp, ts, i%2 == 1}
		}
		add := func(name string, steps ...dhcpPhaseStep) {
			out = append(out, dhcpPhaseCase{fmt.Sprintf("v6%t-%s", six, name), six, steps})
		}
		q, r := makeStep(2, 100, 0, -1), makeStep(3, 101, 0, 0)
		add("ordinary", q, r)
		changedQuery := makeStep(2, 101, -1, -1)
		if six {
			changedQuery.wire = append(changedQuery.wire, 0xfd, 0xe8, 0, 1, 1)
			changedQuery.fields["Options"] = append(changedQuery.fields["Options"].([]map[string]any), map[string]any{"Code": uint16(65000), "Raw": []byte{1}})
		} else {
			changedQuery.wire = append(changedQuery.wire[:len(changedQuery.wire)-1], 12, 3, 'a', 'l', 't', 255)
			changedQuery.fields["Hostname"] = []byte("alt")
			changedQuery.fields["Option Count"] = 2
		}
		ambiguous := makeStep(3, 102, -1, -1)
		ambiguous.fields["Association"] = "ambiguous-client-xid"
		add("changed-nonelapsed-query", q, changedQuery, ambiguous)
		discovery, repeated, offered := makeStep(0, 100, 0, -1), makeStep(0, 129, 0, -1), makeStep(1, 131, 0, 0)
		if six {
			repeated.wire = dhcpPhaseReplace6Option(t, repeated.wire, 8, []byte{0, 29})
			for _, o := range repeated.fields["Options"].([]map[string]any) {
				if o["Code"] == uint16(8) {
					o["Raw"] = []byte{0, 29}
				}
			}
		} else {
			repeated.wire[9] = 29
		}
		add("discovery-retry-renewal", discovery, repeated, offered)

		retry := makeStep(2, 129, 0, -1)
		late := makeStep(3, 131, 0, 0)
		add("exact-retry-renewal", q, retry, late)
		changed := makeStep(2, 129, 0, -1)
		if six {
			changed.wire = dhcpPhaseReplace6Option(t, changed.wire, 8, []byte{0, 1})
			for _, o := range changed.fields["Options"].([]map[string]any) {
				if o["Code"] == uint16(8) {
					o["Raw"] = []byte{0, 1}
				}
			}
		} else {
			changed.wire[9] = 29
		}
		add("elapsed-retry-renewal", q, changed, late)
		backward := makeStep(2, 90, 0, -1)
		add("backward-retry", q, backward, r)
		expired := makeStep(3, 131, -1, -1)
		expired.fields["Association"] = "missing-request"
		add("expired-response", q, expired)
		again := makeStep(3, 101, 0, 0)
		add("future-response-does-not-age-timely-response", q, dhcpPhaseStep{wire(3), func() map[string]any { f := fields(3); f["Association"] = "missing-request"; return f }(), "", -1, -1, 1000, true}, again)
		selectedQ, selectedR := makeStep(2, 100, 0, -1), makeStep(3, 101, 0, 0)
		if !six {
			selectedQ.wire = dhcpPhaseAddServer(selectedQ.wire, [4]byte{192, 0, 2, 67})
			selectedR.wire = dhcpPhaseAddServer(selectedR.wire, [4]byte{192, 0, 2, 67})
			selectedQ.fields["Server Identifier"], selectedQ.fields["Option Count"] = "192.0.2.67", 2
			selectedR.fields["Server Identifier"], selectedR.fields["Option Count"] = "192.0.2.67", 2
		}
		add("selected-server", selectedQ, selectedR)
		bad := makeStep(3, 101, -1, -1)
		bad.fields["Association"] = "server-mismatch"
		if six {
			bad.wire = dhcpPhaseChange6Option(t, bad.wire, 2)
			for _, o := range bad.fields["Options"].([]map[string]any) {
				if o["Code"] == uint16(2) {
					o["Raw"].([]byte)[13] ^= 1
					o["DUID"].([]byte)[13] ^= 1
				}
			}
		} else {
			bad.wire = dhcpPhaseAddServer(bad.wire, [4]byte{192, 0, 2, 68})
			bad.fields["Server Identifier"], bad.fields["Option Count"] = "192.0.2.68", 2
		}
		good := selectedR
		good.ts = 102
		add("mismatched-server-keeps-owner", selectedQ, bad, good)
		future := bad
		future.ts = 1000
		add("foreign-server-clock", selectedQ, future, selectedR)
		missing := makeStep(3, 101, -1, -1)
		missing.fields["Association"] = "missing-server-identifier"
		if six {
			w := missing.wire
			for p := 4; p+4 <= len(w); {
				c, n := binary.BigEndian.Uint16(w[p:]), int(binary.BigEndian.Uint16(w[p+2:]))
				if c == 2 {
					missing.wire = append(bytes.Clone(w[:p]), w[p+4+n:]...)
					break
				}
				p += 4 + n
			}
			opts := missing.fields["Options"].([]map[string]any)
			missing.fields["Options"] = opts[:len(opts)-1]
		}
		add("missing-selected-server", selectedQ, missing)
		if !six {
			add("dora", makeStep(0, 100, 0, -1), makeStep(1, 101, 0, 0), makeStep(2, 102, 0, -1), makeStep(3, 103, 0, 2))
			offer := makeStep(1, 131, -1, -1)
			offer.fields["Association"] = "missing-request"
			add("expired-offer-keeps-request", makeStep(0, 100, 0, -1), makeStep(2, 129, 0, -1), offer, makeStep(3, 131, 0, 1))
			badRole := makeStep(2, 100, -1, -1)
			badRole.wire[0] = 2
			badRole.fields = nil
			badRole.err = "MalformedMessage"
			add("bootp-role-refused", badRole)
		} else {
			for _, rapid := range []bool{false, true} {
				query, response := makeStep(0, 100, 0, -1), makeStep(1, 101, 0, 0)
				response.wire[0] = 7
				response.fields["Message Type"] = byte(7)
				if rapid {
					for _, s := range []*dhcpPhaseStep{&query, &response} {
						s.wire = append(s.wire, 0, 14, 0, 0)
						s.fields["Options"] = append(s.fields["Options"].([]map[string]any), map[string]any{"Code": uint16(14), "Raw": []byte{}})
					}
				} else {
					response.tx, response.resp = -1, -1
					response.fields["Association"] = "missing-request-phase"
				}
				add(fmt.Sprintf("rapid-commit-%t", rapid), query, response)
			}
			for _, kind := range []string{"client-duplicate", "server-duplicate", "elapsed-width", "rapid-width"} {
				s := makeStep(2, 100, -1, -1)
				switch kind {
				case "client-duplicate":
					s.wire = append(s.wire, v6[2][4:22]...)
				case "server-duplicate":
					s.wire = append(s.wire, v6[2][22:40]...)
				case "elapsed-width":
					s.wire = dhcpPhaseReplace6Option(t, s.wire, 8, []byte{0, 0, 0})
				case "rapid-width":
					s.wire = append(s.wire, 0, 14, 0, 1, 0)
				}
				s.fields = nil
				s.err = "MalformedMessage"
				add(kind, s)
			}
		}
	}
	return out
}
func TestDHCPPhaseSealedMatrix(t *testing.T) {
	controls := dhcpPhaseControls(t)
	require.Len(t, controls, 33)
	for _, c := range controls {
		t.Run(c.name, func(t *testing.T) {
			ps := make([]dhcpClockPacket, len(c.steps))
			rows := make([]map[string]any, len(c.steps))
			for i, s := range c.steps {
				ps[i] = dhcpClockPacket{c.v6, s.reverse, s.ts, s.wire}
				rows[i] = map[string]any{"raw_hex": hex.EncodeToString(s.wire), "fields": s.fields, "error": s.err, "transaction_index": s.tx, "response_to_index": s.resp, "timestamp": s.ts, "reverse": s.reverse}
			}
			capture := dhcpClockCapture(t, ps)
			literal, err := json.MarshalIndent(rows, "", "  ")
			require.NoError(t, err)
			if root := os.Getenv("DHCP_PHASE_EXPORT"); root != "" {
				p := filepath.Join(root, "dhcp-phase")
				require.NoError(t, os.MkdirAll(p, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(p, c.name+".pcap"), capture, 0600))
				require.NoError(t, os.WriteFile(filepath.Join(p, c.name+".expected.json"), literal, 0600))
			} else {
				require.Equal(t, capture, finsReadBatch(t, "dhcp-phase", "DHCP_PHASE_TEST_ROOT", c.name+".pcap"))
				require.JSONEq(t, string(literal), string(finsReadBatch(t, "dhcp-phase", "DHCP_PHASE_TEST_ROOT", c.name+".expected.json")))
			}
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var seen atomic.Int64
							events, originals, stats, err := dhcpPhaseReplay(t, capture, workers, deferred, observe, &seen)
							require.NoError(t, err)
							require.Len(t, events, len(c.steps))
							require.Zero(t, stats.BufferedBytes)
							if observe {
								require.EqualValues(t, len(c.steps), seen.Load())
							}
							ids := map[uint64]bool{}
							total, malformed := 0, 0
							for i, e := range events {
								s := c.steps[i]
								total += len(s.wire)
								proto := "dhcp"
								if c.v6 {
									proto = "dhcpv6"
								}
								require.Equal(t, proto, e.Protocol)
								require.Equal(t, proto+"-native", e.Profile)
								require.Equal(t, "udp", e.Transport)
								require.Zero(t, e.FlowID)
								require.Zero(t, e.Direction)
								require.Zero(t, e.Offset)
								require.Equal(t, s.wire, e.Raw)
								require.Equal(t, len(s.wire), e.Length)
								require.True(t, time.Unix(s.ts, 0).Equal(e.Timestamp))
								require.Equal(t, CaptureDomain{}, e.Domain)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(s.wire)), e.SourceBytes.SHA256)
								require.NotZero(t, e.ID)
								require.False(t, ids[e.ID])
								ids[e.ID] = true
								src, dst := "192.0.2.10:68", "192.0.2.67:67"
								if c.v6 {
									src, dst = "[2001:db8::10]:546", "[2001:db8::67]:547"
								}
								if s.reverse {
									src, dst = dst, src
								}
								require.Equal(t, src, e.Source)
								require.Equal(t, dst, e.Destination)
								if s.tx < 0 {
									require.Zero(t, e.TransactionID)
								} else {
									require.Equal(t, events[s.tx].ID, e.TransactionID)
								}
								if s.resp < 0 {
									require.Zero(t, e.ResponseTo)
								} else {
									require.Equal(t, events[s.resp].ID, e.ResponseTo)
								}
								if s.err != "" {
									malformed++
									require.Equal(t, "malformed", e.Status)
									require.Equal(t, "malformed", e.Completeness)
									if c.v6 {
										require.Equal(t, map[string]any{"Message Type": byte(3), "Observation": "observed", "Transaction ID": uint32(0x49174e), "Options": []map[string]any(nil)}, e.Session)
									} else {
										require.Nil(t, e.Session)
									}
								} else {
									require.Equal(t, s.fields, e.Session)
									complete := "message"
									if s.resp >= 0 && (c.v6 || s.fields["Message Type"] == byte(5) || s.fields["Message Type"] == byte(6)) {
										complete = "transaction"
									}
									require.Equal(t, complete, e.Completeness)
								}
								for j := 0; j < 2; j++ {
									f, er := e.GetFields()
									rocTypedError(t, s.err, er)
									if s.err == "" {
										require.Equal(t, s.fields, f)
										mutateRTPSQuery(f)
										tree, er := e.Decode()
										require.NoError(t, er)
										require.Equal(t, s.fields, protocolFields(tree))
										require.Equal(t, s.fields, tree["session"])
										mutateRTPSQuery(tree)
									} else {
										require.Nil(t, f)
										tree, er := e.Decode()
										rocTypedError(t, s.err, er)
										require.Nil(t, tree)
									}
									tlsDTLSPoison(e, "http")
								}
								f, er := originals[i].GetFields()
								rocTypedError(t, s.err, er)
								if s.err == "" {
									require.Equal(t, s.fields, f)
									tree, er := originals[i].Decode()
									require.NoError(t, er)
									require.Equal(t, s.fields, tree["session"])
								} else {
									require.Nil(t, f)
								}
							}
							require.EqualValues(t, total, stats.MessageBytes)
							require.EqualValues(t, total, stats.InputBytes)
							require.EqualValues(t, malformed, stats.Malformed)
						})
					}
				}
			}
		})
	}
}

func dhcpPhaseReplay(t *testing.T, raw []byte, workers int, deferred, observe bool, seen *atomic.Int64) ([]*ProtocolEvent, []*ProtocolEvent, ProtocolStats, error) {
	t.Helper()
	var snapshots, originals []*ProtocolEvent
	var stats ProtocolStats
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
		snap := *e
		snap.Raw = bytes.Clone(e.Raw)
		snap.Session = cloneSession(e.Session)
		snap.Fields = cloneSession(e.Fields)
		snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
		snapshots = append(snapshots, &snap)
		originals = append(originals, e)
		if observe {
			tlsDTLSPoison(e, "http")
			e.ID = 0
			e.FlowID = 99
			e.Source = "caller"
		}
	})}
	if observe {
		opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
	}
	err := ReplayPcap(bytes.NewReader(raw), opts...)
	return snapshots, originals, stats, err
}
