package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestTLSPHAAdmissionFollowsBodyWithoutBorrowingWireRanges(t *testing.T) {
	for _, offer := range []string{"offered", "unknown"} {
		t.Run(offer, func(t *testing.T) {
			e := certificateRequestPresentation(t, true)
			message := e.Session["Handshake Messages"].([]map[string]any)[0]
			message["Certificate Request Admission"] = map[string]any{
				"Client PHA Offer": offer, "Context Uniqueness": "unique among observed requests", "Observed Request Count": uint64(2),
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				body, admission := -1, -1
				for i, field := range view.fields {
					if field.text == "Certificate Request body" {
						body = i
					}
					if strings.HasPrefix(field.text, "Certificate Request Admission") {
						admission = i
						require.True(t, field.branch)
						require.False(t, field.hasRange)
					}
				}
				require.GreaterOrEqual(t, body, 0)
				require.Greater(t, admission, body)
				for _, text := range []string{"Client PHA Offer: " + offer, "Context Uniqueness: unique among observed requests", "Observed Request Count: 2"} {
					count := 0
					for i, field := range view.fields {
						if field.text == text {
							count++
							require.Greater(t, i, admission)
							require.False(t, field.hasRange)
							require.Zero(t, field.source)
						}
					}
					require.Equal(t, 1, count, text)
				}
				require.Len(t, view.sources, 2)
				require.Equal(t, message["Encoded Certificate Request"], view.sources[1].Data)
			}
		})
	}
}
