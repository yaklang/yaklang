package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQUICCIDIdentityPacketRefusalIsAtomic(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for dir := 0; dir < 2; dir++ {
				for _, future := range []bool{false, true} {
					for _, fault := range []string{"token-change", "CID-new-sequence", "retire-current-dcid", "identical-retransmission"} {
						t.Run(fmt.Sprintf("v%x/%s/dir%d/future%t/%s", version, algorithm, dir, future, fault), func(t *testing.T) {
							n := n09ObservedCIDOwner(t, version, algorithm, [2]bool{}, true)
							q := n.q
							q.byteLimit = 1 << 20
							cid := []byte("offered8")
							plain := n09ObservedNewCID(1, cid)
							if fault != "retire-current-dcid" {
								_, err := q.consume(dir, n09ObservedCIDPacket(t, n.key, n.cid[1-dir], 1, 0, plain), 4096)
								require.NoError(t, err)
							}
							switch fault {
							case "token-change":
								plain = bytes.Clone(plain)
								plain[len(plain)-1] ^= 1
							case "CID-new-sequence":
								plain = n09ObservedNewCID(2, cid)
							case "retire-current-dcid":
								plain = []byte{0x19, 0, 1}
							}
							beforeSequences := [2]map[uint64]string{cloneQUICCIDSequences(q.cidSequences[0]), cloneQUICCIDSequences(q.cidSequences[1])}
							beforeRetired := [2]map[uint64]string{cloneQUICCIDSequences(q.cidRetired[0]), cloneQUICCIDSequences(q.cidRetired[1])}
							beforeRoutes := map[string]int{}
							for k, v := range q.cids {
								beforeRoutes[k] = v
							}
							beforeKeys := *q.keys
							beforeRetireFloor := q.cidRetiredBelow
							beforeTokens := q.cloneCIDState().cidTokens
							key, phase := n.key, 0
							if future {
								key = q.keys.app[dir][1]
								phase = 1
							}
							info, err := q.consume(dir, n09ObservedCIDPacket(t, key, n.cid[1-dir], 2, phase, plain), 4096)
							require.Equal(t, true, info["Authentication Verified"])
							if fault == "identical-retransmission" {
								require.NoError(t, err)
								require.Equal(t, beforeSequences, [2]map[uint64]string{cloneQUICCIDSequences(q.cidSequences[0]), cloneQUICCIDSequences(q.cidSequences[1])})
								return
							}
							require.Error(t, err, "a CID identity conflict or retirement of this packet's DCID must not authorize transport state")
							var typed *ProtocolError
							require.ErrorAs(t, err, &typed)
							require.Equal(t, ErrMalformedMessage, typed.Kind)
							require.Equal(t, beforeSequences, [2]map[uint64]string{cloneQUICCIDSequences(q.cidSequences[0]), cloneQUICCIDSequences(q.cidSequences[1])})
							require.Equal(t, beforeRetired, [2]map[uint64]string{cloneQUICCIDSequences(q.cidRetired[0]), cloneQUICCIDSequences(q.cidRetired[1])})
							require.Equal(t, beforeRoutes, q.cids)
							require.Equal(t, beforeRetireFloor, q.cidRetiredBelow)
							require.Equal(t, beforeKeys, *q.keys)
							require.Empty(t, q.streams)
							require.Equal(t, beforeTokens, q.cidTokens)
							require.Contains(t, info, "Rejected CID Frames")
							require.NotContains(t, info, "Frames")
							require.Contains(t, info["Payload Delivery"], "not delivered")
						})
					}
				}
			}
		}
	}
}

func cidIdentityFrame(sequence, retire uint64, cid string, token byte) map[string]any {
	return map[string]any{"Frame Type": "NEW_CONNECTION_ID", "Sequence": sequence, "Retire Prior To": retire, "CID": []byte(cid), "Reset Token": bytes.Repeat([]byte{token}, 16)}
}

