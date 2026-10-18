package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDHCPRefusalSealedMatrix(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6w := dhcp6OwnedInputs(t)
	for _, v6 := range []bool{false, true} {
		for _, kind := range []string{"pending", "first-refused", "foreign-client", "foreign-interface", "nonstandard", "duid-beyond-prefix", "marker-budget"} {
			if !v6 && (kind == "duid-beyond-prefix" || kind == "marker-budget") {
				continue
			}
			if v6 && kind == "nonstandard" {
				continue
			}
			t.Run(fmt.Sprintf("v6%t/%s", v6, kind), func(t *testing.T) {
				req, reply := v4[2], v4[3]
				qf, rf := dhcpOwnedFields(2), dhcpOwnedFields(3)
				if v6 {
					req, reply = v6w[2], v6w[3]
					qf, rf = dhcp6OwnedFields(t, 2), dhcp6OwnedFields(t, 3)
				}
				denied := bytes.Clone(req)
				wf := dhcpOwnedFields(2)
				if v6 {
					wf = dhcp6OwnedFields(t, 2)
				}
				if kind == "foreign-client" {
					if v6 {
						denied[21] ^= 1
						o := wf["Options"].([]map[string]any)[0]
						o["DUID"].([]byte)[13] ^= 1
						o["Raw"].([]byte)[13] ^= 1
					} else {
						denied[33] ^= 1
						wf["CHADDR"].([]byte)[5] ^= 1
					}
				}
				if v6 {
					extra := make([]byte, 300)
					if kind == "duid-beyond-prefix" || kind == "marker-budget" {
						extra = make([]byte, 260)
						denied = append(bytes.Clone(req[:4]), 0xfd, 0xe8, 1, 4)
						denied = append(denied, extra...)
						denied = append(denied, req[4:]...)
						wf["Options"] = append([]map[string]any{{"Code": uint16(65000), "Raw": bytes.Clone(extra)}}, wf["Options"].([]map[string]any)...)
					} else {
						denied = binary.BigEndian.AppendUint16(denied, 65000)
						denied = binary.BigEndian.AppendUint16(denied, 300)
						denied = append(denied, extra...)
						wf["Options"] = append(wf["Options"].([]map[string]any), map[string]any{"Code": uint16(65000), "Raw": bytes.Clone(extra)})
					}
				} else {
					denied = append(denied[:len(denied)-1], 12, 100)
					denied = append(denied, bytes.Repeat([]byte{'x'}, 100)...)
					denied = append(denied, 255)
					wf["Hostname"] = bytes.Repeat([]byte{'x'}, 100)
					wf["Option Count"] = 2
				}
				ps := []dhcpClockPacket{}
				rows := []map[string]any{}
				interfaces := []int{}
				add := func(w []byte, reverse bool, ts int64, fields, wireFields map[string]any, err string, tx, resp, iface int) {
					ps = append(ps, dhcpClockPacket{v6, reverse, ts, w})
					rows = append(rows, map[string]any{"raw_hex": hex.EncodeToString(w), "fields": fields, "wire_fields": wireFields, "error": err, "transaction_index": tx, "response_to_index": resp, "interface": iface, "timestamp": ts, "reverse": reverse})
					interfaces = append(interfaces, iface)
				}
				if kind != "first-refused" {
					add(req, false, 100, qf, qf, "", 0, -1, 0)
				}
				di := 0
				if kind == "foreign-interface" {
					di = 1
				}
				add(denied, false, 101, nil, wf, "ResourceExceeded", -1, -1, di)
				if kind == "first-refused" {
					add(req, false, 102, qf, qf, "", -1, -1, 0)
				}
				owner := -1
				if kind == "foreign-client" || kind == "foreign-interface" {
					owner = 0
				} else {
					rf["Association"] = "refused-client-xid"
				}
				add(reply, true, 103, rf, rf, "", owner, owner, 0)
				flat := dhcpClockCapture(t, ps)
				if kind == "nonstandard" {
					r, err := pcapgo.NewReader(bytes.NewReader(flat))
					require.NoError(t, err)
					var out bytes.Buffer
					pw := pcapgo.NewWriter(&out)
					require.NoError(t, pw.WriteFileHeader(65535, layers.LinkTypeEthernet))
					for i := range ps {
						raw, ci, err := r.ReadPacketData()
						require.NoError(t, err)
						p := gopacket.NewPacket(raw, layers.LinkTypeEthernet, gopacket.Default)
						eth := p.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
						ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
						u := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
						u.SrcPort, u.DstPort = 40000, 10067
						if ps[i].reverse {
							u.SrcPort, u.DstPort = u.DstPort, u.SrcPort
						}
						require.NoError(t, u.SetNetworkLayerForChecksum(ip))
						buf := gopacket.NewSerializeBuffer()
						require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, u, gopacket.Payload(ps[i].wire)))
						ci.Length, ci.CaptureLength = len(buf.Bytes()), len(buf.Bytes())
						require.NoError(t, pw.WritePacket(ci, buf.Bytes()))
					}
					flat = out.Bytes()
				}
				var ng bytes.Buffer
				nw, err := pcapgo.NewNgWriterInterface(&ng, pcapgo.NgInterface{LinkType: layers.LinkTypeEthernet, SnapLength: 65535, Name: "mvp0"}, pcapgo.NgWriterOptions{SectionInfo: pcapgo.NgSectionInfo{Application: "yaklang-dhcp-mvp"}})
				require.NoError(t, err)
				_, err = nw.AddInterface(pcapgo.NgInterface{LinkType: layers.LinkTypeEthernet, SnapLength: 65535, Name: "mvp1"})
				require.NoError(t, err)
				pr, err := pcapgo.NewReader(bytes.NewReader(flat))
				require.NoError(t, err)
				for i := range ps {
					raw, ci, err := pr.ReadPacketData()
					require.NoError(t, err)
					ci.InterfaceIndex = interfaces[i]
					require.NoError(t, nw.WritePacket(ci, raw))
				}
				require.NoError(t, nw.Flush())
				capture := ng.Bytes()
				name := fmt.Sprintf("v6%t-%s", v6, kind)
				literal, err := json.MarshalIndent(rows, "", "  ")
				require.NoError(t, err)
				if root := os.Getenv("DHCP_REFUSAL_EXPORT"); root != "" {
					dir := filepath.Join(root, "dhcp-refusal")
					require.NoError(t, os.MkdirAll(dir, 0700))
					require.NoError(t, os.WriteFile(filepath.Join(dir, name+".pcapng"), capture, 0600))
					require.NoError(t, os.WriteFile(filepath.Join(dir, name+".expected.json"), literal, 0600))
				} else {
					require.Equal(t, capture, finsReadBatch(t, "dhcp-refusal", "DHCP_REFUSAL_TEST_ROOT", name+".pcapng"))
					require.JSONEq(t, string(literal), string(finsReadBatch(t, "dhcp-refusal", "DHCP_REFUSAL_TEST_ROOT", name+".expected.json")))
				}
				for _, workers := range []int{1, 2, 4} {
					for _, deferred := range []bool{false, true} {
						for _, observe := range []bool{false, true} {
							t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
								var events, originals []*ProtocolEvent
								var stats ProtocolStats
								var seen atomic.Int64
								maxBuffered := 4096
								if kind == "marker-budget" {
									maxBuffered = 280
								}
								opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolBudget(256, maxBuffered), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
									snap := *e
									snap.Raw = bytes.Clone(e.Raw)
									snap.Session = cloneSession(e.Session)
									snap.Fields = cloneSession(e.Fields)
									snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
									events = append(events, &snap)
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
								a, err := NewPacketAnalyzer(opts...)
								require.NoError(t, err)
								r, err := NewCaptureReader(bytes.NewReader(capture))
								require.NoError(t, err)
								for {
									raw, ci, er := r.ReadPacketData()
									if er == io.EOF {
										break
									}
									require.NoError(t, er)
									require.NoError(t, a.Feed(raw, ci, r.LinkType()))
									clear(raw)
								}
								require.NoError(t, a.Close())
								require.NoError(t, a.Close())
								require.Len(t, events, len(rows))
								require.Zero(t, stats.BufferedBytes)
								require.Zero(t, stats.Malformed)
								require.EqualValues(t, len(rows), stats.Messages)
								require.EqualValues(t, len(denied), stats.LimitedBytes)
								if observe {
									require.EqualValues(t, len(rows), seen.Load())
								}
								ids := map[uint64]bool{}
								total := 0
								for i, e := range events {
									row := rows[i]
									wire := ps[i].wire
									total += len(wire)
									proto := "dhcp"
									if v6 {
										proto = "dhcpv6"
									}
									require.Equal(t, proto, e.Protocol)
									require.Equal(t, proto+"-native", e.Profile)
									require.Equal(t, "udp", e.Transport)
									complete := "message"
									if row["error"] != "" {
										complete = "limited"
									}
									if ps[i].reverse && row["response_to_index"].(int) >= 0 {
										complete = "transaction"
									}
									require.Equal(t, complete, e.Completeness)
									src, dst := "192.0.2.10:68", "192.0.2.67:67"
									if v6 {
										src, dst = "[2001:db8::10]:546", "[2001:db8::67]:547"
									}
									if kind == "nonstandard" {
										src, dst = "192.0.2.10:40000", "192.0.2.67:10067"
									}
									if ps[i].reverse {
										src, dst = dst, src
									}
									require.Equal(t, src, e.Source)
									require.Equal(t, dst, e.Destination)
									require.Zero(t, e.FlowID)
									require.Zero(t, e.Direction)
									require.Zero(t, e.Offset)
									require.NotZero(t, e.ID)
									require.False(t, ids[e.ID])
									ids[e.ID] = true
									require.Equal(t, len(wire), e.Length)
									require.True(t, time.Unix(ps[i].ts, 0).Equal(e.Timestamp), "capture must preserve the exact instant")
									require.Equal(t, interfaces[i], e.Domain.Interface)
									require.Len(t, e.SourceBytes.PacketRefs, 1)
									require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
									logical := wire
									if row["error"] != "" {
										logical = wire[:256]
									}
									require.Equal(t, "captured", e.SourceBytes.Kind)
									require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(logical)), e.SourceBytes.SHA256)
									tx, resp := row["transaction_index"].(int), row["response_to_index"].(int)
									if tx < 0 {
										require.Zero(t, e.TransactionID)
									} else {
										require.Equal(t, events[tx].ID, e.TransactionID)
									}
									if resp < 0 {
										require.Zero(t, e.ResponseTo)
									} else {
										require.Equal(t, events[resp].ID, e.ResponseTo)
									}
									er := row["error"].(string)
									if er != "" {
										require.Equal(t, wire[:256], e.Raw)
										require.Equal(t, "limited", e.Status)
										require.Nil(t, e.Session)
									} else {
										require.Equal(t, wire, e.Raw)
										require.Empty(t, e.Error)
										status := "decoded"
										if deferred {
											status = "deferred"
										}
										require.Equal(t, status, e.Status)
										require.Equal(t, row["fields"], e.Session)
									}
									for _, query := range []*ProtocolEvent{e, originals[i]} {
										f, err := query.GetFields()
										rocTypedError(t, er, err)
										if er != "" {
											require.Nil(t, f)
											continue
										}
										require.Equal(t, row["fields"], f)
										mutateRTPSQuery(f)
										tree, err := query.Decode()
										require.NoError(t, err)
										require.Equal(t, row["fields"], protocolFields(tree))
										require.Equal(t, row["fields"], tree["session"])
										mutateRTPSQuery(tree)
									}
								}
								require.EqualValues(t, total, stats.MessageBytes)
								require.EqualValues(t, total, stats.InputBytes)
								require.LessOrEqual(t, stats.PeakBufferedBytes, int64(maxBuffered))
								require.Zero(t, stats.CallbackPanics)
							})
						}
					}
				}
			})
		}
	}
}
