package sharkcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func tlsEarlyPresentationTestEvent() *pcaputil.ProtocolEvent {
	return &pcaputil.ProtocolEvent{ID: 4, Protocol: "tls", Status: "decoded", Raw: append([]byte{23, 3, 3, 0, 21}, bytes.Repeat([]byte{0xa5}, 21)...), Session: map[string]any{
		"Record Type": byte(23), "Record Version": uint16(0x303), "Record Length": 21, "Negotiated Version": uint16(0x304), "Cipher Suite": uint16(0x1301),
		"Authentication Verified": true, "Content Visibility": "authenticated-plaintext", "Inner Content Type": byte(22), "TLS Key Epoch": "early", "Early Cipher Suite": uint16(0x1301), "Early Record Sequence": uint64(1), "Early Data Decision": "accepted by observed server", "Early Data Decision PDU": uint64(3),
		"Handshake Messages": []map[string]any{{"Type": byte(5), "Length": 0, "Authenticated": true, "End Of Early Data": "observed accepted early epoch ended; handshake traffic sequence starts independently", "Encoded End Of Early Data": []byte{5, 0, 0, 0}}},
	}}
}

func TestTLSEarlyDataProgressiveOwnedPlaintextAndContext(t *testing.T) {
	e := tlsEarlyPresentationTestEvent()
	require.Contains(t, secureTransportSummary(e), "End Of Early Data")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
		require.Len(t, view.sources, 2)
		require.Equal(t, e.Raw, view.sources[0].Data)
		require.Equal(t, []byte{5, 0, 0, 0}, view.sources[1].Data)
		require.Contains(t, view.sources[1].Name, "authenticated early-epoch handshake plaintext")
		encoded, decision, epoch := 0, 0, 0
		for _, f := range view.fields {
			if strings.HasPrefix(f.text, "Encoded End Of Early Data:") {
				encoded++
				require.True(t, f.hasRange)
				require.Equal(t, 2, f.source)
				require.Zero(t, f.start)
				require.Equal(t, 4, f.end)
			}
			if strings.HasPrefix(f.text, "Early Data Decision PDU:") {
				decision++
				require.False(t, f.hasRange)
				require.Zero(t, f.source)
			}
			if strings.HasPrefix(f.text, "TLS Key Epoch:") {
				epoch++
				require.Contains(t, f.text, "early")
				require.False(t, f.hasRange)
				require.Zero(t, f.source)
			}
		}
		require.Equal(t, 1, encoded)
		require.Equal(t, 1, decision)
		require.Equal(t, 1, epoch)
	}
}

func TestTLSEarlyDataCannotBorrowCiphertextRanges(t *testing.T) {
	for _, mode := range []string{"authentication", "handshake-authentication", "handshake-epoch", "version", "wrong-wire", "trailing-wire"} {
		t.Run(mode, func(t *testing.T) {
			e := tlsEarlyPresentationTestEvent()
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			switch mode {
			case "authentication":
				e.Session["Authentication Verified"] = false
			case "handshake-authentication":
				m["Authenticated"] = false
			case "handshake-epoch":
				e.Session["TLS Key Epoch"] = "handshake"
			case "version":
				e.Session["Negotiated Version"] = uint16(0x303)
			case "wrong-wire":
				m["Encoded End Of Early Data"] = []byte{14, 0, 0, 0}
			case "trailing-wire":
				m["Encoded End Of Early Data"] = []byte{5, 0, 0, 0, 0}
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
			for _, f := range fields {
				if strings.HasPrefix(f.text, "Encoded End Of Early Data:") {
					require.False(t, f.hasRange)
					require.Zero(t, f.source)
				}
			}
		})
	}
}

func TestTLSRejectedEarlyDataKeepsInspectablePlaintextWithoutAcceptance(t *testing.T) {
	e := tlsEarlyPresentationTestEvent()
	delete(e.Session, "Handshake Messages")
	data := []byte("GET /rejected HTTP/1.1\r\nHost: early.invalid\r\n\r\n")
	e.Session["Inner Content Type"] = byte(23)
	e.Session["Early Data Decision"] = "rejected by observed server"
	e.Session["Authenticated Plaintext"] = data
	e.Session["Application Decoding"] = "early data rejected by observed server; plaintext not delivered"
	require.Contains(t, secureTransportSummary(e), "Rejected Early Application Data")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.Len(t, sources, 2)
	require.Equal(t, data, sources[1].Data)
	require.NotEqual(t, e.Raw, sources[1].Data)
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Early Data Decision:") || strings.HasPrefix(f.text, "Application Decoding:") {
			require.Zero(t, f.source)
			require.False(t, f.hasRange)
		}
	}
}
