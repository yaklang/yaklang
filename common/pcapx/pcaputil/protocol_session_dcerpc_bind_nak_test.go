package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDCERPCBindNakObservedAlignmentAndOwnership(t *testing.T) {
	body := []byte{4, 0, 1, 5, 0, 0, 0, 0}
	for _, padding := range [][]byte{{0, 0, 0}, {0xff, 0x80, 0xa5}} {
		copy(body[5:], padding)
		wire := dcerpcPDU(13, 3, 2, body)
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{0, 1} {
				events, _ := sessionTestFlow(t, "dcerpc", []sessionStep{
					{0, dcerpcBind(dcerpcEPM, 0, 1)}, {1, dcerpcBindAck(1)},
					{0, dcerpcRequest(2, 0, 2, nil)}, {1, wire},
					{1, dcerpcResponse(2, 0, nil)},
				}, chunk, deferred)
				require.GreaterOrEqual(t, len(events), 5)
				nak := events[3]
				wantStatus := "decoded"
				if deferred {
					wantStatus = "deferred"
				}
				require.Equal(t, wantStatus, nak.Status, nak.Error)
				require.Equal(t, uint16(4), nak.Session["Reject Reason"])
				require.Equal(t, 1, nak.Session["Version Count"])
				require.Equal(t, []string{"5.0"}, nak.Session["Protocol Versions"])
				require.Equal(t, padding, nak.Session["Trailing Padding"])
				require.Equal(t, []uint64{21, 24}, nak.Session["Wire Field Ranges"].(map[string]any)["Trailing Padding"])
				require.Zero(t, nak.ResponseTo, "a Bind NAK cannot consume an ordinary Request owner")
				require.Equal(t, "request-type-mismatch", nak.Session["Association Status"])
				fields, err := nak.GetFields()
				require.NoError(t, err)
				parsed := fields["PDU"].(map[string]any)["BindNak"].(map[string]any)
				require.Equal(t, padding, parsed["Trailing Padding"])
				require.Equal(t, wire, nak.Raw)
				require.Equal(t, events[2].ID, events[4].ResponseTo)
			}
		}
	}
	for _, extra := range []int{0, 3} {
		d := newReviewDCERPC()
		_, err := d.consumeFrom(0, dcerpcBind(dcerpcEPM, 0, 2), 4096)
		require.NoError(t, err)
		info, err := d.consumeFrom(1, dcerpcPDU(13, 3, 2, body[:5+extra]), 4096)
		require.NoError(t, err)
		require.Equal(t, true, info["Context Rejected"])
		require.Empty(t, d.pending)
		require.Empty(t, d.proposals)
		require.Empty(t, d.ctx)
	}
}

func TestDCERPCBindNakInvalidTailCannotConsumeProposal(t *testing.T) {
	for _, body := range [][]byte{
		{4, 0}, {4, 0, 1}, {4, 0, 1, 5},
		{4, 0, 1, 5, 0, 0}, {4, 0, 1, 5, 0, 0, 0},
		{4, 0, 1, 5, 0, 0, 0, 0, 0}, {4, 0, 21},
	} {
		d := newReviewDCERPC()
		_, err := d.consumeFrom(0, dcerpcBind(dcerpcEPM, 0, 2), 4096)
		require.NoError(t, err)
		_, err = d.consumeFrom(1, dcerpcPDU(13, 3, 2, body), 4096)
		require.Error(t, err)
		require.Equal(t, "Bind", d.pending[2], "bad bytes cannot retire the pending negotiation")
		info, err := d.consumeFrom(1, dcerpcBindAck(2), 4096)
		require.NoError(t, err)
		require.Equal(t, "Bind", info["Matched Request"])
		require.Equal(t, "EPM", d.ctx[0])
	}
	d := newReviewDCERPC()
	wire := dcerpcPDU(13, 3, 2, []byte{4, 0, 1, 5, 0, 0xff, 0x80, 0xa5})
	info, err := d.consumeFrom(1, wire, 4096)
	// A byte view owns its storage even when the capture reader reuses a buffer.
	require.NoError(t, err)
	saved := bytes.Clone(info["Trailing Padding"].([]byte))
	wire[len(wire)-1] = 0
	require.Equal(t, saved, info["Trailing Padding"])
}
