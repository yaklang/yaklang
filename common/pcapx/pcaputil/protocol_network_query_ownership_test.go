package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type networkOwnershipControl struct {
	ID          string         `json:"id"`
	Capture     string         `json:"capture"`
	SHA         string         `json:"sha256"`
	Protocol    string         `json:"protocol"`
	Source      string         `json:"source"`
	Destination string         `json:"destination"`
	Frame       string         `json:"frame_hex"`
	Fields      map[string]any `json:"fields"`
	Error       string         `json:"error"`
}

func corruptNetworkObservation(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			corruptNetworkObservation(v)
			x[k] = "forged"
		}
	case []map[string]any:
		for _, v := range x {
			corruptNetworkObservation(v)
		}
	case []any:
		for _, v := range x {
			corruptNetworkObservation(v)
		}
	case []byte:
		clear(x)
	}
}
func testNetworkObservationQueryOwnership(t *testing.T, arp bool) {
	if arp {
		t.Run("caller-provided-fields", func(t *testing.T) {
			for _, name := range []string{"arp", "icmp", "icmpv6"} {
				t.Run(name, func(t *testing.T) {
					want := map[string]any{"caller-cache": "preserved"}
					e := &ProtocolEvent{Protocol: name, Transport: "network", Fields: want}
					got, err := e.GetFields()
					require.NoError(t, err)
					require.Equal(t, want, got)
				})
			}
		})
	}
	b, err := trafficfixture.ReadFile("network-query-ownership/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []networkOwnershipControl }
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Len(t, doc.Cases, 11)
	var original struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(b, &original))
	for i, c := range doc.Cases {
		answer, err := trafficfixture.ReadFile("network-query-ownership/answers/" + c.ID + ".json")
		require.NoError(t, err)
		var got, want map[string]any
		require.NoError(t, json.Unmarshal(answer, &got))
		require.NoError(t, json.Unmarshal(original.Cases[i], &want))
		delete(want, "answer_sha256")
		require.Equal(t, want, got)
	}
	for _, c := range doc.Cases {
		if (c.Protocol == "arp") != arp {
			continue
		}
		if c.Error == "" {
			testNetworkObservationFrameBudget(t, c)
		}
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, observe := range []bool{false, true} {
					t.Run(fmt.Sprintf("private-network/%s/w%d/deferred%t/observe%t", c.ID, workers, deferred, observe), func(t *testing.T) {
						raw, err := trafficfixture.ReadFile("network-query-ownership/" + c.Capture)
						require.NoError(t, err)
						require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
						var events []*ProtocolEvent
						var stats ProtocolStats
						seen := 0
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
						if observe {
							opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen++ }))
						}
						require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
						clear(raw)
						require.Len(t, events, 1)
						require.Zero(t, stats.BufferedBytes)
						require.Zero(t, stats.CallbackPanics)
						if observe {
							require.Equal(t, 1, seen)
						}
						e := events[0]
						require.Equal(t, "network", e.Transport)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						f, err := e.GetFields()
						if c.Error != "" {
							require.EqualError(t, err, "protocol parser: no exact rule for this message")
							require.Nil(t, f)
							require.Equal(t, "ip", e.Protocol)
							require.Equal(t, "malformed", e.Status)
							require.Equal(t, c.Error, e.ExpertCode)
							require.Equal(t, c.Error, e.Error)
							require.Nil(t, e.Raw)
							require.Empty(t, e.Session)
							require.Zero(t, stats.Messages)
							return
						}
						require.NoError(t, err)
						require.Equal(t, c.Protocol, e.Protocol)
						require.Equal(t, c.Source, e.Source)
						require.Equal(t, c.Destination, e.Destination)
						require.Equal(t, doipDiscoveryWire(t, c.Frame), e.Raw)
						require.Equal(t, "decoded", e.Status)
						require.Equal(t, "message", e.Completeness)
						require.Empty(t, e.Error)
						require.EqualValues(t, 1, stats.Messages)
						rocEqualFields(t, c.Fields, f)
						rocEqualFields(t, c.Fields, e.Session)
						d, err := e.Decode()
						require.NoError(t, err)
						rocEqualFields(t, c.Fields, d["fields"].(map[string]any))
						rocEqualFields(t, c.Fields, d["session"].(map[string]any))
						corruptNetworkObservation(f)
						corruptNetworkObservation(d)
						corruptNetworkObservation(e.Session)
						corruptNetworkObservation(e.Fields)
						corruptNetworkObservation(e.Structured)
						clear(e.Raw)
						e.Session = map[string]any{"forged": true}
						e.Fields = map[string]any{"forged": true}
						e.Structured = map[string]any{"fields": "forged"}
						f, err = e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, c.Fields, f)
						d, err = e.Decode()
						require.NoError(t, err)
						rocEqualFields(t, c.Fields, d["fields"].(map[string]any))
						rocEqualFields(t, c.Fields, d["session"].(map[string]any))
					})
				}
			}
		}
	}
}

