package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func dhcpOwnedInputs(t *testing.T) ([]byte, [][]byte) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("testdata/protocol-sessions/dhcp-discover-ack.pcap")
	require.NoError(t, err)
	require.Equal(t, "e179b89c12a6163b2c11b75f7bb0811b9d59cdc3611782ee89ba6e3781d63dff", fmt.Sprintf("%x", sha256.Sum256(raw)))
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
		u, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.True(t, ok)
		wires = append(wires, bytes.Clone(u.Payload))
	}
	require.Len(t, wires, 4)
	// RFC2131 Figure1 and RFC2132 9.6 supply literal field offsets and
	// message types. This historical fixture observes zero addresses; its
	// Offer/Ack is not evidence of a valid lease or a successful DHCP endpoint.
	for i, typ := range []byte{1, 2, 3, 5} {
		w := make([]byte, 240)
		w[0], w[1], w[2] = []byte{1, 2, 1, 2}[i], 1, 6
		copy(w[4:8], []byte{0x12, 0x34, 0x56, 0x78})
		copy(w[28:34], []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc})
		copy(w[236:240], []byte{0x63, 0x82, 0x53, 0x63})
		w = append(w, 53, 1, typ, 255)
		require.Equal(t, w, wires[i])
	}
	return raw, wires
}

func dhcpOwnedFields(index int) map[string]any {
	out := map[string]any{
		"Message Type": []byte{1, 2, 3, 5}[index], "Packet Name": []string{"Discover", "Offer", "Request", "Ack"}[index],
		"Xid": uint32(0x12345678), "CHADDR": []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		"Client IP": "0.0.0.0", "Offered IP": "0.0.0.0", "Relay IP": "0.0.0.0",
		"Observation": "unverified-lease", "Options Overload": byte(0), "Option Count": 1,
	}
	if index == 1 || index == 3 {
		out["Association"] = "observed-client-xid"
	}
	return out
}

func dhcpAssertOwnedQuery(t *testing.T, e *ProtocolEvent, want map[string]any, wire []byte) {
	t.Helper()
	require.Equal(t, "dhcp", e.Protocol)
	require.Equal(t, "dhcp-native", e.Profile)
	require.Equal(t, "udp", e.Transport)
	require.Empty(t, e.Error)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, len(wire), e.Length)
	require.Equal(t, want, e.Session)
	f, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, f)
	f["Xid"] = uint32(0)
	clear(f["CHADDR"].([]byte))
	tree, err := e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, protocolFields(tree))
	protocolFields(tree)["Packet Name"] = "changed"
	tree["session"].(map[string]any)["Observation"] = "changed"
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

func testDHCPOwnedQueries(t *testing.T) {
	capture, wires := dhcpOwnedInputs(t)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("workers%d/deferred%t/observe%t", workers, deferred, observe), func(t *testing.T) {
					var packets atomic.Int64
					opts := []CaptureOption{WithProtocolDeferred(deferred)}
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { packets.Add(1) }))
					}
					events, stats, err := binReplay(t, capture, workers, opts...)
					require.NoError(t, err)
					require.Len(t, events, 4)
					require.Zero(t, stats.Malformed)
					require.Zero(t, stats.BufferedBytes)
					ids := map[uint64]bool{}
					for i, e := range events {
						require.NotZero(t, e.ID)
						require.False(t, ids[e.ID])
						ids[e.ID] = true
						require.Equal(t, events[0].ID, e.TransactionID)
						if i == 1 || i == 3 {
							require.Equal(t, events[0].ID, e.ResponseTo)
						} else {
							require.Zero(t, e.ResponseTo)
						}
						if i == 3 {
							require.Equal(t, "transaction", e.Completeness)
						} else {
							require.Equal(t, "message", e.Completeness)
						}
						// Native UDP capture projections are per datagram; roles
						// come from the exact endpoints, not TCP flow directions.
						require.Zero(t, e.Direction)
						endpoints := []string{"192.0.2.1:40000", "192.0.2.2:10067"}
						require.Equal(t, endpoints[i%2], e.Source)
						require.Equal(t, endpoints[1-i%2], e.Destination)
						require.Equal(t, time.Unix(1700000000, int64(i)*1234), e.Timestamp)
						require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wires[i])), e.SourceBytes.SHA256)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						dhcpAssertOwnedQuery(t, e, dhcpOwnedFields(i), wires[i])
					}
					if observe {
						require.EqualValues(t, 4, packets.Load())
					}
				})
			}
		}
	}
	for _, deferred := range []bool{false, true} {
		for _, kind := range []string{"missing-END", "option-value-truncated", "option-budget"} {
			t.Run(fmt.Sprintf("deferred%t/%s", deferred, kind), func(t *testing.T) {
				budget := DefaultParserBudget()
				bad, errorKind := bytes.Clone(wires[0][:243]), "MalformedMessage"
				if kind == "option-value-truncated" {
					bad = bytes.Clone(wires[0][:242])
				} else if kind == "option-budget" {
					budget.MaxCollectionElements = 1
					bad = append(bytes.Clone(wires[0][:243]), 12, 1, 'a', 255)
					errorKind = "ResourceExceeded"
				}
				s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(40000, 10067))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				out := s.Feed(0, time.Unix(1, 0), bad)
				rocTypedError(t, errorKind, out.Err)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				f, err := e.GetFields()
				rocTypedError(t, errorKind, err)
				require.Nil(t, f)
				require.Nil(t, e.Session)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				require.Equal(t, bad, e.Raw)
				assertOwnedProtocolRefusal(t, e, errorKind, err)
				// A refusal must not create a pending client/xid. A subsequent
				// offer is observed but cannot match the malformed request.
				offer := s.Feed(1, time.Unix(2, 0), wires[1])
				require.Nil(t, offer.Err)
				require.Len(t, offer.Events, 1)
				require.Zero(t, offer.Events[0].ResponseTo)
				require.Zero(t, offer.Events[0].TransactionID)
				want := dhcpOwnedFields(1)
				want["Association"] = "missing-request"
				dhcpAssertOwnedQuery(t, offer.Events[0], want, wires[1])
				next := s.Feed(0, time.Unix(3, 0), wires[0])
				require.Nil(t, next.Err)
				require.Len(t, next.Events, 1)
				require.Equal(t, next.Events[0].ID, next.Events[0].TransactionID)
				dhcpAssertOwnedQuery(t, next.Events[0], dhcpOwnedFields(0), wires[0])
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
