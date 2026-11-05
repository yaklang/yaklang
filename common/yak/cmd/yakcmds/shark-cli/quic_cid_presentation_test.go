package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Projection fixtures do not claim native AEAD verification. Real packet
// protection and admission are exercised by the independent native tests.
func TestQUICRejectedCIDFieldsKeepWireOrderAndAdmissionSeparate(t *testing.T) {
	for _, frameType := range []string{"NEW_CONNECTION_ID", "RETIRE_CONNECTION_ID"} {
		for _, ipc := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ipc%v", frameType, ipc), func(t *testing.T) {
				e := quicCoalescedDiagnosticEvents(string(pcaputil.ErrMalformedMessage), "QUIC CID reset token changed")[2]
				frame := map[string]any{"Frame Type": frameType, "Sequence": uint64(1)}
				if frameType == "NEW_CONNECTION_ID" {
					frame["Retire Prior To"], frame["CID"], frame["Reset Token"] = uint64(0), []byte("offered8"), bytes.Repeat([]byte{0x21}, 16)
				}
				e.Session["CID Frame Admission"] = "refused; prior CID, stream and TLS state retained"
				e.Session["Rejected CID Frames"] = []map[string]any{frame}
				require.NotContains(t, e.Session, "Frames")
				before, err := json.Marshal(e)
				require.NoError(t, err)
				events := quicDiagnosticIPC(t, []*pcaputil.ProtocolEvent{e}, ipc)
				fields, sources := protocolEventDetail(events)
				text := detailLines(fields)
				require.Contains(t, text, "CID Frame Admission: refused; prior CID, stream and TLS state retained")
				require.Contains(t, text, "Rejected CID Frames")
				require.Less(t, strings.Index(text, "Frame Type: "+frameType), strings.Index(text, "Sequence: 1"))
				if frameType == "NEW_CONNECTION_ID" {
					require.Less(t, strings.Index(text, "Sequence: 1"), strings.Index(text, "Retire Prior To: 0"))
					require.Less(t, strings.Index(text, "Retire Prior To: 0"), strings.Index(text, "CID: 8 bytes"))
					require.Less(t, strings.Index(text, "CID: 8 bytes"), strings.Index(text, "Reset Token: 16 bytes"))
				}
				for _, f := range fields {
					if strings.HasPrefix(f.text, "CID:") || strings.HasPrefix(f.text, "Reset Token:") || strings.HasPrefix(f.text, "Sequence:") {
						require.False(t, f.hasRange, "CID fields have no ciphertext byte offsets")
					}
					if strings.HasPrefix(f.text, "Rejected Packet Payload:") {
						require.True(t, f.hasRange)
						require.Equal(t, e.Session["Rejected Packet Payload"], sources[f.source-1].Data[f.start:f.end])
					}
				}
				after, err := json.Marshal(e)
				require.NoError(t, err)
				require.Equal(t, before, after)
			})
		}
	}
}