func TestQUICCIDIdentityPrivateOwnershipAndRetiredBounds(t *testing.T) {
	for dir := 0; dir < 2; dir++ {
		t.Run(fmt.Sprint(dir), func(t *testing.T) {
			q := &binQUIC{native: true, cids: map[string]int{}}
			frame := cidIdentityFrame(1, 0, "owned", 0x21)
			require.NoError(t, q.applyNativeNewCID(dir, frame, 2))
			frame["CID"].([]byte)[0] ^= 0xff
			frame["Reset Token"].([]byte)[0] ^= 0xff
			require.Equal(t, "owned", q.cidSequences[dir][1])
			ownedToken := q.cidTokens[dir][1]
			require.Equal(t, bytes.Repeat([]byte{0x21}, 16), ownedToken[:])
			probe := q.cloneCIDState()
			probe.cidTokens[dir][1] = [16]byte{}
			delete(probe.cids, "owned")
			delete(probe.cidSequences[dir], 1)
			require.Contains(t, q.cids, "owned")
			require.Equal(t, "owned", q.cidSequences[dir][1])
			require.NotEqual(t, [16]byte{}, q.cidTokens[dir][1])
			for seq := uint64(2); seq <= 8; seq++ {
				require.NoError(t, q.applyNativeNewCID(dir, cidIdentityFrame(seq, seq, fmt.Sprint(seq), byte(seq)), 2))
				require.Len(t, q.cidSequences[dir], 1)
				require.LessOrEqual(t, len(q.cidRetired[dir]), 2)
				require.LessOrEqual(t, len(q.cidTokens[dir]), len(q.cidSequences[dir])+len(q.cidRetired[dir]))
			}
			require.NotContains(t, q.cidTokens[dir], uint64(1))
			old := cidIdentityFrame(1, 0, "owned", 0x99)
			before := q.cloneCIDState()
			require.NoError(t, q.applyNativeNewCID(dir, old, 2), "evicted identity is unknown, not a proven token conflict")
			require.Equal(t, true, old["Already Retired"])
			require.Equal(t, before.cids, q.cids)
			require.Equal(t, before.cidTokens, q.cidTokens)
			require.NotContains(t, q.cids, "owned")
		})
	}
}

func TestQUICCIDBudgetRefusalDoesNotRetireLiveMappings(t *testing.T) {
	q := &binQUIC{native: true, cids: map[string]int{}}
	for seq, cid := range []string{"a", "b"} {
		require.NoError(t, q.applyNativeNewCID(0, cidIdentityFrame(uint64(seq+1), 0, cid, byte(seq)), 2))
	}
	before := q.cloneCIDState()
	require.ErrorContains(t, q.applyNativeNewCID(0, cidIdentityFrame(3, 1, "c", 3), 2), "CID budget")
	require.Equal(t, before.cids, q.cids)
	require.Equal(t, before.cidSequences, q.cidSequences)
	require.Equal(t, before.cidRetiredBelow, q.cidRetiredBelow)
	require.Equal(t, before.cidTokens, q.cidTokens)
	require.NoError(t, q.applyNativeNewCID(0, cidIdentityFrame(3, 2, "c", 3), 2))
	require.NotContains(t, q.cids, "a")
	require.Contains(t, q.cids, "b")
	require.Contains(t, q.cids, "c")
	require.Contains(t, q.cidRetired[0], uint64(1))
	require.Equal(t, uint64(2), q.cidRetiredBelow[0])
}

func TestQUICCIDIncompleteIdentityCannotChangeState(t *testing.T) {
	for _, name := range []string{"missing-cid", "empty-cid", "long-cid", "missing-token", "short-token", "long-token", "sequence-type", "retire-after-sequence"} {
		t.Run(name, func(t *testing.T) {
			q := &binQUIC{native: true, cids: map[string]int{}}
			frame := cidIdentityFrame(1, 0, "CID", 1)
			switch name {
			case "missing-cid":
				delete(frame, "CID")
			case "empty-cid":
				frame["CID"] = []byte{}
			case "long-cid":
				frame["CID"] = make([]byte, 21)
			case "missing-token":
				delete(frame, "Reset Token")
			case "short-token":
				frame["Reset Token"] = make([]byte, 15)
			case "long-token":
				frame["Reset Token"] = make([]byte, 17)
			case "sequence-type":
				frame["Sequence"] = 1
			case "retire-after-sequence":
				frame["Retire Prior To"] = uint64(2)
			}
			before := *q
			require.ErrorContains(t, q.applyNativeNewCID(0, frame, 8), "complete CID, sequence and reset token")
			require.Equal(t, before, *q)
		})
	}
}

