package pcaputil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func identityObservation(id uint64, refs ...PacketReference) *ProtocolEvent {
	return &ProtocolEvent{ID: id, Protocol: "bgp", Status: "decoded", Raw: []byte{4}, SourceBytes: ByteSource{PacketRefs: refs}}
}

func TestContributorIdentitySurvivesMixedFrameAndOversizedDetailEviction(t *testing.T) {
	ref := PacketReference{Number: 294}
	for _, oversized := range []bool{false, true} {
		t.Run(map[bool]string{false: "mixed-frame", true: "oversized-callback"}[oversized], func(t *testing.T) {
			v, err := NewProtocolInspector(1, 4096)
			require.NoError(t, err)
			first := identityObservation(299, ref)
			if oversized {
				first.Raw = make([]byte, 8192)
			}
			v.OnEvent(first)
			v.OnEvent(identityObservation(300, ref))
			ids, complete := v.PacketEventWindow(ref, false)
			require.Equal(t, []uint64{300}, ids)
			require.False(t, complete, "first query must see missing same-frame contributor fields")
			w := v.PacketContributorWindow(ref, false, 4096)
			require.True(t, w.Complete, "identity coverage survives loss of payload")
			require.Equal(t, []ProtocolContributorIdentity{{ID: 299, Arrival: 1}, {ID: 300, Arrival: 2}}, w.Identities)
			require.Equal(t, uint64(2), v.Revision())
			require.Equal(t, uint64(1), v.Evicted())
			_, err = v.Details(299)
			require.Error(t, err)
			first.ID, first.SourceBytes.PacketRefs[0].Number = 999, 999
			w.Identities[0].ID = 999
			require.Equal(t, uint64(299), v.PacketContributorWindow(ref, false, 4096).Identities[0].ID)
		})
	}
}

func TestContributorIdentityLateTCPAndExactPhysicalDomains(t *testing.T) {
	v, err := NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	a := PacketReference{Number: 1, Domain: CaptureDomain{Section: 1, Interface: 2, Encapsulation: "/gre:a"}}
	b := a
	b.Domain.Encapsulation = "/gre:b"
	tail := a
	tail.Number = 2
	before := v.PacketQueryRevision(a, nil, false)
	v.OnEvent(identityObservation(91, a, a, b, tail))
	require.NotEqual(t, before, v.PacketQueryRevision(a, nil, false))
	for _, ref := range []PacketReference{a, b, tail} {
		w := v.PacketContributorWindow(ref, false, 4096)
		require.True(t, w.Complete)
		require.Equal(t, []ProtocolContributorIdentity{{ID: 91, Arrival: 1}}, w.Identities)
	}
	require.Len(t, v.PacketContributorWindow(a, true, 4096).Identities, 1, "physical dedup uses arrival, not tunnel-reference count")
	foreign := a
	foreign.Domain.Section++
	require.Empty(t, v.PacketContributorWindow(foreign, true, 4096).Identities)
	foreign = a
	foreign.Domain.Interface++
	require.Empty(t, v.PacketContributorWindow(foreign, true, 4096).Identities)
	current := a
	current.Number = 10
	v.OnEvent(identityObservation(92, current))
	for _, ref := range []PacketReference{a, b, tail} {
		require.Equal(t, uint64(91), v.PacketContributorWindow(ref, false, 4096).Identities[0].ID)
		_, complete := v.PacketEventWindow(ref, false)
		require.False(t, complete)
	}
	require.Equal(t, []ProtocolContributorIdentity{{ID: 92, Arrival: 2}}, v.PacketContributorWindow(current, false, 4096).Identities)
}

func TestContributorIdentityDuplicateIDsNeverBorrowAnotherArrival(t *testing.T) {
	a, b := PacketReference{Number: 1}, PacketReference{Number: 2}
	for _, same := range []bool{false, true} {
		v, err := NewProtocolInspector(2, 1<<20)
		require.NoError(t, err)
		other := b
		if same {
			other = a
		}
		v.OnEvent(identityObservation(7, a))
		v.OnEvent(identityObservation(7, other))
		detail, err := v.Details(7)
		require.NoError(t, err)
		require.Equal(t, uint64(1), detail.HistoryArrival())
		_, complete := v.PacketEventWindow(other, false)
		require.False(t, complete)
		window := v.PacketContributorWindow(other, false, 4096)
		require.True(t, window.Complete)
		require.Equal(t, uint64(2), window.Identities[len(window.Identities)-1].Arrival)
		v.OnEvent(identityObservation(8, b))
		_, complete = v.PacketEventWindow(other, false)
		require.Equal(t, !same, complete, "only the other-domain second arrival now owns Details7")
	}
}

func TestContributorIdentityIndexesReferencesBeyondPDUViewWindow(t *testing.T) {
	v, err := NewProtocolInspector(140, 1<<20)
	require.NoError(t, err)
	refs := make([]PacketReference, 129)
	for i := range refs {
		refs[i].Number = uint64(i + 1)
	}
	v.OnEvent(identityObservation(1, refs...))
	require.Equal(t, uint64(1), v.PacketContributorWindow(refs[128], false, 4096).Identities[0].ID)
	for id := uint64(2); id <= 129; id++ {
		v.OnEvent(identityObservation(id, refs[0]))
	}
	ids, complete := v.PacketEventWindow(refs[0], false)
	require.Len(t, ids, 128)
	require.False(t, complete)
	w := v.PacketContributorWindow(refs[0], false, 4096)
	require.Len(t, w.Identities, 129)
	require.True(t, w.Complete)
	require.Equal(t, uint64(129), w.Identities[128].ID)
	require.False(t, v.PacketContributorWindow(refs[0], false, 128).Complete)
}

