package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type dhcpClockPacket struct {
	v6, reverse bool
	ts          int64
	wire        []byte
}

func dhcpClockCapture(t *testing.T, packets []dhcpClockPacket) []byte {
	t.Helper()
	var out bytes.Buffer
	w := pcapgo.NewWriter(&out)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for _, p := range packets {
		src, dst := "192.0.2.10", "192.0.2.67"
		sp, dp := uint16(68), uint16(67)
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
		if p.v6 {
			src, dst = "2001:db8::10", "2001:db8::67"
			sp, dp = 546, 547
			eth.EthernetType = layers.EthernetTypeIPv6
		}
		if p.reverse {
			src, dst = dst, src
			sp, dp = dp, sp
		}
		u := &layers.UDP{SrcPort: layers.UDPPort(sp), DstPort: layers.UDPPort(dp)}
		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
		if p.v6 {
			ip := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP(src), DstIP: net.ParseIP(dst)}
			require.NoError(t, u.SetNetworkLayerForChecksum(ip))
			require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, u, gopacket.Payload(p.wire)))
		} else {
			ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP(src).To4(), DstIP: net.ParseIP(dst).To4()}
			require.NoError(t, u.SetNetworkLayerForChecksum(ip))
			require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, u, gopacket.Payload(p.wire)))
		}
		raw := buf.Bytes()
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(p.ts, 0), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	return out.Bytes()
}

