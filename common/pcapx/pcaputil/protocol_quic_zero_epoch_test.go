package pcaputil

import (
	"bytes"
	"container/list"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQUICZeroCIDNewInitialAndServerResponse(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer a.closeUDPSessions()
	base := &ProtocolEvent{Source: "client", Destination: "server", Timestamp: time.Unix(1, 0)}
	var flows []*binFlow
	for epoch := byte(1); epoch <= 3; epoch++ {
		seed := bytes.Repeat([]byte{epoch}, 8)
		wire := retryTestProtectedInitial(t, seed, seed, nil, nil, 0, 0)
		base.Timestamp = base.Timestamp.Add(time.Second)
		events, ok := a.decodeQUICDatagram(base, wire)
		require.True(t, ok)
		require.Len(t, events, 1)
		require.Equal(t, true, events[0].Session["Authentication Verified"])
		var flow *binFlow
		for _, el := range a.udpSessions.entries {
			f := el.Value.(*binUDPEntry).flow
			if f.id == events[0].FlowID {
				flow = f
			}
		}
		require.NotNil(t, flow)
		for _, old := range flows {
			require.NotEqual(t, old.id, flow.id, "new Initial owns a distinct captured epoch")
		}
		flows = append(flows, flow)
		response := *base
		response.Source, response.Destination = base.Destination, base.Source
		wire = retryTestProtectedInitial(t, seed, nil, nil, nil, 0, 1)
		events, ok = a.decodeQUICDatagram(&response, wire)
		require.True(t, ok)
		require.Len(t, events, 1)
		require.Equal(t, flow.id, events[0].FlowID, "closed zero-CID tuple cannot steal the new server Initial")
		require.Equal(t, true, events[0].Session["Authentication Verified"])
		require.Equal(t, [2]bool{true, true}, flow.quic.zeroCID)
		require.NoError(t, flow.quic.applyFrames(1, quicSpaceApplication, []map[string]any{{"Frame Type": "CONNECTION_CLOSE_APP"}}, map[string]any{}, 64))
		require.True(t, flow.quic.closed)
	}
	// A late old server Initial remains inspectable in the old epoch, even
	// after two newer epochs. Public Initial protection is not peer identity.
	late := *base
	late.Source, late.Destination = base.Destination, base.Source
	events, ok := a.decodeQUICDatagram(&late, retryTestProtectedInitial(t, bytes.Repeat([]byte{1}, 8), nil, nil, nil, 1, 1))
	require.True(t, ok)
	require.Equal(t, flows[0].id, events[0].FlowID)
	require.Equal(t, true, events[0].Session["Authentication Verified"])
	require.NotEqual(t, true, events[0].Session["Peer Identity Verified"])
}

func TestQUICZeroCIDClosedEpochRequiresUniquePacketProtection(t *testing.T) {
	for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
		t.Run(algorithm, func(t *testing.T) {
			c := NewDefaultConfig()
			require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
			require.NoError(t, c.prepareBinParser())
			a := c.binParser
			defer a.closeUDPSessions()
			store := &binUDPStore{entries: map[binUDPKey]*list.Element{}}
			a.udpSessions = store
			var keys []*quicTrafficKeys
			var flows []*binFlow
			for epoch := byte(1); epoch <= 3; epoch++ {
				n := 32
				if algorithm == quicAEADAES256GCM {
					n = 48
				}
				k, err := quicDeriveTraffic(bytes.Repeat([]byte{epoch}, n), algorithm)
				require.NoError(t, err)
				next, err := quicNextPhase(k)
				require.NoError(t, err)
				q := &binQUIC{native: true, byteLimit: 4096, zeroCID: [2]bool{true, true}, keys: &quicKeyring{}, closed: epoch < 3}
				for d := 0; d < 2; d++ {
					q.keys.app[d] = [2]*quicTrafficKeys{k, next}
				}
				f := &binFlow{a: a, id: uint64(epoch), protocol: "quic", quic: q, endpoints: [2]string{"client", "server"}}
				key := binUDPKey{fmt.Sprint(epoch), "server", CaptureDomain{}}
				store.entries[key] = store.lru.PushBack(&binUDPEntry{key, f, time.Unix(1, 0)})
				keys, flows = append(keys, k), append(flows, f)
			}
			base := &ProtocolEvent{Source: "client", Destination: "server", Timestamp: time.Unix(2, 0), FlowID: 900, TransactionID: 901, ResponseTo: 902}
			for _, epoch := range []int{2, 0, 1, 2, 0} {
				for d := 0; d < 2; d++ {
					base.Source, base.Destination = flows[epoch].endpoints[d], flows[epoch].endpoints[1-d]
					wire := epochQUICPacket(t, keys[epoch], nil, uint64(20+epoch), 0, 2)
					events, ok := a.decodeQUICDatagram(base, wire)
					require.True(t, ok)
					require.Len(t, events, 1)
					require.Equal(t, flows[epoch].id, events[0].FlowID)
					require.Equal(t, true, events[0].Session["Authentication Verified"])
					require.Zero(t, events[0].TransactionID)
				}
			}
			for _, el := range store.entries {
				entry := el.Value.(*binUDPEntry)
				if entry.flow.quic.closed {
					require.Equal(t, time.Unix(1, 0), entry.touched, "late closed packets do not renew a key lease")
				}
			}
			before := flows[2].quic.spaces
			wire := epochQUICPacket(t, keys[2], nil, 30, 0, 2)
			wire[len(wire)-1] ^= 1
			events, ok := a.decodeQUICDatagram(base, wire)
			require.True(t, ok)
			require.Len(t, events, 1)
			require.Zero(t, events[0].FlowID, "tuple alone cannot assign a damaged packet to an epoch")
			require.Zero(t, events[0].TransactionID)
			require.Zero(t, events[0].ResponseTo)
			require.Nil(t, events[0].Session)
			require.Equal(t, before, flows[2].quic.spaces, "failed ownership probe never changes PN history")
			cid := bytes.Repeat([]byte{0xa5}, 20)
			flows[2].quic.zeroCID = [2]bool{}
			flows[2].quic.cids = map[string]int{string(cid): 1}
			base.Source, base.Destination = "client", "server"
			events, ok = a.decodeQUICDatagram(base, epochQUICPacket(t, keys[2], cid, 31, 0, 2))
			require.True(t, ok)
			require.Equal(t, flows[2].id, events[0].FlowID, "closed empty CID does not steal an active explicit 20-byte CID")
			require.Equal(t, cid, events[0].Session["DCID"])
			// Duplicate capture contexts with identical keys cannot resolve an
			// owner by map iteration, even though packet protection succeeds.
			flows[1].quic.keys.app = flows[0].quic.keys.app
			events, ok = a.decodeQUICDatagram(base, epochQUICPacket(t, keys[0], nil, 40, 0, 2))
			require.True(t, ok)
			require.Zero(t, events[0].FlowID)
			require.Equal(t, "unresolved-zero-CID-generation", events[0].Admission)
			base.Domain.Interface = 1
			_, ok = a.decodeQUICDatagram(base, wire)
			require.False(t, ok, "neither closed nor active epochs cross a capture domain")
		})
	}
}