func FuzzQUICCIDIdentityPacketIsolation(f *testing.F) {
	for _, seed := range []struct{ mode, flags byte }{{0, 0}, {1, 31}, {2, 42}, {3, 17}, {4, 33}, {0, 63}, {2, 19}} {
		f.Add(seed.mode, seed.flags, []byte{0, 1, 2})
	}
	f.Fuzz(func(t *testing.T, mode, flags byte, data []byte) {
		version := quicVersion1
		if flags&1 != 0 {
			version = quicVersion2
		}
		dir := int(flags>>1) & 1
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[(flags>>2)%3]
		n := n09ObservedCIDOwner(t, version, algorithm, [2]bool{}, true)
		q := n.q
		q.byteLimit = 1 << 20
		plain := n09ObservedNewCID(1, []byte("offered8"))
		_, err := q.consume(dir, n09ObservedCIDPacket(t, n.key, n.cid[1-dir], 1, 0, plain), 64)
		require.NoError(t, err)
		mode %= 5
		switch mode {
		case 0:
			at := 0
			if len(data) > 0 {
				at = int(data[0]) % 16
			}
			plain[len(plain)-16+at] ^= 1
		case 1:
			plain = n09ObservedNewCID(2, []byte("offered8"))
		case 2:
			plain = []byte{0x19, 0, 1}
		case 4:
			cid := []byte("newCID!!")
			for i, b := range data[:min(len(data), 7)] {
				cid[i+1] ^= b
			}
			plain = n09ObservedNewCID(2, cid)
		}
		before := q.cloneCIDState()
		keys := *q.keys
		key, phase := n.key, 0
		if flags&16 != 0 {
			key, phase = q.keys.app[dir][1], 1
		}
		wire := n09ObservedCIDPacket(t, key, n.cid[1-dir], 2, phase, plain)
		info, err := q.consume(dir, wire, 64)
		require.Equal(t, true, info["Authentication Verified"])
		if mode < 3 {
			require.Error(t, err)
			require.Equal(t, before.cids, q.cids)
			require.Equal(t, before.cidSequences, q.cidSequences)
			require.Equal(t, before.cidTokens, q.cidTokens)
			require.Equal(t, keys, *q.keys)
			require.NotContains(t, info, "Frames")
			require.Contains(t, info, "Rejected CID Frames")
			duplicate, again := q.consume(dir, wire, 64)
			require.Error(t, again)
			require.Equal(t, err.Error(), again.Error())
			require.Equal(t, true, duplicate["Duplicate"])
		} else {
			require.NoError(t, err)
		}
	})
}