// Derive one over-budget frame and its inclusive normal neighbor from each
// immutable positive control. Only link padding changes, never IP/checksum or
// application bytes, so capture artifacts cannot be mistaken for malformed wire.
func testNetworkObservationFrameBudget(t *testing.T, c networkOwnershipControl) {
	raw, err := trafficfixture.ReadFile("network-query-ownership/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	reader, err := pcapgo.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())
	frame, ci, err := reader.ReadPacketData()
	require.NoError(t, err)
	limit := max(64, len(frame))
	admitted := append(bytes.Clone(frame), make([]byte, limit-len(frame))...)
	denied := append(bytes.Clone(admitted), 0)
	var capture bytes.Buffer
	writer := pcapgo.NewWriter(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for _, f := range [][]byte{denied, admitted} {
		ci.CaptureLength, ci.Length = len(f), len(f)
		require.NoError(t, writer.WritePacket(ci, f))
	}
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("network-budget/%s/w%d/deferred%t/observe%t", c.ID, workers, deferred, observe), func(t *testing.T) {
					var events []*ProtocolEvent
					var stats ProtocolStats
					seen := 0
					opts := []CaptureOption{WithProtocolBudget(limit, 1<<20), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
					if observe {
						opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen++ }))
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(capture.Bytes()), opts...))
					require.Len(t, events, 2)
					for i, e := range events {
						require.Equal(t, c.Protocol, e.Protocol)
						require.Equal(t, "network", e.Transport)
						require.Equal(t, c.Source, e.Source)
						require.Equal(t, c.Destination, e.Destination)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					}
					e := events[0]
					require.Equal(t, limit+1, e.Length)
					require.Equal(t, "limited", e.Status)
					require.Equal(t, "limited", e.Completeness)
					require.Equal(t, "ResourceExceeded", e.ExpertCode)
					require.Nil(t, e.Raw)
					require.Empty(t, e.Session)
					require.Empty(t, e.Structured)
					require.Empty(t, e.semanticFields)
					require.Empty(t, e.semanticSession)
					f, err := e.GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
					var returned *ProtocolError
					require.ErrorAs(t, err, &returned)
					returned.Kind = ErrMalformedMessage
					e.Fields = map[string]any{"forged": true}
					e.Structured = map[string]any{"fields": "forged"}
					e.Session = map[string]any{"forged": true}
					f, err = e.GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
					d, err := e.Decode()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, d)
					e = events[1]
					require.Equal(t, limit, e.Length)
					require.Equal(t, "decoded", e.Status)
					require.Equal(t, admitted, e.Raw)
					require.Empty(t, e.Error)
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Fields, f)
					rocEqualFields(t, c.Fields, e.Session)
					d, err = e.Decode()
					require.NoError(t, err)
					rocEqualFields(t, c.Fields, d["fields"].(map[string]any))
					rocEqualFields(t, c.Fields, d["session"].(map[string]any))
					require.EqualValues(t, 1, stats.Messages)
					require.EqualValues(t, 1, stats.Decoded)
					require.EqualValues(t, limit+1, stats.LimitedBytes)
					require.Zero(t, stats.BufferedBytes)
					require.Zero(t, stats.CallbackPanics)
					if observe {
						require.Equal(t, 2, seen)
					}
				})
			}
		}
	}
}
