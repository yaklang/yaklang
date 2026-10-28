package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// Native capture tests live in pcaputil. The roadmap must retain the narrower
// library scope even when the old scorecard expected every P0/P1 row "done".
// Run the independent complete/malformed wire checks before allowing these
// explicitly documented partial rows; no generic partial-status exemption.
func requireObservedSessionScopes(t *testing.T, priority string) map[string]bool {
	t.Helper()
	covered := map[string]bool{}
	if priority == priP1 {
		boundedESP := t.Run("ESPHeaderCanonicalBoundedScope", TestESPCanonicalHeaderEntryAndLegacyCompatibility)
		for _, name := range []string{"IPsec ESP", "IPsec ESP Header Fields"} {
			for _, c := range ProtocolCatalog {
				if c.Name == name {
					require.Equal(t, statusPartial, c.Status)
					require.NotEmpty(t, c.Notes)
				}
			}
			for _, r := range ProtocolRoadmap {
				if r.Name == name {
					require.Equal(t, stPartial, r.Status)
					require.NotEmpty(t, r.Notes)
				}
			}
			covered[name] = boundedESP
		}
		// This is an explicit historical false-completion correction. It
		// certifies the boundary of outer SSH support, not an SFTP grammar.
		covered["SFTP"] = t.Run("SFTPOuterSSHScope", TestSFTPOuterSSHScopeDoesNotClaimSubsystem)
		boundedSASL := t.Run("IMAPSASLCanonicalBoundedScope", TestIMAPSASLCanonicalBoundedScope)
		for _, name := range []string{"IMAP SASL Response", "IMAP SASL Challenge", "IMAP SASL Initial Value"} {
			covered[name] = boundedSASL
		}
	}
	for name, pri := range map[string]string{"AJP": priP0, "BGP": priP1, "RTMP": priP1, "L2TP": priP1, "STP": priP1, "RSTP": priP1} {
		if pri != priority {
			continue
		}
		catalog, roadmap := false, false
		for _, c := range ProtocolCatalog {
			if c.Name == name {
				catalog = true
				require.Equal(t, statusPartial, c.Status)
				require.NotEmpty(t, c.Notes)
			}
		}
		for _, r := range ProtocolRoadmap {
			if r.Name == name {
				roadmap = true
				require.Equal(t, stPartial, r.Status)
				require.Equal(t, pri, r.Priority)
				require.NotEmpty(t, r.Notes)
			}
		}
		require.True(t, catalog)
		require.True(t, roadmap)
		if name == "STP" || name == "RSTP" {
			covered[name] = t.Run(name+"CanonicalBoundedScope", TestSTPCanonicalWireEntries)
		} else if name == "L2TP" {
			covered[name] = t.Run("L2TPv3CanonicalBoundedScope", TestL2TPv3CanonicalWireEntries)
		} else {
			covered[name] = t.Run(name+"CanonicalBoundedScope", TestObservedSessionCanonicalEntries)
		}
	}
	return covered
}

func TestSFTPOuterSSHScopeDoesNotClaimSubsystem(t *testing.T) {
	sc, ok := ResolveP1Scorecard("SFTP")
	require.True(t, ok)
	require.Equal(t, "L4", sc.SampleClass)
	require.False(t, sc.G5, "outer SSH captures cannot establish SFTP application coverage")
	require.Zero(t, sc.Schema+sc.Traffic+sc.Tests+sc.Branches+sc.Stack)
	for _, r := range ProtocolRoadmap {
		if r.Name == "SFTP" {
			require.Equal(t, stPartial, r.Status)
			require.Contains(t, r.Notes, "not decoded")
		}
	}
	for _, c := range ProtocolCatalog {
		require.NotEqual(t, "SFTP", c.Name, "no SFTP entry may be synthesized from outer transport identity")
	}
	n := parseRule(t, sshDirectKEX(), "application-layer.ssh", "SSHPacket")
	require.Equal(t, uint64(20), uintVal(t, mustChild(t, n, "Payload", "Message Number")))
	require.Nil(t, n.Child("SFTP"))
}

func TestObservedSessionCanonicalEntries(t *testing.T) {
	bgp := append(bytes.Repeat([]byte{255}, 16), 0, 19, 4)
	handshake := make([]byte, 1537)
	handshake[0] = 3
	chunk := []byte{2, 0, 0, 0, 0, 0, 4, 1, 0, 0, 0, 0, 0, 0, 0, 128}
	for _, c := range []struct {
		rule, entry string
		wire        []byte
		key         string
		value       any
	}{
		{"bgp", "BGPMessageFields", bgp, "Type", byte(4)},
		{"application-layer.ajp", "AJPMessageFields", []byte{0x12, 0x34, 0, 1, 10}, "Code", byte(10)},
		{"application-layer.ajp", "AJPRequestBodyFields", []byte{0x12, 0x34, 0, 5, 0, 3, 1, 2, 3}, "Body Length", 3},
		{"rtmp", "RTMPInitialHandshakeFields", handshake, "Version", byte(3)},
		{"rtmp", "RTMPInitialChunkFields", chunk, "Message Type", byte(1)},
		{"rtmp", "AMF0MessageFields", []byte{2, 0, 1, 'x', 5}, "Values", nil},
	} {
		t.Run(c.entry, func(t *testing.T) {
			v, err := ParseStructured(c.wire, c.rule, c.entry)
			require.NoError(t, err)
			fields := v["fields"].(map[string]any)["Message"].(map[string]any)
			if c.value != nil {
				require.Equal(t, c.value, fields[c.key])
			} else {
				require.Len(t, fields[c.key], 2)
			}
			truncated := c.wire[:len(c.wire)-1]
			if c.entry == "AMF0MessageFields" {
				truncated = []byte{2, 0, 2, 'x'}
			}
			for _, bad := range [][]byte{truncated, append(append([]byte(nil), c.wire...), 0xff)} {
				_, err = ParseStructured(bad, c.rule, c.entry)
				require.Error(t, err)
			}
		})
	}
	// Sixteen-bit extended attribute length must not be read as a byte.
	w := append(bytes.Repeat([]byte{255}, 16), 0, 0, 2, 0, 0, 0, 0)
	attrs := append([]byte{0xd0, 8, 1, 0}, make([]byte, 256)...)
	binary.BigEndian.PutUint16(w[21:], uint16(len(attrs)))
	w = append(w, attrs...)
	binary.BigEndian.PutUint16(w[16:], uint16(len(w)))
	_, err := ParseStructured(w, "bgp", "BGPMessageFields")
	require.NoError(t, err)
}
