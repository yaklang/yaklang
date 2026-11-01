package sharkcli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// The actual worker for control398's valid NST displayed Base64 instead of
// octets and supplied no range. Packet authentication is a separate native gate.
func quicCryptoByteEvent() (*pcaputil.ProtocolEvent, []byte) {
	wire := []byte{0xff, 0, 0xc3, 0x80, 1, 0, 0x7f, 0xee}
	return &pcaputil.ProtocolEvent{Protocol: "quic", Status: "decoded", Raw: []byte{0x43, 0xc7, 0xfd}, Session: map[string]any{
		"Native Carrier": true, "Authentication Verified": true, "Packet Name": "1-RTT",
		"Frames": []map[string]any{{"Frame Type": "CRYPTO", "Offset": uint64(1234), "Length": uint64(len(wire)), "Crypto Data": wire}},
	}}, wire
}

func TestQUICCRYPTOByteSourceSurvivesWorkerIPC(t *testing.T) {
	for _, ipc := range []bool{false, true} {
		t.Run(fmt.Sprintf("ipc%v", ipc), func(t *testing.T) {
			e, expected := quicCryptoByteEvent()
			e = quicDiagnosticIPC(t, []*pcaputil.ProtocolEvent{e}, ipc)[0]
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			positions := map[string]int{}
			for i, f := range fields {
				for _, name := range []string{"Frame Type", "Offset", "Length", "Crypto Data"} {
					if strings.HasPrefix(f.text, name+":") {
						positions[name] = i
					}
				}
				if strings.HasPrefix(f.text, "Crypto Data:") {
					require.Contains(t, f.text, "8 bytes")
					require.Contains(t, f.text, "HEX ff 00 c3 80 01 00 7f ee")
					require.True(t, f.hasRange)
					require.Greater(t, f.source, 0, "ciphertext coordinates cannot describe plaintext")
					require.Equal(t, 0, f.start)
					require.Equal(t, len(expected), f.end)
					require.Equal(t, expected, sources[f.source-1].Data)
					require.Equal(t, "QUIC CRYPTO · packet AEAD verified", sources[f.source-1].Name)
				}
			}
			require.Len(t, positions, 4)
			require.Less(t, positions["Frame Type"], positions["Offset"])
			require.Less(t, positions["Offset"], positions["Length"])
			require.Less(t, positions["Length"], positions["Crypto Data"])
		})
	}
}

func TestQUICCRYPTOByteSourceNeedsAuthenticatedCompleteFrameVector(t *testing.T) {
	for _, mode := range []string{"not-authenticated", "missing-length", "wrong-length", "outside-frame"} {
		t.Run(mode, func(t *testing.T) {
			e, data := quicCryptoByteEvent()
			frame := e.Session["Frames"].([]map[string]any)[0]
			switch mode {
			case "not-authenticated":
				e.Session["Authentication Verified"] = false
			case "missing-length":
				delete(frame, "Length")
			case "wrong-length":
				frame["Length"] = uint64(len(data) + 1)
			case "outside-frame":
				delete(e.Session, "Frames")
				e.Session["Crypto Data"] = data
			}
			for _, ipc := range []bool{false, true} {
				fields, sources := protocolEventDetail(quicDiagnosticIPC(t, []*pcaputil.ProtocolEvent{e}, ipc))
				for _, source := range sources {
					require.NotContains(t, source.Name, "QUIC CRYPTO · packet AEAD verified")
				}
				for _, f := range fields {
					if strings.HasPrefix(f.text, "Crypto Data:") {
						require.False(t, f.hasRange)
					}
				}
			}
		})
	}
}