func TestDHCPContextMatrix(t *testing.T) {
	_, v4 := dhcpOwnedInputs(t)
	_, v6w := dhcp6OwnedInputs(t)
	for _, v6 := range []bool{false, true} {
		for _, kind := range []string{"adjacent", "foreign-client", "foreign-protocol", "same-owner-expired", "backwards-clock", "completed-reuse"} {
			t.Run(fmt.Sprintf("v6%t/%s", v6, kind), func(t *testing.T) {
				w := v4
				protocol := "dhcp"
				wantQ, wantR := dhcpOwnedFields(0), dhcpOwnedFields(1)
				if v6 {
					w = v6w
					protocol = "dhcpv6"
					wantQ, wantR = dhcp6OwnedFields(t, 0), dhcp6OwnedFields(t, 1)
				}
				if kind == "completed-reuse" {
					w = w[2:4]
					wantQ, wantR = dhcpOwnedFields(2), dhcpOwnedFields(3)
					if v6 {
						wantQ, wantR = dhcp6OwnedFields(t, 2), dhcp6OwnedFields(t, 3)
					}
				}
				ps := []dhcpClockPacket{{v6, false, 100, w[0]}}
				wants := []map[string]any{wantQ}
				protocols := []string{protocol}
				tx := []int{0}
				response := []int{-1}
				switch kind {
				case "completed-reuse":
					f := dhcpOwnedFields(3)
					if v6 {
						f = dhcp6OwnedFields(t, 3)
					}
					ps = append(ps, dhcpClockPacket{v6, true, 101, w[1]}, dhcpClockPacket{v6, false, 102, w[0]})
					wants = append(wants, f, wantQ)
					protocols = append(protocols, protocol, protocol)
					tx = append(tx, 0, -1)
					response = append(response, 0, -1)
				case "foreign-client":
					other := bytes.Clone(w[0])
					f := dhcpOwnedFields(0)
					if v6 {
						other[21] ^= 1
						f = dhcp6OwnedFields(t, 0)
						o := f["Options"].([]map[string]any)[0]
						o["Raw"].([]byte)[13] ^= 1
						o["DUID"].([]byte)[13] ^= 1
					} else {
						other[33] ^= 1
						f["CHADDR"].([]byte)[5] ^= 1
					}
					ps = append(ps, dhcpClockPacket{v6, false, 1000, other})
					wants = append(wants, f)
					protocols = append(protocols, protocol)
					tx = append(tx, 1)
					response = append(response, -1)
				case "foreign-protocol":
					other := v6w[0]
					f := dhcp6OwnedFields(t, 0)
					pr := "dhcpv6"
					if v6 {
						other = v4[0]
						f = dhcpOwnedFields(0)
						pr = "dhcp"
					}
					ps = append(ps, dhcpClockPacket{!v6, false, 1000, other})
					wants = append(wants, f)
					protocols = append(protocols, pr)
					tx = append(tx, 1)
					response = append(response, -1)
				case "backwards-clock":
					ps = append(ps, dhcpClockPacket{v6, false, 90, w[0]})
					wants = append(wants, wantQ)
					protocols = append(protocols, protocol)
					tx = append(tx, 0)
					response = append(response, -1)
				}
				ts := int64(101)
				owner := 0
				if kind == "completed-reuse" {
					ts = 103
					owner = -1
					wantR["Association"] = "ambiguous-client-xid"
				}
				if kind == "same-owner-expired" {
					ts = 131
					owner = -1
					wantR["Association"] = "missing-request"
				}
				ps = append(ps, dhcpClockPacket{v6, true, ts, w[1]})
				wants = append(wants, wantR)
				protocols = append(protocols, protocol)
				tx = append(tx, owner)
				response = append(response, owner)
				capture := dhcpClockCapture(t, ps)
				// Export only independently constructed inputs and literal RFC field answers,
				// never values obtained from the decoder under test.
				if root := os.Getenv("DHCP_CONTEXT_EXPORT"); root != "" {
					dir := filepath.Join(root, "dhcp-context")
					require.NoError(t, os.MkdirAll(dir, 0700))
					name := fmt.Sprintf("v6%t-%s", v6, kind)
					require.NoError(t, os.WriteFile(filepath.Join(dir, name+".pcap"), capture, 0600))
					rows := make([]map[string]any, len(ps))
					for i, p := range ps {
						rows[i] = map[string]any{"protocol": protocols[i], "timestamp": p.ts, "raw_hex": hex.EncodeToString(p.wire), "fields": wants[i], "response_to_index": response[i], "transaction_index": tx[i]}
					}
					b, err := json.MarshalIndent(rows, "", "  ")
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(dir, name+".expected.json"), b, 0600))
				}
				if os.Getenv("DHCP_CONTEXT_EXPORT") == "" {
					name := fmt.Sprintf("v6%t-%s", v6, kind)
					sealed := finsReadBatch(t, "dhcp-context", "DHCP_CONTEXT_TEST_ROOT", name+".pcap")
					require.Equal(t, capture, sealed)
					capture = sealed
					rows := make([]map[string]any, len(ps))
					for i, p := range ps {
						rows[i] = map[string]any{"protocol": protocols[i], "timestamp": p.ts, "raw_hex": hex.EncodeToString(p.wire), "fields": wants[i], "response_to_index": response[i], "transaction_index": tx[i]}
					}
					literal, err := json.Marshal(rows)
					require.NoError(t, err)
					require.JSONEq(t, string(literal), string(finsReadBatch(t, "dhcp-context", "DHCP_CONTEXT_TEST_ROOT", name+".expected.json")))
				}
				for _, workers := range []int{1, 2, 4} {
					for _, deferred := range []bool{false, true} {
						for _, observe := range []bool{false, true} {
							t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
								var packets atomic.Int64
								opts := []CaptureOption{WithProtocolDeferred(deferred)}
								if observe {
									opts = append(opts, WithEveryPacket(func(gopacket.Packet) { packets.Add(1) }))
								}
								ev, stats, err := binReplay(t, capture, workers, opts...)
								require.NoError(t, err)
								require.Len(t, ev, len(ps))
								require.Zero(t, stats.BufferedBytes)
								require.Zero(t, stats.Malformed)
								if observe {
									require.EqualValues(t, len(ps), packets.Load())
								}
								ids := map[uint64]bool{}
								for i, e := range ev {
									p := ps[i]
									require.Equal(t, protocols[i], e.Protocol)
									require.Equal(t, e.Protocol+"-native", e.Profile)
									require.Equal(t, "udp", e.Transport)
									require.Zero(t, e.FlowID)
									require.Zero(t, e.Direction)
									require.Zero(t, e.Offset)
									require.NotZero(t, e.ID)
									require.False(t, ids[e.ID])
									ids[e.ID] = true
									require.Equal(t, p.wire, e.Raw)
									require.Equal(t, len(p.wire), e.Length)
									require.Equal(t, time.Unix(p.ts, 0), e.Timestamp)
									require.Empty(t, e.Error)
									src, dst := "192.0.2.10:68", "192.0.2.67:67"
									if p.v6 {
										src, dst = "[2001:db8::10]:546", "[2001:db8::67]:547"
									}
									if p.reverse {
										src, dst = dst, src
									}
									require.Equal(t, src, e.Source)
									require.Equal(t, dst, e.Destination)
									require.Equal(t, CaptureDomain{}, e.Domain)
									require.Len(t, e.SourceBytes.PacketRefs, 1)
									require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
									if tx[i] < 0 {
										require.Zero(t, e.TransactionID)
									} else {
										require.Equal(t, ev[tx[i]].ID, e.TransactionID)
									}
									if response[i] < 0 {
										require.Zero(t, e.ResponseTo)
									} else {
										require.Equal(t, ev[response[i]].ID, e.ResponseTo)
									}
									complete := "message"
									if p.reverse && response[i] >= 0 && (p.v6 || wants[i]["Message Type"] == byte(5) || wants[i]["Message Type"] == byte(6)) {
										complete = "transaction"
									}
									require.Equal(t, complete, e.Completeness)
									require.Equal(t, wants[i], e.Session)
									f, err := e.GetFields()
									require.NoError(t, err)
									require.Equal(t, wants[i], f)
									mutateRTPSQuery(f)
									tree, err := e.Decode()
									require.NoError(t, err)
									require.Equal(t, wants[i], protocolFields(tree))
									require.Equal(t, wants[i], tree["session"])
									mutateRTPSQuery(tree)
									tlsDTLSPoison(e, "http")
									f, err = e.GetFields()
									require.NoError(t, err)
									require.Equal(t, wants[i], f)
									tree, err = e.Decode()
									require.NoError(t, err)
									require.Equal(t, wants[i], tree["session"])
								}
							})
						}
					}
				}
			})
		}
	}
}
