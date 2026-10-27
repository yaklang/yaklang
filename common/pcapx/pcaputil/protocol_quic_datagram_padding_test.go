package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQUICUnprotectedDatagramPaddingIsOwnedAndUnauthenticated(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		wire := append(bytes.Clone(rfc9001ClientInitial()), make([]byte, 680)...)
		events, stats := discoveryReplay(t, sessionDatagramPCAP(t, []sessionStep{{0, wire}}, 14443), 1, deferred, false)
		require.Len(t, events, 2)
		padding := events[1]
		require.Empty(t, padding.Error, "a known complete QUIC packet may be followed by unprotected UDP padding")
		require.Equal(t, "observed-datagram-padding", padding.Admission)
		require.Equal(t, "datagram-padding", padding.Completeness)
		require.Equal(t, events[0].FlowID, padding.FlowID)
		require.Equal(t, 680, padding.Length)
		require.Equal(t, bytes.Repeat([]byte{0}, 680), padding.Raw)
		require.Equal(t, events[0].SourceBytes.PacketRefs, padding.SourceBytes.PacketRefs)
		require.Equal(t, false, padding.Session["Authentication Verified"])
		require.Equal(t, 1200, padding.Session["Datagram Offset"])
		require.Equal(t, uint64(680), padding.Session["Datagram Padding Length"])
		require.Nil(t, padding.Session["Frames"], "padding is not an authenticated QUIC PADDING frame")
		require.Empty(t, padding.AuthenticatedQUICApplicationProtocol())
		values := padding.DisplayFieldValues().FieldMap()
		require.Equal(t, []any{uint64(680)}, values["quic.datagram.padding_length"])
		require.Equal(t, []any{uint64(1200)}, values["quic.datagram.padding_offset"])
		require.Equal(t, []any{false}, values["quic.packet.authenticated"])
		require.Empty(t, values["quic.packet_number"])
		fields, err := padding.GetFields()
		require.NoError(t, err)
		require.Equal(t, bytes.Repeat([]byte{0}, 680), fields["Datagram Padding"])
		fields["Datagram Padding"].([]byte)[0] = 1
		padding.Raw[0] = 2
		again, err := padding.GetFields()
		require.NoError(t, err)
		require.Equal(t, bytes.Repeat([]byte{0}, 680), again["Datagram Padding"])
		require.Zero(t, stats.BufferedBytes)
	}
}

func TestQUICDatagramPaddingCannotGrantIdentityOrHideBadTails(t *testing.T) {
	initial := rfc9001ClientInitial()
	bad := bytes.Clone(initial)
	bad[len(bad)-1] ^= 1
	for _, wire := range [][]byte{make([]byte, 32), append(bad, make([]byte, 20)...), append(bytes.Clone(initial), 0, 0, 1)} {
		events, _ := discoveryReplay(t, sessionDatagramPCAP(t, []sessionStep{{0, wire}}, 14443), 1, false, false)
		for _, event := range events {
			require.NotEqual(t, "observed-datagram-padding", event.Admission)
			if !bytes.Equal(event.Raw, initial) {
				require.Empty(t, event.AuthenticatedQUICApplicationProtocol())
			}
		}
	}
}