func TestQUICSharedCIDIssuerRouting(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for first := 0; first < 2; first++ {
				t.Run(fmt.Sprintf("v%x/%s/first%d", version, algorithm, first), func(t *testing.T) {
					n := n09ObservedCIDOwner(t, version, algorithm, [2]bool{}, true)
					shared := []byte("shared!!")
					n.q.byteLimit = 1 << 20
					for step, owner := range []int{first, 1 - first} {
						plain := n09ObservedNewCID(1, shared)
						plain[len(plain)-1] ^= byte(owner)
						e := n.dispatch(t, owner, uint64(step+3), n09ObservedCIDPacket(t, n.key, n.cid[1-owner], 1, 0, plain))
						require.Equal(t, "decoded", e.Status)
					}
					require.NotEqual(t, n.q.cidTokens[0][1], n.q.cidTokens[1][1], "same CID bytes in the peer namespace do not equate the reset token identities")
					for dir := 0; dir < 2; dir++ {
						packet := n09ObservedCIDPacket(t, n.key, shared, 2, 0, []byte{1})
						h, err := n.q.wireHeader(packet, dir)
						require.NoError(t, err, "the last issuer must not remove the other peer's equal-length CID route")
						require.Equal(t, shared, h.dcid)
						e := n.dispatch(t, dir, uint64(dir+5), packet)
						require.Equal(t, "decoded", e.Status)
						require.Equal(t, dir, e.Direction)
						require.Equal(t, n.f.id, e.FlowID)
						require.Equal(t, true, e.Session["Authentication Verified"])
					}
				})
			}
		}
	}
}
func TestQUICSharedCIDRetirementIsIssuerScoped(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for owner := 0; owner < 2; owner++ {
			for _, floor := range []bool{false, true} {
				t.Run(fmt.Sprintf("v%x/owner%d/floor%t", version, owner, floor), func(t *testing.T) {
					n := n09ObservedCIDOwner(t, version, quicAEADAES128GCM, [2]bool{}, true)
					n.q.byteLimit = 1 << 20
					shared := []byte("shared!!")
					for dir := 0; dir < 2; dir++ {
						plain := n09ObservedNewCID(1, shared)
						plain[len(plain)-1] ^= byte(dir)
						require.Equal(t, "decoded", n.dispatch(t, dir, uint64(dir+3), n09ObservedCIDPacket(t, n.key, n.cid[1-dir], 1, 0, plain)).Status)
					}
					retiringDir := 1 - owner
					plain := []byte{0x19, 1, 1}
					if floor {
						retiringDir = owner
						plain = n09ObservedNewCID(2, []byte("new-cid8"))
						plain[2] = 2
					}
					retired := n.dispatch(t, retiringDir, 5, n09ObservedCIDPacket(t, n.key, n.cid[1-retiringDir], 2, 0, plain))
					require.Equal(t, "decoded", retired.Status)
					require.Equal(t, string(shared), n.q.cidRetired[owner][1])
					require.Equal(t, string(shared), n.q.cidSequences[1-owner][1])
					// The peer issuer's live route and the retired issuer's reordered packet
					// have identical wire CID bytes but independently observed endpoint roles.
					live := n.dispatch(t, owner, 6, n09ObservedCIDPacket(t, n.key, shared, 3, 0, []byte{1}))
					require.Equal(t, "decoded", live.Status)
					require.Equal(t, owner, live.Direction)
					require.Equal(t, true, live.Session["Authentication Verified"])
					late := n.dispatch(t, 1-owner, 7, n09ObservedCIDPacket(t, n.key, shared, 4, 0, []byte{1}))
					require.Equal(t, "decoded", late.Status)
					require.Equal(t, 1-owner, late.Direction)
					require.Equal(t, true, late.Session["Authentication Verified"])
					require.Equal(t, true, late.Session["Retired Destination CID"])
				})
			}
		}
	}
}
func TestQUICSharedCIDIssuerBudgetIsAtomic(t *testing.T) {
	q := &binQUIC{native: true, cids: map[string]int{}}
	require.NoError(t, q.applyNativeNewCID(0, cidIdentityFrame(1, 0, "shared", 0x21), 2))
	require.NoError(t, q.applyNativeNewCID(1, cidIdentityFrame(1, 0, "shared", 0x22), 2))
	before := q.cloneCIDState()
	require.ErrorContains(t, q.applyNativeNewCID(0, cidIdentityFrame(2, 0, "third", 0x23), 2), "CID budget", "both issuer-qualified identities consume the configured collection budget")
	require.Equal(t, before.cids, q.cids)
	require.Equal(t, before.cidSequences, q.cidSequences)
	require.Equal(t, before.cidTokens, q.cidTokens)
	require.Equal(t, before.cidRetiredBelow, q.cidRetiredBelow)
}