func TestContributorIdentityBudgetLossIsBoundedAndConservative(t *testing.T) {
	v, err := NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	v.identities = newProtocolIdentityIndex(1024)
	for _, id := range []uint64{200, 1, 201} {
		v.OnEvent(identityObservation(id, PacketReference{Number: id}))
		require.LessOrEqual(t, v.identities.bytes, v.identities.maxBytes)
		require.LessOrEqual(t, v.identities.count, len(v.identities.rows))
	}
	x := v.identities
	require.NotZero(t, x.losses)
	require.Equal(t, uint64(200), x.lostMaxNumber)
	require.Zero(t, v.Evicted(), "identity loss is independent of retained details")
	old := PacketReference{Number: 1, Domain: CaptureDomain{Interface: 99}}
	require.False(t, v.PacketContributorWindow(old, false, 4096).Complete, "old unrelated domain is conservatively partial")
	newer := PacketReference{Number: 201}
	require.True(t, v.PacketContributorWindow(newer, false, 4096).Complete)
	_, complete := v.PacketEventWindow(newer, false)
	require.True(t, complete)
	exact, physical := 0, 0
	for _, rows := range x.byExact {
		exact += len(rows)
	}
	for _, rows := range x.byPhysical {
		physical += len(rows)
	}
	require.Equal(t, x.count, exact)
	require.Equal(t, x.count, physical)
	before := v.PacketQueryRevision(newer, nil, false)
	huge := PacketReference{Number: 300, Domain: CaptureDomain{Encapsulation: strings.Repeat("a", 2048)}}
	v.OnEvent(identityObservation(9, huge))
	require.NotEqual(t, before, v.PacketQueryRevision(newer, nil, false))
	require.False(t, v.PacketContributorWindow(huge, false, 4096).Complete)
	require.Empty(t, v.PacketContributorWindow(huge, false, 4096).Identities)
	require.LessOrEqual(t, x.bytes, x.maxBytes)
}

func TestContributorIdentityRejectsPlaceholdersAndReportsReferenceRefusals(t *testing.T) {
	v, err := NewProtocolInspector(4, 1<<20)
	require.NoError(t, err)
	v.OnEvent(identityObservation(7, PacketReference{}))
	v.OnEvent(identityObservation(0, PacketReference{Number: 1}))
	placeholder := identityObservation(99, PacketReference{Number: 2})
	placeholder.Completeness = "evicted"
	v.OnEvent(placeholder)
	for _, ref := range []PacketReference{{}, {Number: 1}, {Number: 2}} {
		w := v.PacketContributorWindow(ref, false, 4096)
		require.Empty(t, w.Identities)
		require.False(t, w.Complete)
	}
	limited := identityObservation(8, PacketReference{Number: 3})
	limited.ExpertCode = "PacketReferencesLimited"
	v.OnEvent(limited)
	w := v.PacketContributorWindow(PacketReference{Number: 3}, false, 4096)
	require.False(t, w.Complete)
	require.Equal(t, uint64(8), w.Identities[0].ID, "known IDs survive an unbounded omitted-reference scope")
}

func TestContributorIdentityInputAndOutputWindowsHaveSeparateBounds(t *testing.T) {
	v, err := NewProtocolInspector(5000, 16<<20)
	require.NoError(t, err)
	refs := make([]PacketReference, maxPacketIdentityWindow+1)
	for i := range refs {
		refs[i].Number = uint64(i + 1)
	}
	v.OnEvent(identityObservation(1, refs...))
	w := v.PacketContributorWindow(refs[len(refs)-1], false, 4096)
	require.Empty(t, w.Identities)
	require.False(t, w.Complete)
	require.Equal(t, uint64(len(refs)), v.identities.lostMaxNumber)
	// The input-ref cap must not silently certify an empty union; a newer
	// separate packet is still complete when its own source remains retained.
	newer := PacketReference{Number: 5000}
	v.OnEvent(identityObservation(2, newer))
	require.True(t, v.PacketContributorWindow(newer, false, 4096).Complete)
	for id := uint64(3); id < 4100; id++ {
		v.OnEvent(identityObservation(id, newer))
	}
	w = v.PacketContributorWindow(newer, false, 4096)
	require.Len(t, w.Identities, 4096)
	require.False(t, w.Complete)
}

func TestContributorIdentityLongDomainCannotBypassDetailBudget(t *testing.T) {
	domain := CaptureDomain{Encapsulation: strings.Repeat("/observed", 1024)}
	ref := PacketReference{Number: 1, Domain: domain}
	e := identityObservation(1, ref)
	e.Domain, e.SourceBytes.CompletedIn = domain, &ref
	require.GreaterOrEqual(t, e.RetainedBytes(), 3*len(domain.Encapsulation))
	v, err := NewProtocolInspector(2, 4096)
	require.NoError(t, err)
	v.OnEvent(e)
	require.Equal(t, uint64(1), v.Evicted())
	_, err = v.Details(1)
	require.Error(t, err, "domain storage cannot hide behind a small raw message")
	w := v.PacketContributorWindow(ref, false, 4096)
	require.True(t, w.Complete)
	require.Equal(t, uint64(1), w.Identities[0].ID, "a small source identity can outlive oversized detail")
}
