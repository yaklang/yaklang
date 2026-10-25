package sharkcli

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestISISProgressiveIdentityAndNestedSNPAByteRanges(t *testing.T) {
	w, err := hex.DecodeString("831b01000f01000001010001000100001e0023010100010001000106060025456017c0")
	require.NoError(t, err)
	m, err := stream_parser.DecodeISISWire(w, 64)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "isis", Transport: "ethernet", Status: "decoded", Raw: w, Session: m, Summary: "IS-IS L1 LAN Hello"}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "IS-IS", Layers: []string{"Ethernet", "LLC", "IS-IS"}})
	require.Equal(t, "IS-IS Hello", r.Protocol)
	require.Equal(t, []string{"Ethernet", "LLC", "IS-IS Hello"}, r.ProtocolStack)
	require.Equal(t, "IS-IS L1 LAN Hello", r.Info)
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	seen := map[string]bool{}
	for _, f := range fs {
		require.NotContains(t, f.text, "Wire Field Ranges")
		for name, octets := range map[string]string{"Source System ID: 0100.0100.0100": "010001000100", "SNPA: 00:25:45:60:17:c0": "0025456017c0"} {
			if strings.HasPrefix(f.text, name) {
				require.True(t, f.hasRange, name)
				require.Equal(t, octets, hex.EncodeToString(sources[f.source-1].Data[f.start:f.end]))
				seen[name] = true
			}
		}
	}
	require.Len(t, seen, 2)
	term := searchTerm{kind: "proto", value: "isis"}
	require.True(t, term.matches(nil, r, nil))
}

func TestISISKeepsMalformedBodySeparateFromCompleteHeader(t *testing.T) {
	w, err := hex.DecodeString("831b010012010000001b04b0010001000100000000000001dd1d01")
	require.NoError(t, err)
	w[23] ^= 1
	m, err := stream_parser.DecodeISISWire(w, 64)
	require.Error(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "isis", Profile: "isis-six-octet-independent-advertisement", Transport: "ethernet", Status: "malformed", Raw: w, Session: m, Summary: "IS-IS L1 Link State · invalid checksum"}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "LLC", Layers: []string{"Ethernet", "LLC"}})
	require.Equal(t, "IS-IS LSP", r.Protocol)
	require.Contains(t, r.Info, "invalid checksum")
	require.Equal(t, "malformed", r.ProtocolStatus)
	// A damaged declaration has no independently complete fixed header.
	w[9]--
	e.Session, err = stream_parser.DecodeISISWire(w, 64)
	require.Error(t, err)
	r = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "LLC", Layers: []string{"Ethernet", "LLC"}})
	require.NotEqual(t, "IS-IS LSP", r.Protocol)
}