func sharedCIDTestOwner(t *testing.T, version uint32, algorithm string, distinct bool) (*n09ObservedCID, [2]*quicTrafficKeys) {
	t.Helper()
	n := n09ObservedCIDOwner(t, version, algorithm, [2]bool{}, true)
	n.q.byteLimit = 1 << 20
	keys := [2]*quicTrafficKeys{n.key, n.key}
	if distinct {
		size := 32
		if algorithm == quicAEADAES256GCM {
			size = 48
		}
		key, err := quicDeriveTrafficVersion(bytes.Repeat([]byte{0x83}, size), algorithm, version)
		require.NoError(t, err)
		next, err := quicNextPhase(key)
		require.NoError(t, err)
		n.q.keys.app[1][0], n.q.keys.app[1][1] = key, next
		keys[1] = key
	}
	for owner := 0; owner < 2; owner++ {
		plain := n09ObservedNewCID(1, []byte("shared!!"))
		plain[len(plain)-1] ^= byte(owner)
		event := n.dispatch(t, owner, uint64(owner+3), n09ObservedCIDPacket(t, keys[owner], n.cid[1-owner], 1, 0, plain))
		require.Equal(t, "decoded", event.Status)
		require.Equal(t, owner, event.Direction)
	}
	return n, keys
}

func sharedCIDPNSnapshot(q *binQUIC) [2][3]quicPNSpace {
	copy := q.spaces
	for d := range copy {
		for s := range copy[d] {
			copy[d][s].seen = append([]uint64(nil), q.spaces[d][s].seen...)
			if q.spaces[d][s].rejected != nil {
				copy[d][s].rejected = map[uint64]ProtocolError{}
				for pn, err := range q.spaces[d][s].rejected {
					copy[d][s].rejected[pn] = err
				}
			}
		}
	}
	return copy
}

func TestQUICSharedCIDMigratedRoleAndBadTagRecovery(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for dir := 0; dir < 2; dir++ {
				for _, future := range []bool{false, true} {
					t.Run(fmt.Sprintf("v%x/%s/dir%d/future%t", version, algorithm, dir, future), func(t *testing.T) {
						n, keys := sharedCIDTestOwner(t, version, algorithm, true)
						key, phase := keys[dir], 0
						if future {
							key, phase = n.q.keys.app[dir][1], 1
						}
						good := n09ObservedCIDPacket(t, key, []byte("shared!!"), 2, phase, []byte{1})
						bad := bytes.Clone(good)
						bad[len(bad)-1] ^= 1
						base := earlyRecoveryBase(n.f, n.domain, 5)
						base.Source, base.Destination = "198.51.100.18:51000", "198.51.100.19:443"
						before := n.q.cloneCIDState()
						pn := sharedCIDPNSnapshot(n.q)
						ring := *n.q.keys
						keyPointer := n.q.keys
						epoch, floor, first, last := n.q.keyPhase, n.q.keyFloor, n.q.keyFirst, n.q.keyLast
						charge := n.a.buffered.Load()
						touched := n.a.udpSessions.lru.Front().Value.(*binUDPEntry).touched
						events, handled := n.a.decodeQUICDatagram(base, bad)
						require.True(t, handled)
						require.Len(t, events, 1)
						e := events[0]
						require.Equal(t, "unresolved-CID-endpoint", e.Admission)
						require.Zero(t, e.FlowID)
						require.Nil(t, e.Session)
						require.Equal(t, len(bad), e.Length)
						require.Equal(t, pn, sharedCIDPNSnapshot(n.q))
						require.Equal(t, before.cids, n.q.cids)
						require.Equal(t, before.cidSequences, n.q.cidSequences)
						require.Equal(t, before.cidTokens, n.q.cidTokens)
						require.Same(t, keyPointer, n.q.keys)
						require.Equal(t, ring, *n.q.keys)
						require.Equal(t, epoch, n.q.keyPhase)
						require.Equal(t, floor, n.q.keyFloor)
						require.Equal(t, first, n.q.keyFirst)
						require.Equal(t, last, n.q.keyLast)
						require.Equal(t, charge, n.a.buffered.Load())
						require.Equal(t, touched, n.a.udpSessions.lru.Front().Value.(*binUDPEntry).touched)
						require.Empty(t, n.q.streams)
						// No observed tuple exists at this migrated address. Exact protection,
						// including a speculative future epoch, is the only direction evidence.
						events, handled = n.a.decodeQUICDatagram(base, good)
						require.True(t, handled)
						require.Len(t, events, 1)
						n.a.emit(events[0])
						e = events[0]
						require.Equal(t, "decoded", e.Status)
						require.Equal(t, dir, e.Direction)
						require.Equal(t, n.f.id, e.FlowID)
						require.Equal(t, true, e.Session["Authentication Verified"])
						require.Equal(t, phase, n.q.keyPhase[dir])
						require.Equal(t, 0, n.q.keyPhase[1-dir])
						require.Empty(t, n.q.streams)
					})
				}
			}
		}
	}
}

