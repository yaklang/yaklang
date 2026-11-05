package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func h3TunnelHistoryObservations() (*ProtocolEvent, *ProtocolEvent) {
	target, context := h3HistoryObservations()
	delete(context.semanticSession, "HTTP3 Late Associations")
	d := context.Domain
	context.semanticSession["HTTP3 Tunnel Observations"] = []map[string]any{{"Frame PDU ID": target.ID, "Observed In PDU ID": context.ID, "Flow ID": context.FlowID, "Stream ID": uint64(0), "Direction": uint64(1), "Frame Offset": uint64(8), "Frame Type Value": uint64(1), "Error Name": "H3_FRAME_UNEXPECTED", "Error Code": uint64(0x105), "Error Scope": "connection", "Capture Domain": map[string]any{"Section": d.Section, "Interface": d.Interface, "Encapsulation": d.Encapsulation}}}
	return target, context
}

func TestHTTP3TunnelHistoryRejectsWrongOwnerDomainAndAmbiguousTargets(t *testing.T) {
	for _, change := range []string{"flow", "domain", "observation", "not-native", "not-authenticated", "ambiguous-target"} {
		t.Run(change, func(t *testing.T) {
			history, err := NewProtocolInspector(16, 1<<20)
			require.NoError(t, err)
			target, context := h3TunnelHistoryObservations()
			fact := context.semanticSession["HTTP3 Tunnel Observations"].([]map[string]any)[0]
			switch change {
			case "flow":
				fact["Flow ID"] = uint64(999)
			case "domain":
				fact["Capture Domain"].(map[string]any)["Interface"] = 999
			case "observation":
				fact["Observed In PDU ID"] = uint64(999)
			case "not-native":
				context.nativeQUICObservation = false
			case "not-authenticated":
				context.semanticSession["Authentication Verified"] = false
			}
			history.OnEvent(target)
			if change == "ambiguous-target" {
				history.OnEvent(target)
			}
			history.OnEvent(context)
			if change != "ambiguous-target" {
				view, err := history.Details(target.ID)
				require.NoError(t, err)
				require.NotContains(t, view.Session, "HTTP3 Resolved Tunnel Observations")
				require.Empty(t, view.DisplayFieldValues().FieldMap()["http3.tunnel.error_code"])
			}
		})
	}
}

func TestHTTP3TunnelHistoryUsesSharedBudgetAndCannotResurrectEvictedPDU(t *testing.T) {
	for _, mode := range []string{"edge-cap", "byte-cap", "evicted", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			history, err := NewProtocolInspector(8, 1<<20)
			require.NoError(t, err)
			target, context := h3TunnelHistoryObservations()
			fact := context.semanticSession["HTTP3 Tunnel Observations"].([]map[string]any)[0]
			history.OnEvent(target)
			context.Raw = make([]byte, 2<<20) // record context, without storing this oversized DTO
			switch mode {
			case "edge-cap":
				for i := uint64(0); i < 65; i++ {
					fact["Stream ID"] = i * 4
					history.OnEvent(context)
				}
			case "byte-cap":
				history.maxBytes = history.bytes + 1
				history.OnEvent(context)
			case "evicted":
				for id := uint64(100); id < 110; id++ {
					history.OnEvent(&ProtocolEvent{ID: id, Protocol: "tcp"})
				}
				history.OnEvent(context)
				_, err := history.Details(target.ID)
				require.Error(t, err)
				return
			case "conflict":
				history.OnEvent(context)
				fact["Frame Type Value"] = uint64(3)
				history.OnEvent(context)
			}
			view, err := history.Details(target.ID)
			require.NoError(t, err)
			require.Equal(t, false, view.Session["HTTP3 Tunnel View"].(map[string]any)["Complete"])
			require.True(t, view.DisplayFieldValues().Incomplete)
			if mode == "edge-cap" {
				require.Len(t, view.Session["HTTP3 Resolved Tunnel Observations"], 64)
			}
			if mode == "byte-cap" || mode == "conflict" {
				require.Empty(t, view.DisplayFieldValues().FieldMap()["http3.tunnel.error_code"])
			}
			require.LessOrEqual(t, history.bytes, history.maxBytes)
		})
	}
}

func TestHTTP3TunnelConnectionFaultKeepsOtherStreamEvidenceAndFirstFailure(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, _, err := h3BodyTestFeed(q, 0, 0, 0, h3TunnelRequest(), false)
	require.NoError(t, err)
	response := h3ResponseHeaders()
	_, _, err = h3BodyTestFeed(q, 1, 0, 0, response, false)
	require.NoError(t, err)
	_, _, err = h3BodyTestFeed(q, 0, 4, 0, h3BodyRequest("GET", "-"), false)
	require.NoError(t, err)
	bad := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(len(response)), "Stream Data": []byte{1}, "FIN": false}
	other := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(4), "Offset": uint64(0), "Stream Data": response, "FIN": false}
	err = q.applyFrames(1, quicSpaceApplication, []map[string]any{bad, other}, map[string]any{}, 1024)
	require.True(t, isH3ConnectionObservationError(err))
	require.NotEmpty(t, other["HTTP3 Frames"])
	require.Equal(t, "h3", q.alpn)
	require.Equal(t, uint64(0x105), q.h3Fault.snapshot()["Error Code"])
	_, _, err = h3BodyTestFeed(q, 0, 8, 0, append(h3BodyRequest("GET", "-"), 1), true)
	require.True(t, isH3ConnectionObservationError(err))
	require.Equal(t, uint64(0x105), q.h3Fault.snapshot()["Error Code"], "later frame truncation must not overwrite the first connection violation")
}
