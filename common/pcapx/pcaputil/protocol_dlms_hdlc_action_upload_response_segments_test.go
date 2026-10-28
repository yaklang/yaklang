package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func hdlcActionResponseControls(t *testing.T) []hdlcOpCase {
	return hdlcActionResponseCases(t, "controls.json", 8, "61bb64065b60e2a69d2a600328ae2935d8211edf3d359005f246e6972836291e")
}
func TestDLMSHDLCActionUploadResponseSegmentsNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionResponseControls(t), 41000, 4059)
}
func TestDLMSHDLCActionUploadResponseSegmentsOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcActionResponseControls(t)[:3], 45000, 45001)
}
func TestDLMSHDLCActionUploadResponseSegmentsMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcActionResponseControls(t), "dlms-hdlc-action-upload-response-segments", "DLMS_HDLC_ACTION_UPLOAD_RESPONSE_SEGMENTS_TEST_ROOT")
}
func TestDLMSHDLCActionUploadResponseSegmentsCarrier(t *testing.T) {
	for _, c := range hdlcActionResponseControls(t) {
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
				var flow uint64
				for _, w := range c.Steps {
					raw := wrapperWire(t, w.Wire)
					r := s.Feed(w.Direction, time.Unix(w.At, 0), raw)
					require.Equal(t, len(raw), r.Consumed)
					require.False(t, r.NeedMore)
					if tr == "tcp" && w.TCPOverride != nil {
						require.Empty(t, r.Events)
					} else {
						require.Len(t, r.Events, 1)
					}
					for _, e := range r.Events {
						if tr == "tcp" {
							if flow == 0 {
								require.NotZero(t, e.FlowID)
								flow = e.FlowID
							}
							require.Equal(t, flow, e.FlowID)
						}
						if e.Error == "" {
							f, err := e.GetFields()
							require.NoError(t, err)
							if seg, ok := f["HDLC Segmentation"].(map[string]any); ok {
								require.IsType(t, int(0), seg["observed_frames"])
								require.IsType(t, false, seg["more"])
								require.IsType(t, false, seg["complete"])
								require.IsType(t, "", seg["information_hex"])
								if _, ok := seg["assembled_information_hex"]; ok {
									require.IsType(t, "", seg["assembled_information_hex"])
								}
							}
						}
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
				if tr == "tcp" && c.TCPCloseOverride != nil {
					require.Len(t, closed, len(c.TCPCloseOverride.Events))
				} else {
					require.Len(t, closed, len(c.Close.Events))
				}
				for i, e := range closed {
					w := c.Close.Events[i]
					if tr == "tcp" {
						require.Equal(t, flow, e.FlowID)
					}
					require.Equal(t, CaptureDomain{}, e.Domain)
					if tr == "tcp" {
						if flow == 0 {
							require.NotZero(t, e.FlowID)
							flow = e.FlowID
						}
						require.Equal(t, flow, e.FlowID)
					}
					if e.Error == "" {
						f, err := e.GetFields()
						require.NoError(t, err)
						if seg, ok := f["HDLC Segmentation"].(map[string]any); ok {
							require.IsType(t, int(0), seg["observed_frames"])
							require.IsType(t, false, seg["more"])
							require.IsType(t, false, seg["complete"])
							require.IsType(t, "", seg["information_hex"])
							if _, ok := seg["assembled_information_hex"]; ok {
								require.IsType(t, "", seg["assembled_information_hex"])
							}
						}
					}
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