func TestQUICSharedCIDAmbiguousEndpointDoesNotChooseMapWinner(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		t.Run(fmt.Sprintf("v%x", version), func(t *testing.T) {
			n, _ := sharedCIDTestOwner(t, version, quicAEADAES128GCM, false)
			base := earlyRecoveryBase(n.f, n.domain, 5)
			base.Source, base.Destination = "198.51.100.18:51000", "198.51.100.19:443"
			wire := n09ObservedCIDPacket(t, n.key, []byte("shared!!"), 2, 0, []byte{1})
			before := sharedCIDPNSnapshot(n.q)
			keys := *n.q.keys
			for i := 0; i < 8; i++ {
				events, handled := n.a.decodeQUICDatagram(base, wire)
				require.True(t, handled)
				require.Len(t, events, 1)
				require.Equal(t, "unresolved-CID-endpoint", events[0].Admission)
				require.Zero(t, events[0].FlowID)
				require.Nil(t, events[0].Session)
				require.Equal(t, before, sharedCIDPNSnapshot(n.q))
				require.Equal(t, keys, *n.q.keys)
			}
			good := n.dispatch(t, 1, 6, wire)
			require.Equal(t, "decoded", good.Status)
			require.Equal(t, 1, good.Direction)
			require.Equal(t, n.f.id, good.FlowID)
		})
	}
}

func TestQUICSharedCIDRetiredGenerationKeepsOnlyObservedRole(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for dir := 0; dir < 2; dir++ {
			t.Run(fmt.Sprintf("v%x/dir%d", version, dir), func(t *testing.T) {
				n, _ := sharedCIDTestOwner(t, version, quicAEADAES128GCM, true)
				wire := n09ObservedCIDPacket(t, n.key, []byte("shared!!"), 2, 0, []byte{1})
				n.f.retireQUICUDP()
				require.Nil(t, n.q.keys)
				e := n.dispatch(t, dir, 5, wire)
				require.Equal(t, "observed-retired-CID", e.Admission)
				require.Equal(t, dir, e.Direction)
				require.Equal(t, n.f.id, e.FlowID)
				require.Nil(t, e.Session)
				base := earlyRecoveryBase(n.f, n.domain, 6)
				base.Source, base.Destination = "198.51.100.18:51000", "198.51.100.19:443"
				events, handled := n.a.decodeQUICDatagram(base, wire)
				require.True(t, handled)
				require.Len(t, events, 1)
				require.Equal(t, "observed-retired-CID", events[0].Admission)
				require.Zero(t, events[0].FlowID)
				require.Nil(t, events[0].Session)
				require.Nil(t, n.q.keys)
			})
		}
	}
}

func TestQUICSharedCIDLongHeaderBudgetPrecedesKeyCommit(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		t.Run(fmt.Sprintf("v%x", version), func(t *testing.T) {
			n := n09ObservedCIDOwner(t, version, quicAEADAES128GCM, [2]bool{}, false)
			before := n.q.cloneCIDState()
			keys := n.q.keys
			ring := *keys
			initial := bytes.Clone(n.q.initialDCID)
			// Two existing issuer-qualified routes: ODCID and client SCID. A server
			// selecting the same SCID bytes requires a third route, not a free overwrite.
			wire := versionedTestInitial(t, version, []byte("seedcid8"), n.cid[0], n.cid[0], nil, 0, 1)
			info, err := n.q.consume(1, wire, 2)
			require.ErrorContains(t, err, "CID budget")
			require.Equal(t, true, info["Authentication Verified"])
			require.Equal(t, before.cids, n.q.cids)
			require.Equal(t, before.cidSequences, n.q.cidSequences)
			require.Equal(t, before.cidTokens, n.q.cidTokens)
			require.Nil(t, n.q.cidSequences[1])
			require.Same(t, keys, n.q.keys)
			require.Equal(t, ring, *keys)
			require.Equal(t, initial, n.q.initialDCID)
			require.Equal(t, before.zeroCID, n.q.zeroCID)
		})
	}
}

