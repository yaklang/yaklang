package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

func dhcp6LiteralOption(t *testing.T, code uint16, wire string, fields map[string]any) map[string]any {
	t.Helper()
	b, err := hex.DecodeString(wire)
	require.NoError(t, err)
	out := map[string]any{"Code": code, "Raw": b}
	for k, v := range fields {
		out[k] = v
	}
	return out
}

// Literal answers for the selected historical IA_PD exchange. RFC8415
// sections8,11.2,21.1/2/12/21/22 give the field widths and meanings; no
// runtime decoder is used to construct the expected option graph. An observed
// prefix/lifetime is not evidence of allocation, reachability or authentication.
func dhcp6OwnedFields(t *testing.T, index int) map[string]any {
	t.Helper()
	duid := func(server bool) map[string]any {
		code, wire := uint16(1), "000100011c39cf88080027fe8f95"
		if server {
			code, wire = 2, "000100011c3825e8080027d410bb"
		}
		b, err := hex.DecodeString(wire)
		require.NoError(t, err)
		return dhcp6LiteralOption(t, code, wire, map[string]any{"DUID Type": uint16(1), "DUID": b})
	}
	oro := func() map[string]any { return dhcp6LiteralOption(t, 6, "00170018", nil) }
	elapsed := func() map[string]any { return dhcp6LiteralOption(t, 8, "0000", nil) }
	ia := func() map[string]any {
		head := []string{"27fe8f9500000e1000001518", "27fe8f950000000000000000", "27fe8f9500000e1000001518", "27fe8f950000000000000000", "27fe8f950000000000000000"}[index]
		var children []map[string]any
		tail := ""
		if index != 0 {
			prefix := []string{"", "0000119400001c2040200100000000fe000000000000000000", "00001c2000001d4c40200100000000fe000000000000000000", "0000119400001c2040200100000000fe000000000000000000", "000000000000000040200100000000fe000000000000000000"}[index]
			pref := []uint32{0, 4500, 7200, 4500, 0}[index]
			valid := []uint32{0, 7200, 7500, 7200, 0}[index]
			children = []map[string]any{dhcp6LiteralOption(t, 26, prefix, map[string]any{"Prefix Length": byte(64), "Prefix": "2001:0:0:fe00::", "Preferred Lifetime": pref, "Valid Lifetime": valid, "Options": []map[string]any(nil)})}
			tail = "001a0019" + prefix
		}
		t1, t2 := uint32(0), uint32(0)
		if index == 0 || index == 2 {
			t1, t2 = 3600, 5400
		}
		return dhcp6LiteralOption(t, 25, head+tail, map[string]any{"IAID": uint32(0x27fe8f95), "T1": t1, "T2": t2, "Options": children})
	}
	var options []map[string]any
	switch index {
	case 0:
		options = []map[string]any{duid(false), oro(), elapsed(), ia()}
	case 1, 3:
		options = []map[string]any{ia(), duid(false), duid(true)}
	case 2, 4:
		options = []map[string]any{duid(false), duid(true), oro(), elapsed(), ia()}
	case 5:
		options = []map[string]any{duid(false), duid(true), dhcp6LiteralOption(t, 13, "000052656c656173652072656365697665642e", map[string]any{"Status": uint16(0), "Message": "Release received."})}
	}
	out := map[string]any{"Message Type": []byte{1, 2, 3, 7, 8, 7}[index], "Observation": "observed", "Transaction ID": []uint32{0x100874, 0x100874, 0x49174e, 0x49174e, 0xc789b0, 0xc789b0}[index], "Options": options}
	if index%2 == 1 {
		out["Association"] = "observed-client-xid"
	}
	return out
}

func dhcp6OwnedInputs(t *testing.T) ([]byte, [][]byte) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("testdata/protocol-sessions/first-batch-m1/dhcpv6-native.pcap")
	require.NoError(t, err)
	require.Equal(t, "4ae07cbcfdd3fb616c1c051945e931576778057762576510c54ebea52c636c25", fmt.Sprintf("%x", sha256.Sum256(raw)))
	r, err := pcapgo.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	var wires [][]byte
	for {
		packet, _, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		p := gopacket.NewPacket(packet, r.LinkType(), gopacket.Default)
		if u, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP); ok {
			wires = append(wires, bytes.Clone(u.Payload))
		}
	}
	require.Len(t, wires, 6)
	return raw, wires
}

func dhcp6AssertOwnedQuery(t *testing.T, e *ProtocolEvent, want map[string]any, wire []byte) {
	t.Helper()
	require.Equal(t, "dhcpv6", e.Protocol)
	require.Equal(t, "dhcpv6-native", e.Profile)
	require.Equal(t, "udp", e.Transport)
	require.Empty(t, e.Error)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, len(wire), e.Length)
	require.Equal(t, want, e.Session)
	f, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, f)
	mutateRTPSQuery(f)
	tree, err := e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, protocolFields(tree))
	mutateRTPSQuery(tree)
	mutateRTPSQuery(e.Session)
	mutateRTPSQuery(e.Fields)
	mutateRTPSQuery(e.Structured)
	again, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, again)
	tree, err = e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, tree["session"])
	require.Equal(t, wire, e.Raw)
}

