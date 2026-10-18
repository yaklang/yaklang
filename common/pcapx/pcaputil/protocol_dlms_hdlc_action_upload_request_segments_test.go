package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func hdlcActionRequestControls(t *testing.T) []hdlcOpCase {
	return hdlcActionRequestCases(t, "controls.json", 8, "09237292210b586c9d45abb8cd8efa1c996dbed9cfc72bbe16d8daeaa37c5301")
}
func TestDLMSHDLCActionUploadRequestSegmentsNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionRequestControls(t), 41000, 4059)
}
func TestDLMSHDLCActionUploadRequestSegmentsOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcActionRequestControls(t)[:3], 45000, 45001)
}
func TestDLMSHDLCActionUploadRequestSegmentsMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcActionRequestControls(t), "dlms-hdlc-action-upload-request-segments", "DLMS_HDLC_ACTION_UPLOAD_REQUEST_SEGMENTS_TEST_ROOT")
}
func TestDLMSHDLCActionUploadRequestSegmentsCarrier(t *testing.T) {
	for _, c := range append(hdlcActionRequestControls(t), hdlcActionRequestColdCase(t)) {
		for _, tr := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+tr, func(t *testing.T) {
				b := DefaultParserBudget()
				if c.Budget.Collection > 0 {
					b.MaxCollectionElements = c.Budget.Collection
				}
				if c.Budget.Depth > 0 {
					b.MaxRecursionDepth = c.Budget.Depth
				}
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(tr), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				ends := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
				for _, w := range c.Steps {
					wire := wrapperWire(t, w.Wire)
					r := s.Feed(w.Direction, time.Unix(w.At, 0), wire)
					require.Equal(t, len(wire), r.Consumed)
					require.False(t, r.NeedMore)
					if tr == "tcp" && w.TCPOverride != nil {
						require.Empty(t, r.Events)
					} else {
						require.Len(t, r.Events, 1)
					}
					for _, e := range r.Events {
						require.Equal(t, ends[w.Direction], e.Source)
						require.Equal(t, ends[1-w.Direction], e.Destination)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Equal(t, time.Unix(w.At, 0), e.Timestamp)
						require.Zero(t, e.TransactionID)
						require.NotZero(t, e.ID)
						require.Equal(t, "dlms", e.Protocol)
						require.Equal(t, tr, e.Transport)
					}
				}
				closed := s.Close("carrier")
				wantClose := len(c.Close.Events)
				if tr == "tcp" && c.TCPCloseOverride != nil {
					wantClose = 0
				}
				require.Len(t, closed, wantClose)
				for i, e := range closed {
					w := c.Close.Events[i]
					require.Equal(t, CaptureDomain{}, e.Domain)
					require.Equal(t, ends[w.Direction], e.Source)
					require.Equal(t, ends[1-w.Direction], e.Destination)
					at := w.TimeUDP
					if tr == "tcp" {
						at = w.TimeTCP
					}
					require.Equal(t, time.Unix(at, 0), e.Timestamp)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
				}
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