func FuzzQUICSharedCIDEndpointIsolation(f *testing.F) {
	for _, seed := range []byte{0, 1, 2, 7, 16, 31, 63, 127} {
		f.Add(seed, []byte{0, 1, 2})
	}
	f.Fuzz(func(t *testing.T, flags byte, data []byte) {
		version := quicVersion1
		if flags&1 != 0 {
			version = quicVersion2
		}
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[(flags>>1)%3]
		dir := int(flags>>3) & 1
		distinct := flags&16 != 0
		n, keys := sharedCIDTestOwner(t, version, algorithm, distinct)
		before := sharedCIDPNSnapshot(n.q)
		ring := *n.q.keys
		wire := n09ObservedCIDPacket(t, keys[dir], []byte("shared!!"), 2, 0, []byte{1})
		bad := flags&32 != 0
		if bad {
			at := len(wire) - 1
			if len(data) > 0 {
				at = len(wire) - 16 + int(data[0])%16
			}
			wire[at] ^= 1
		}
		base := earlyRecoveryBase(n.f, n.domain, 5)
		base.Source, base.Destination = "198.51.100.18:51000", "198.51.100.19:443"
		events, handled := n.a.decodeQUICDatagram(base, wire)
		require.True(t, handled)
		require.Len(t, events, 1)
		if !distinct || bad {
			require.Equal(t, "unresolved-CID-endpoint", events[0].Admission)
			require.Zero(t, events[0].FlowID)
			require.Equal(t, before, sharedCIDPNSnapshot(n.q))
			require.Equal(t, ring, *n.q.keys)
		} else {
			n.a.emit(events[0])
			require.Equal(t, "decoded", events[0].Status)
			require.Equal(t, dir, events[0].Direction)
			require.Equal(t, n.f.id, events[0].FlowID)
		}
		require.Empty(t, n.q.streams)
	})
}

func TestQUICCIDPrefixWithoutKeysRequiresContext(t *testing.T) {
	for owner := 0; owner < 2; owner++ {
		for _, retired := range []bool{false, true} {
			t.Run(fmt.Sprintf("owner%d/retired%t", owner, retired), func(t *testing.T) {
				q := &binQUIC{native: true, cids: map[string]int{"prefix": 1 << owner}}
				if retired {
					q.cidRetired[owner] = map[uint64]string{1: "prefix-long"}
				} else {
					q.cids["prefix-long"] = 1 << owner
				}
				wire := append([]byte{0x40}, []byte("prefix-long")...)
				wire = append(wire, make([]byte, 32)...)
				h, err := q.wireHeader(wire, 1-owner)
				require.ErrorContains(t, err, "CID length is ambiguous")
				require.Empty(t, h.dcid)
				require.Nil(t, q.keys)
			})
		}
	}
}

