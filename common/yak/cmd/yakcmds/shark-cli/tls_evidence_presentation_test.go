package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func streamPrefixTLSForTest(t *testing.T, wire []byte, malformed bool) (*streamSnapshot, *pcaputil.ProtocolEvent) {
	t.Helper()
	s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionPorts(40000, 443))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	r := s.Feed(0, time.Unix(100, 0), wire)
	if malformed {
		require.NotNil(t, r.Err)
	} else {
		require.Nil(t, r.Err)
	}
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	require.Equal(t, "tls", e.Protocol)
	e.TCPFlowID = 17
	e.SourceBytes.Kind = "reassembled"
	e.SourceBytes.CompletedIn = &pcaputil.PacketReference{Number: 1, Domain: e.Domain}
	e.SourceBytes.PacketRefs = []pcaputil.PacketReference{{Number: 1, Domain: e.Domain}}
	digest := sha256.Sum256(e.Raw)
	e.SourceBytes.SHA256 = hex.EncodeToString(digest[:])
	v := &streamSnapshot{nativeFlowID: 17, domain: e.Domain, endpoints: [2]string{e.Source, e.Destination}, prefix: [2][]byte{bytes.Clone(wire)}, events: []*pcaputil.ProtocolEvent{e}, protocolStack: []string{"TLSv1.3"}}
	return v, e
}

func TestStreamBeginningEvidenceAcceptsOwnedTLSEnvelope(t *testing.T) {
	v, e := streamPrefixTLSForTest(t, []byte{21, 3, 3, 0, 2, 1, 0}, false)
	require.Equal(t, "envelope", e.Completeness)
	for _, label := range []string{"tls", "http"} {
		e.Protocol = label
		text := streamEvidenceTextForTest(&tui{stream: v})
		require.Contains(t, text, "Validated TLS record PDU at captured offset 0")
		require.Contains(t, text, "Alert")
		require.NotContains(t, text, "Validated TLSv1.3")
		require.NotContains(t, text, "Protocol at captured offset 0 is not identified.")
	}
}

func TestStreamBeginningEvidenceRejectsUnprovenTLSEnvelope(t *testing.T) {
	for _, name := range []string{"type", "version", "length", "foreign-completion", "foreign-reference", "header-only", "refused", "short-ciphertext", "other-envelope"} {
		t.Run(name, func(t *testing.T) {
			wire := []byte{21, 3, 3, 0, 2, 1, 0}
			if name == "refused" {
				wire = []byte{20, 3, 3, 0, 2, 0, 0} // CCS must contain exactly one byte with value one.
			}
			if name == "short-ciphertext" {
				wire = []byte{23, 3, 3, 0, 1, 0}
			}
			v, e := streamPrefixTLSForTest(t, wire, name == "refused")
			switch name {
			case "type", "version", "length":
				e.Raw = bytes.Clone(e.Raw)
				at := map[string]int{"type": 0, "version": 2, "length": 4}[name]
				e.Raw[at] ^= 1
				v.prefix[0] = bytes.Clone(e.Raw)
				digest := sha256.Sum256(e.Raw)
				e.SourceBytes.SHA256 = hex.EncodeToString(digest[:])
			case "foreign-completion":
				e.SourceBytes.CompletedIn.Domain.Interface++
			case "foreign-reference":
				e.SourceBytes.PacketRefs[0].Domain.Interface++
			case "header-only":
				e.Completeness = "headers"
			case "refused":
				e.Status, e.Error, e.Completeness = "decoded", "", "envelope"
				e.Fields = map[string]any{"Record Type": byte(20), "Record Version": uint16(0x303), "Record Length": 2, "Authentication Verified": true}
				e.Session, e.Structured = e.Fields, map[string]any{"fields": e.Fields}
			case "other-envelope":
				v, e = streamPrefixAcceptedSMBForTest(t)
				e.Completeness = "envelope"
			}
			require.NotContains(t, streamEvidenceTextForTest(&tui{stream: v}), "Validated")
		})
	}
}

// This checks trusted detail/worker presentation, not synthetic cryptographic acceptance.
func TestTLSRecordDetailsShowDistinctSources(t *testing.T) {
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: []byte{23, 3, 3, 0, 1, 42}, Fields: map[string]any{"ZZZ remainder": "preserved"}, Session: map[string]any{
		"Record Type": byte(23), "Record Version": uint16(0x303), "Record Length": 1,
		"Negotiated Version": uint16(0x304), "Cipher Suite": uint16(0x1301), "SNI": "source.test", "ALPN": "h2",
		"Content Visibility": "authenticated-plaintext", "Authentication Verified": true, "Inner Content Type": byte(22), "Certificate Trust": "not-evaluated",
		"Session Establishment": map[string]any{"Evidence Scope": "retained hello"},
	}}
	before, err := json.Marshal(e)
	require.NoError(t, err)
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	views := []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)}
	for _, view := range views {
		positions := map[string]int{}
		for i, f := range view.fields {
			if f.text == "Record header" || f.text == "Record decoding" || f.text == "Observed connection" {
				positions[f.text] = i
				require.Equal(t, 1, f.depth)
				require.False(t, f.hasRange)
			}
			for name, span := range map[string][2]int{"Record Type": {0, 1}, "Record Version": {1, 3}, "Record Length": {3, 5}} {
				if strings.HasPrefix(f.text, name+":") {
					require.True(t, f.hasRange, f.text)
					require.Equal(t, 1, f.source)
					require.Equal(t, span[0], f.start)
					require.Equal(t, span[1], f.end)
					require.Equal(t, 2, f.depth)
				}
			}
			for _, name := range []string{"Negotiated Version", "Cipher Suite", "SNI", "ALPN", "Inner Content Type", "Record AEAD verified", "Certificate Trust"} {
				if strings.HasPrefix(f.text, name+":") {
					require.False(t, f.hasRange, f.text)
					require.Zero(t, f.source, f.text)
				}
			}
		}
		require.Len(t, positions, 3)
		require.Less(t, positions["Record header"], positions["Record decoding"])
		require.Less(t, positions["Record decoding"], positions["Observed connection"])
		require.Len(t, view.sources, 1)
		require.Equal(t, e.Raw, view.sources[0].Data)
		require.Contains(t, detailLines(view.fields), "ZZZ remainder: preserved")
	}
	after, err := json.Marshal(e)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
