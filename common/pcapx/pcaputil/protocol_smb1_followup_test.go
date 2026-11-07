package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// MS-CIFS 2.2.4.47.1: a secondary may restate a smaller total, completing the
// transaction early. The primary total is not authoritative when reduced.
func TestSMB1Transaction2SecondaryMayReduceTotal(t *testing.T) {
	primaryWords := smb1Words(uint16(12), uint16(0), uint16(16), uint16(64), byte(0), byte(0), uint16(0), uint32(0), uint16(0), uint16(6), uint16(68), uint16(0), uint16(0), byte(1), byte(0), uint16(5))
	primaryData := []byte{0, 0, 0, 0x07, 0x01, 0, 0, 0, 0}
	primary := smb1Command(0x32, 0x08, 0x8000, 2048, 1, 2048, 7, primaryWords, primaryData)
	secondaryWords := smb1Words(uint16(10), uint16(0), uint16(4), uint16(56), uint16(6), uint16(0), uint16(0), uint16(0), uint16(0xffff))
	secondaryData := []byte{0, 0, 0, 0x61, 0x62, 0x63, 0x64}
	secondary := smb1Command(0x33, 0x08, 0x8000, 2048, 1, 2048, 7, secondaryWords, secondaryData)

	session := smb1Session(t)
	first := smb1FeedOK(t, session, 0, primary)
	require.Contains(t, first.Session["Body"].(map[string]any)["Transaction Status"], "incomplete")
	done := smb1FeedOK(t, session, 0, secondary)
	require.Equal(t, "complete", done.Session["Reassembly Status"])
	require.Len(t, done.Session["Reassembled Parameters"].([]byte), 10)
}

// A long capture accumulates completed identity owners. At the collection
// budget, a completed/ambiguous owner is evicted instead of stopping the flow.
func TestSMB1IdentityCollectionEvictsCompletedOwners(t *testing.T) {
	budget := DefaultParserBudget()
	budget.MaxCollectionElements = 16
	session, err := NewProtocolSession(budget)
	require.NoError(t, err)
	defer session.Close("test")
	smb1FeedComplete(t, session, 0, smb1ExtendedOffer())

	var observed string
	for mid := uint16(1); mid <= 24; mid++ {
		request := smb1FeedComplete(t, session, 0, smb1FileWire(0x75, false, mid, 2048, 0))
		if status, ok := request.Session["Identity Collection Status"].(string); ok {
			observed = status
		}
		response := smb1FeedComplete(t, session, 1, smb1FileWire(0x75, true, mid, 2048, mid))
		if status, ok := response.Session["Identity Collection Status"].(string); ok {
			observed = status
		}
	}
	require.Contains(t, observed, "evicted")
}

func TestSMB1BehaviorProjection(t *testing.T) {
	session := smb1SetupSession(t)
	offer := smb1FeedComplete(t, session, 0, smb1ExtendedOffer())
	require.Equal(t, "negotiate", offer.Session["Operation"])

	setup := smb1FeedComplete(t, session, 0, smb1SetupWire(false, 2, 0, 0))
	require.Equal(t, "session.setup", setup.Session["Operation"])
	require.Contains(t, setup.Session["Observed Behaviors"], "authentication")

	tree := smb1FeedComplete(t, session, 0, smb1FileWire(0x75, false, 3, 2048, 0))
	require.Equal(t, "share.connect", tree.Session["Operation"])
	require.Contains(t, tree.Session["Observed Behaviors"], "share-connect")
	require.Equal(t, `\\server\共享`, tree.Session["Resource"])
}