func TestQUICCIDPrefixLengthAndRetiredPacketIdentity(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for owner := 0; owner < 2; owner++ {
				for _, retired := range []string{"none", "short", "long"} {
					t.Run(fmt.Sprintf("v%x/%s/owner%d/retired-%s", version, algorithm, owner, retired), func(t *testing.T) {
						n := n09ObservedCIDOwner(t, version, algorithm, [2]bool{}, true)
						n.q.byteLimit = 1 << 20
						cids := [][]byte{[]byte("prefix!!"), []byte("prefix!!long")}
						plain := append(n09ObservedNewCID(1, cids[0]), n09ObservedNewCID(2, cids[1])...)
						require.Equal(t, "decoded", n.dispatch(t, owner, 3, n09ObservedCIDPacket(t, n.key, n.cid[1-owner], 1, 0, plain)).Status)
						if retired != "none" {
							seq := byte(1)
							if retired == "long" {
								seq = 2
							}
							require.Equal(t, "decoded", n.dispatch(t, 1-owner, 4, n09ObservedCIDPacket(t, n.key, n.cid[owner], 1, 0, []byte{0x19, seq})).Status)
						}
						dir := 1 - owner
						for index, cid := range cids {
							wire := n09ObservedCIDPacket(t, n.key, cid, uint64(index+2), 0, []byte{1})
							before := sharedCIDPNSnapshot(n.q)
							keys := n.q.keys
							ring := *keys
							for trial := 0; trial < 32; trial++ {
								h, err := n.q.wireHeader(wire, dir)
								require.NoError(t, err)
								require.Equal(t, cid, h.dcid, "protection must select the CID length, never map iteration")
								require.Equal(t, len(cid)+1, h.payloadOff)
							}
							require.Equal(t, before, sharedCIDPNSnapshot(n.q))
							require.Same(t, keys, n.q.keys)
							require.Equal(t, ring, *keys)
							e := n.dispatch(t, dir, uint64(index+5), wire)
							require.Equal(t, "decoded", e.Status)
							require.Equal(t, dir, e.Direction)
							require.Equal(t, n.f.id, e.FlowID)
							require.Equal(t, true, e.Session["Authentication Verified"])
							if retired == []string{"short", "long"}[index] {
								require.Equal(t, true, e.Session["Retired Destination CID"])
								require.False(t, n.q.hasNativeCID(owner, string(cid)), "a late packet cannot reactivate the retired CID")
							} else {
								require.NotEqual(t, true, e.Session["Retired Destination CID"])
							}
						}
					})
				}
			}
		}
	}
}

func FuzzQUICCIDPrefixProtectionIsolation(f *testing.F) {
	for _, flags := range []byte{0, 1, 7, 16, 31, 63, 127, 255} {
		f.Add(flags, []byte{1, 2, 3})
	}
	f.Fuzz(func(t *testing.T, flags byte, data []byte) {
		version := quicVersion1
		if flags&1 != 0 {
			version = quicVersion2
		}
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[(flags>>1)%3]
		owner := int(flags>>3) & 1
		shortLength := 1
		longLength := 20
		if len(data) > 0 {
			shortLength = 1 + int(data[0])%19
		}
		if len(data) > 1 {
			longLength = shortLength + 1 + int(data[1])%(20-shortLength)
		}
		short := bytes.Repeat([]byte{0xe0}, shortLength)
		long := append(bytes.Clone(short), bytes.Repeat([]byte{0xe1}, longLength-shortLength)...)
		n := n09ObservedCIDOwner(t, version, algorithm, [2]bool{}, true)
		n.q.byteLimit = 1 << 20
		plain := append(n09ObservedNewCID(1, short), n09ObservedNewCID(2, long)...)
		require.Equal(t, "decoded", n.dispatch(t, owner, 3, n09ObservedCIDPacket(t, n.key, n.cid[1-owner], 1, 0, plain)).Status)
		key, phase := n.key, 0
		dir := 1 - owner
		if flags&16 != 0 {
			key, phase = n.q.keys.app[dir][1], 1
		}
		wire := n09ObservedCIDPacket(t, key, long, 2, phase, []byte{1})
		bad := flags&32 != 0
		if bad {
			wire[len(wire)-1] ^= 1
		}
		unknown := flags&64 != 0
		if unknown {
			n.q.keys = nil
		}
		before := sharedCIDPNSnapshot(n.q)
		cids := n.q.cloneCIDState()
		keys := n.q.keys
		h, err := n.q.wireHeader(wire, dir)
		if bad || unknown {
			require.ErrorContains(t, err, "CID length is ambiguous")
			require.Empty(t, h.dcid)
		} else {
			require.NoError(t, err)
			require.Equal(t, long, h.dcid)
			require.Equal(t, len(long)+1, h.payloadOff)
		}
		require.Equal(t, before, sharedCIDPNSnapshot(n.q))
		require.Equal(t, cids.cids, n.q.cids)
		require.Equal(t, cids.cidSequences, n.q.cidSequences)
		require.Equal(t, cids.cidTokens, n.q.cidTokens)
		require.Same(t, keys, n.q.keys)
		require.Empty(t, n.q.streams)
	})
}
