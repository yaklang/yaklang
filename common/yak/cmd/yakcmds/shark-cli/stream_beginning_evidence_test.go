package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func streamEvidenceTextForTest(u *tui) string {
	var text strings.Builder
	for _, row := range u.streamEvidenceRows() {
		text.WriteString(row.text)
		text.WriteByte('\n')
	}
	return text.String()
}

func streamPrefixAcceptedSMBForTest(t *testing.T) (*streamSnapshot, *pcaputil.ProtocolEvent) {
	t.Helper()
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	w, _ := smb1ResponseDisplaySource()
	r := s.Feed(0, time.Unix(100, 0), w)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.TCPFlowID = 17
	e.SourceBytes.CompletedIn = &pcaputil.PacketReference{Number: 1, Domain: e.Domain}
	e.SourceBytes.Kind = "reassembled"
	digest := sha256.Sum256(e.Raw)
	e.SourceBytes.SHA256 = hex.EncodeToString(digest[:])
	v := &streamSnapshot{nativeFlowID: 17, domain: e.Domain, endpoints: [2]string{e.Source, e.Destination}, prefix: [2][]byte{append(bytes.Clone(w), 1, 2, 3)}, events: []*pcaputil.ProtocolEvent{e}, protocolStack: []string{"TLSv1.3"}}
	return v, e
}

func TestStreamBeginningEvidenceUsesCompleteCurrentWirePDU(t *testing.T) {
	v, e := streamPrefixAcceptedSMBForTest(t)
	view := streamEvidenceTextForTest(&tui{stream: v})
	require.Contains(t, view, "Validated SMB1 PDU at captured offset 0")
	require.Contains(t, view, "SMB_COM_NEGOTIATE")
	require.NotContains(t, view, "Protocol at captured offset 0 is not identified")
	require.Contains(t, view, "actual TCP beginning unknown")
	require.Contains(t, view, "Raw first 32 bytes")
	// A connection or editable protocol label cannot override the literal SMB
	// header, whose typed source range has been checked against this PDU.
	e.Protocol = "http"
	view = streamEvidenceTextForTest(&tui{stream: v})
	require.Contains(t, view, "Validated SMB1 PDU")
	require.NotContains(t, view, "Validated HTTP PDU")
}

func TestStreamBeginningEvidenceRejectsBorrowedPartialOrDecryptedPDU(t *testing.T) {
	for _, name := range []string{"generation", "domain", "direction", "offset", "short-raw", "prefix", "digest", "parent", "parents", "decrypted", "header-only", "error", "no-completion", "no-native-flow", "later-PDU"} {
		t.Run(name, func(t *testing.T) {
			v, e := streamPrefixAcceptedSMBForTest(t)
			switch name {
			case "generation":
				e.TCPFlowID++
			case "domain":
				e.Domain.Interface++
			case "direction":
				e.Source, e.Destination = e.Destination, e.Source
			case "offset":
				e.Offset = 1
			case "short-raw":
				e.Raw = e.Raw[:len(e.Raw)-1]
			case "prefix":
				v.prefix[0][8] ^= 1
			case "digest":
				e.SourceBytes.SHA256 = strings.Repeat("0", 64)
			case "parent":
				e.SourceBytes.ParentPDU = 1
			case "parents":
				e.SourceBytes.ParentPDUs = []uint64{1}
			case "decrypted":
				e.SourceBytes.Kind = "decrypted"
			case "header-only":
				e.Completeness = "headers"
			case "error":
				e.Error = "refused"
			case "no-completion":
				e.SourceBytes.CompletedIn = nil
			case "no-native-flow":
				v.nativeFlowID = 0
			case "later-PDU":
				e.Offset = uint64(len(e.Raw))
				v.prefix[0] = append([]byte{0}, v.prefix[0]...)
			}
			view := streamEvidenceTextForTest(&tui{stream: v})
			require.NotContains(t, view, "Validated SMB1 PDU")
			require.Contains(t, view, "Protocol at captured offset 0 is not identified.")
		})
	}
}

