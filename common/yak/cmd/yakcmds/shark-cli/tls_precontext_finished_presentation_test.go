package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func precontextFinishedPresentation() *pcaputil.ProtocolEvent {
	wire := append([]byte{20, 0, 0, 12}, bytes.Repeat([]byte{0x44}, 12)...)
	sum := sha256.Sum256([]byte("owned earlier ciphertext"))
	return &pcaputil.ProtocolEvent{ID: 9, Protocol: "tls", Status: "decoded", Raw: []byte{23, 3, 3, 0, 1, 42}, Session: map[string]any{
		"Negotiated Version": uint16(0x303), "Cipher Suite": uint16(0xc02f), "Authentication Verified": true,
		"Precontext Finished Recovery Verified": true, "Precontext Finished Original PDUs": []uint64{7}, "Precontext Finished Original Record SHA256": []string{hex.EncodeToString(sum[:])},
		"Recovered Context Handshake Messages": []map[string]any{{"Type": byte(20), "Length": 12, "Authenticated": true, "Finished Phase": "handshake", "Finished Version": uint16(0x303), "Finished Cipher Suite": uint16(0xc02f), "Encoded Finished": wire, "Finished Verify Data": bytes.Clone(wire[4:])}},
	}}
}

func TestTLSPrecontextFinishedSupplementHasDistinctOwnedSource(t *testing.T) {
	e := precontextFinishedPresentation()
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for _, d := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
		require.Len(t, d.sources, 2)
		require.Contains(t, d.sources[1].Name, "recovered earlier authenticated handshake plaintext")
		require.Equal(t, append([]byte{20, 0, 0, 12}, bytes.Repeat([]byte{0x44}, 12)...), d.sources[1].Data)
		found := false
		for _, f := range d.fields {
			if strings.HasPrefix(f.text, "Finished Verify Data:") {
				found = true
				require.True(t, f.hasRange)
				require.Equal(t, 2, f.source)
				require.Equal(t, 4, f.start)
				require.Equal(t, 16, f.end)
			}
		}
		require.True(t, found)
		require.Contains(t, detailLines(d.fields), "Recovered earlier Finished")
	}
}

func TestTLSPrecontextFinishedInvalidManifestCannotAttachSource(t *testing.T) {
	for _, kind := range []string{"unverified", "empty-ids", "current-id", "duplicate-id", "digest", "count", "body", "auth", "cipher"} {
		t.Run(kind, func(t *testing.T) {
			e := precontextFinishedPresentation()
			s := e.Session
			m := mapList(s["Recovered Context Handshake Messages"])[0]
			switch kind {
			case "unverified":
				s["Precontext Finished Recovery Verified"] = false
			case "empty-ids":
				s["Precontext Finished Original PDUs"] = []uint64{}
			case "current-id":
				s["Precontext Finished Original PDUs"] = []uint64{9}
			case "duplicate-id":
				s["Precontext Finished Original PDUs"] = []uint64{7, 7}
			case "digest":
				s["Precontext Finished Original Record SHA256"] = []string{"invalid"}
			case "count":
				s["Precontext Finished Original Record SHA256"] = []string{}
			case "body":
				m["Finished Verify Data"] = []byte{1}
			case "auth":
				m["Authenticated"] = false
			case "cipher":
				m["Finished Cipher Suite"] = uint16(0xc030)
			}
			_, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
		})
	}
}