func testDHCP6OwnedQueries(t *testing.T) {
	capture, wires := dhcp6OwnedInputs(t)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%t/observe%t", workers, deferred, observe), func(t *testing.T) {
					var packets atomic.Int64
					opts := []CaptureOption{WithProtocolDeferred(deferred)}
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { packets.Add(1) }))
					}
					all, stats, err := binReplay(t, capture, workers, opts...)
					require.NoError(t, err)
					require.Zero(t, stats.Malformed)
					require.Zero(t, stats.BufferedBytes)
					var events []*ProtocolEvent
					for _, e := range all {
						if e.Protocol == "dhcpv6" {
							events = append(events, e)
						}
					}
					require.Len(t, events, 6)
					ids := map[uint64]bool{}
					for i, e := range events {
						require.NotZero(t, e.ID)
						require.False(t, ids[e.ID])
						ids[e.ID] = true
						require.Equal(t, events[i-i%2].ID, e.TransactionID)
						if i%2 == 1 {
							require.Equal(t, events[i-1].ID, e.ResponseTo)
							require.Equal(t, "transaction", e.Completeness)
						} else {
							require.Zero(t, e.ResponseTo)
							require.Equal(t, "message", e.Completeness)
						}
						require.Zero(t, e.Direction)
						require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wires[i])), e.SourceBytes.SHA256)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.Equal(t, []uint64{2, 5, 7, 8, 11, 12}[i], e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						seconds := []int64{1420235564, 1420235564, 1420235565, 1420235565, 1420235569, 1420235569}[i]
						nanos := []int64{775688000, 777375000, 837792000, 839157000, 892611000, 893300000}[i]
						require.Equal(t, time.Unix(seconds, nanos), e.Timestamp)
						if i%2 == 0 {
							require.Equal(t, "[fe80::a00:27ff:fefe:8f95]:546", e.Source)
							require.Equal(t, "[ff02::1:2]:547", e.Destination)
						} else {
							require.Equal(t, "[fe80::a00:27ff:fed4:10bb]:547", e.Source)
							require.Equal(t, "[fe80::a00:27ff:fefe:8f95]:546", e.Destination)
						}
						dhcp6AssertOwnedQuery(t, e, dhcp6OwnedFields(t, i), wires[i])
					}
					if observe {
						require.EqualValues(t, 12, packets.Load())
					}
				})
			}
		}
	}
	for _, deferred := range []bool{false, true} {
		for _, kind := range []string{"option-length", "option-header", "option-budget"} {
			t.Run(fmt.Sprintf("deferred%t/%s", deferred, kind), func(t *testing.T) {
				budget := DefaultParserBudget()
				bad := bytes.Clone(wires[0][:len(wires[0])-1])
				errorKind := "MalformedMessage"
				if kind == "option-header" {
					bad = append(bytes.Clone(wires[0]), 0)
				} else if kind == "option-budget" {
					budget.MaxCollectionElements = 1
					bad = bytes.Clone(wires[0])
					errorKind = "ResourceExceeded"
				}
				s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(546, 547))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				out := s.Feed(0, time.Unix(1, 0), bad)
				require.Len(t, out.Events, 1)
				rocTypedError(t, errorKind, out.Err)
				e := out.Events[0]
				f, err := e.GetFields()
				require.Nil(t, f)
				rocTypedError(t, errorKind, err)
				// Preserve the established public partial-header diagnostic. It cannot
				// become a successful field query or create an association from bad bytes.
				require.Equal(t, map[string]any{"Message Type": byte(1), "Observation": "observed", "Transaction ID": uint32(0x100874), "Options": []map[string]any(nil)}, e.Session)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				require.Equal(t, bad, e.Raw)
				require.Zero(t, s.Stats().BufferedBytes)
				assertOwnedProtocolRefusal(t, e, errorKind, err)
				// Under limit1, a client-DUID-only Solicit is the adjacent native
				// observation. It tests recovery, not a full IA_PD endpoint exchange.
				good, want := wires[0], dhcp6OwnedFields(t, 0)
				if kind == "option-budget" {
					good = bytes.Clone(wires[0][:22])
					want["Options"] = want["Options"].([]map[string]any)[:1]
				}
				next := s.Feed(0, time.Unix(2, 0), good)
				require.Nil(t, next.Err)
				require.Len(t, next.Events, 1)
				require.Equal(t, next.Events[0].ID, next.Events[0].TransactionID)
				dhcp6AssertOwnedQuery(t, next.Events[0], want, good)
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