func TestStreamBeginningEvidenceCannotPromoteEditedNativeRefusal(t *testing.T) {
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("FIN")
	w := smb1SetupDisplaySource(true)
	binary.LittleEndian.PutUint16(w[50:52], 0xd800)
	r := s.Feed(1, time.Unix(100, 0), w)
	require.NotNil(t, r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.TCPFlowID = 17
	e.Status, e.Error, e.Completeness = "decoded", "", "message"
	e.Fields = map[string]any{"Packet Name": "fake successful protocol"}
	e.Structured = map[string]any{"fields": e.Fields}
	e.SourceBytes.CompletedIn = &pcaputil.PacketReference{Number: 1, Domain: e.Domain}
	e.SourceBytes.Kind = "reassembled"
	digest := sha256.Sum256(e.Raw)
	e.SourceBytes.SHA256 = hex.EncodeToString(digest[:])
	v := &streamSnapshot{nativeFlowID: 17, domain: e.Domain, endpoints: [2]string{e.Source, e.Destination}, prefix: [2][]byte{bytes.Clone(e.Raw)}, events: []*pcaputil.ProtocolEvent{e}}
	view := streamEvidenceTextForTest(&tui{stream: v})
	require.NotContains(t, view, "Validated")
	require.Contains(t, view, "Protocol at captured offset 0 is not identified.")
}

func TestStreamBeginningEvidenceDoesNotImposeHTTPOnOtherProtocols(t *testing.T) {
	req, _ := smb1ResponseDisplaySource()
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"SMB1", req}, {"TLSv1.3", []byte{0x17, 3, 3, 0, 4, 0x11, 0x22, 0x33, 0x44}},
		{"SSH", []byte("SSH-2.0-source-test\r\n")}, {"gRPC", []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := bytes.Clone(tc.data)
			v := &streamSnapshot{protocolStack: []string{tc.name}, ports: [2]uint16{50000, 80}, prefix: [2][]byte{tc.data}}
			view := streamEvidenceTextForTest(&tui{stream: v})
			require.Contains(t, view, "Observed connection: "+tc.name)
			require.NotContains(t, view, "No complete, valid HTTP header")
			require.NotContains(t, view, "Verified HTTP at captured offset 0")
			require.Contains(t, view, "actual TCP beginning unknown")
			require.Contains(t, view, "Raw first 32 bytes")
			require.Equal(t, original, v.prefix[0])
		})
	}
}

func TestStreamBeginningEvidenceKeepsIndependentHTTPProof(t *testing.T) {
	header := "POST /first HTTP/1.1\r\nHost: source.test\r\nContent-Length: 10000\r\n\r\n"
	data := append([]byte(header), 0xff, 0, 'S', 'M', 'B')
	for _, connection := range []string{"SMB1", "TLSv1.3", "HTTP", ""} {
		v := &streamSnapshot{protocolStack: []string{connection}, ports: [2]uint16{50000, 445}, prefix: [2][]byte{data}}
		view := streamEvidenceTextForTest(&tui{stream: v})
		require.Contains(t, view, "Verified HTTP at captured offset 0")
		require.Contains(t, view, "Host: source.test")
		require.Contains(t, view, "Content-Length: 10000")
		require.Contains(t, view, "body bytes are not proof of HTTP")
	}
	for _, payload := range []string{"GET /", "GET / HTTP/1.1\r\nHost: source.test\r\n", "GET / HTTP/9.9\r\n\r\n", "\xff\x00\xaa"} {
		v := &streamSnapshot{protocol: "HTTP", evidence: "port hint", ports: [2]uint16{50000, 80}, prefix: [2][]byte{[]byte(payload)}}
		view := streamEvidenceTextForTest(&tui{stream: v})
		require.NotContains(t, view, "Verified HTTP at captured offset 0")
		require.NotContains(t, view, "Prefix signature: HTTP")
		require.Contains(t, view, "Protocol at captured offset 0 is not identified.")
	}
}
