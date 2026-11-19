package pcaputil

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// Round-trip the same snapshots pcapdb stores, including JSON numbers and
// base64 octets. No live flow, private event flags or prebuilt tree are inputs.
func TestMessageDisplayNodesFromStoredObservations(t *testing.T) {
	igmp := rfcHex("220012340000000201010002ef010203c0000201c0000202deadbeef06000000ef040506")
	events, _ := espNATTReplay(t, [][]byte{
		nativeIGMPFrame(t, igmp), nativeMLDFrame(t, nativeMLDReport(t)),
		ospfv2NativeFrame(t, ospfv2NativeHello(17, 2, 3), 0, false),
		ospfv3NativeFrame(ospfv3NativeHello(17, 2, 3), 89, nil),
		rsvpNativeFrame(t, rsvpNativeWire(10), false),
		pimControlFrame(t, []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105}, false),
		espNATTFrame(t, gtpv2NativeWire(7, 1), false, 40000, 2123),
		espNativeFrame(t, []byte{1, 2, 3, 4, 0, 0, 0, 5, 6}, false, layers.IPProtocolESP),
	})
	quic, _ := discoveryReplay(t, sessionDatagramPCAP(t, []sessionStep{{0, rfc9369ClientInitial()}, {1, rfc9369ServerInitial()}}, 14443), 1, false, false)
	events = append(events, quic...)
	require.Len(t, events, 10)
	for _, e := range events {
		t.Run(e.Protocol+"/"+e.Profile, func(t *testing.T) {
			fields := e.Fields
			if fields == nil {
				fields, _ = e.GetFields()
			}
			m := MessageDisplayInput{Protocol: e.Protocol, Status: e.Status, Profile: e.Profile, Transport: e.Transport,
				Prefix: e.Raw, Length: len(e.Raw), Source: e.SourceBytes, Session: e.Session, Fields: fields}
			b, err := json.Marshal(m)
			require.NoError(t, err)
			var fromDB MessageDisplayInput
			d := json.NewDecoder(bytes.NewReader(b))
			d.UseNumber()
			require.NoError(t, d.Decode(&fromDB))
			nodes, truncated := BuildMessageDisplayNodes(fromDB)
			want := BuildDisplayFieldTree(e)
			require.NotEmpty(t, want.Nodes, "fixture must have literal wire fields")
			require.Equal(t, want.Nodes, nodes)
			require.Equal(t, want.Truncated, truncated)
		})
	}
}

func TestMessageDisplayNodesBeyondPreview(t *testing.T) {
	// SMB range evidence lives in nested header snapshots. It is valid even
	// when prior messages used up the initial byte budget for this detail.
	m := MessageDisplayInput{Protocol: "smb", Length: 90, Session: map[string]any{"Header": map[string]any{
		"Command": json.Number("114"), "Flags": json.Number("1"),
		"Wire Field Ranges": map[string]any{
			"Command": []any{json.Number("8"), json.Number("9")},
			"Flags":   []any{json.Number("89"), json.Number("91")}, // outside the stored PDU
		},
	}}}
	nodes, truncated := BuildMessageDisplayNodes(m)
	require.False(t, truncated)
	require.Len(t, nodes, 1)
	leaves := displayNode(t, nodes[0].Children, "Decoded fields").Children
	require.Len(t, leaves, 1)
	require.Equal(t, "smb.cmd", leaves[0].Name)
	require.Equal(t, displayByteRange(8, 1), leaves[0].Ranges)

	// A lookalike record is not enough to reconstruct TLS range evidence.
	nodes, _ = BuildMessageDisplayNodes(MessageDisplayInput{Protocol: "tls", Prefix: []byte{22, 3, 3, 0, 5}, Length: 10,
		Fields: map[string]any{"Record Type": 23, "Record Version": 771, "Record Length": 5}})
	require.Empty(t, nodes)
}
