package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQUICUnparsedTailCannotInheritPacketAuthentication(t *testing.T) {
	initial := rfc9001ClientInitial()
	for _, tail := range [][]byte{{0, 0, 1}, {0x80, 1, 2}, {0x40}} {
		raw := sessionDatagramPCAP(t, []sessionStep{{0, append(bytes.Clone(initial), tail...)}, {0, initial}}, 14443)
		es, stats := discoveryReplay(t, raw, 1, false, false)
		require.Len(t, es, 3)
		good, refused, later := es[0], es[1], es[2]
		require.Equal(t, "authenticated-wire-and-observed-CID", good.Admission)
		require.Equal(t, true, good.Session["Authentication Verified"])
		require.Equal(t, "unparsed-datagram-remainder", refused.Admission)
		require.Equal(t, tail, refused.Raw)
		require.Contains(t, refused.Summary, "offset=1200")
		require.Contains(t, refused.Summary, "authentication not verified")
		require.Nil(t, refused.Session)
		require.Equal(t, "malformed", refused.Status)
		require.Equal(t, good.FlowID, refused.FlowID, "association with an earlier packet is not byte authentication")
		require.Equal(t, good.FlowID, later.FlowID)
		require.Equal(t, true, later.Session["Duplicate"], "a malformed tail must not erase accepted packet state")
		require.Equal(t, "authenticated-wire-and-observed-CID", later.Admission)
		require.Empty(t, refused.AuthenticatedQUICApplicationProtocol())
		require.Zero(t, stats.BufferedBytes)
	}
}

func TestQUICFailedCandidateAdmissionNeverClaimsAuthentication(t *testing.T) {
	bad := bytes.Clone(rfc9001ClientInitial())
	bad[len(bad)-1] ^= 1
	es, stats := discoveryReplay(t, sessionDatagramPCAP(t, []sessionStep{{0, bad}}, 14443), 1, false, false)
	require.Len(t, es, 1)
	require.Equal(t, "unverified-QUIC-candidate", es[0].Admission)
	require.Zero(t, es[0].FlowID)
	require.Equal(t, false, es[0].Session["Authentication Verified"])
	require.Equal(t, string(ErrAuthenticationFailed), es[0].ExpertCode)
	require.Zero(t, stats.BufferedBytes)
}