func FuzzQUICReusedTupleOwnershipProbe(f *testing.F) {
	f.Add(uint64(769), uint8(1), uint16(31), uint8(1))
	f.Add(uint64(4), uint8(0), uint16(0), uint8(255))
	f.Fuzz(func(t *testing.T, packetNumber uint64, keyPhase uint8, offset uint16, flip uint8) {
		k, err := quicDeriveTraffic(bytes.Repeat([]byte{7}, 32), quicAEADAES128GCM)
		require.NoError(t, err)
		next, err := quicNextPhase(k)
		require.NoError(t, err)
		phase, pn := int(keyPhase&1), packetNumber%(1<<30)+1
		q := &binQUIC{native: true, closed: true, zeroCID: [2]bool{true, true}, keys: &quicKeyring{}}
		q.keys.app[0] = [2]*quicTrafficKeys{k, next}
		q.spaces[0][quicSpaceApplication] = quicPNSpace{largest: pn + 8, init: true, seen: []uint64{pn + 8}}
		wire := epochQUICPacket(t, q.keys.app[0][phase], nil, pn, phase, 2)
		before := q.spaces
		require.True(t, q.authenticatesExistingWire(0, wire))
		require.Equal(t, before, q.spaces)
		require.Zero(t, q.keyPhase[0], "ownership probe must not commit a newer key phase")
		if flip == 0 {
			flip = 1
		}
		wire[int(offset)%len(wire)] ^= flip
		require.False(t, q.authenticatesExistingWire(0, wire), "changed wire must not borrow the old epoch")
		require.Equal(t, before, q.spaces)
	})
}
