package pcaputil

import (
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Valid outer GBT envelopes do not implicitly grant the new plaintext
// multiresult capability to a shared inner decoder or a service-group owner.
func TestDLMSWrapperSNMultiResultOuterOnly(t *testing.T) {
	for _, v := range []struct{ name, apdu, message string }{
		{"Read-multi-block-results", "0c020200000102110702010002021108", "DLMS Wrapper SN service-block selected single-result count1 only"},
		{"Write-mixed-block-number", "0d03000103020001", "DLMS Wrapper SN Write selected response success/error choices0/1 only"},
	} {
		inner := wrapperWire(t, v.apdu)
		gbt := append([]byte{0xe0, 0x81, 0, 1, 0, 0, byte(len(inner))}, inner...)
		wire := append([]byte{0, 1, 0, 1, 0, 16, 0, 0}, gbt...)
		binary.BigEndian.PutUint16(wire[6:8], uint16(len(gbt)))
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				chunks := []int{0, 1, 7}
				if tr == "udp" {
					chunks = []int{0}
				}
				for _, chunk := range chunks {
					t.Run(fmt.Sprintf("%s/%s/d%t/chunk%d", v.name, tr, deferred, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = deferred
						size := chunk
						if size == 0 {
							size = len(wire)
						}
						var events []*ProtocolEvent
						for at := 0; at < len(wire); at += size {
							in := append([]byte(nil), wire[at:min(at+size, len(wire))]...)
							result := s.Feed(1, time.Unix(100, 0), in)
							clear(in)
							require.Equal(t, min(size, len(wire)-at), result.Consumed)
							if len(result.Events) == 0 {
								rocTypedError(t, "NeedMore", result.Err)
								require.True(t, result.NeedMore)
							} else {
								rocTypedError(t, "UnsupportedFeature", result.Err)
								require.Equal(t, v.message, result.Err.Message)
								require.False(t, result.NeedMore)
							}
							events = append(events, result.Events...)
						}
						require.Len(t, events, 1)
						e := events[0]
						require.NotZero(t, e.ID)
						require.Equal(t, "dlms-wrapper", e.Protocol)
						require.Equal(t, "dlms-wrapper-v1-gbt", e.Profile)
						require.Equal(t, "context-required", e.Status)
						require.Equal(t, "context-required", e.Completeness)
						require.Equal(t, wire, e.Raw)
						require.Equal(t, len(wire), e.Length)
						require.Equal(t, time.Unix(100, 0), e.Timestamp)
						require.Zero(t, e.Offset)
						require.Equal(t, 1, e.Direction)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Equal(t, "192.0.2.2:4059", e.Source)
						require.Equal(t, "192.0.2.1:40000", e.Destination)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						require.Nil(t, e.Fields)
						require.Nil(t, e.Session)
						require.Nil(t, e.Structured)
						require.Equal(t, "UnsupportedFeature: "+v.message, e.Error)
						snMultiOwnedDiagnosis(t, e, "UnsupportedFeature", v.message)
						require.Empty(t, s.Close("scope"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
