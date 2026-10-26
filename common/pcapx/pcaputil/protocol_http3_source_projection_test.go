package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHTTP3FrameScalarProjectionDoesNotDuplicateRootSummaries(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	headers := h3RequestHeaders()
	quicSourceFrame(t, q, 1, 0, string(headers))
	fr := quicSourceFrame(t, q, 2, uint64(len(headers)), string(h3Data([]byte("abc"))))
	e := &ProtocolEvent{Protocol: "http3", Session: map[string]any{"Frames": []map[string]any{fr}, "HTTP3 Frames": fr["HTTP3 Frames"]}}
	require.Equal(t, []any{uint64(0)}, e.DisplayFields()["http3.frame_type"])
	require.Equal(t, []any{uint64(3)}, e.DisplayFields()["http3.frame_length"])
}
func TestHTTP3ResumedHeaderScalarsUseApplicationFramesWithoutCiphertextRanges(t *testing.T) {
	e := &ProtocolEvent{Protocol: "http3", Raw: []byte("ciphertext"), Session: map[string]any{"QPACK Resumed Streams": []map[string]any{{"HTTP3 Frames": []map[string]any{{"Frame Type Value": uint64(1<<60) + 17, "Payload Length": uint64(7)}}}}}}
	s := e.DisplayFieldValues()
	require.Equal(t, []any{uint64(1<<60) + 17}, s.FieldMap()["http3.frame_type"])
	for _, v := range s.Values {
		if v.Name == "http3.frame_type" {
			require.Equal(t, "1152921504606846993", v.ExactValue)
			require.Nil(t, v.Range, "application header is not a range in the ciphertext event.Raw")
		}
	}
}
