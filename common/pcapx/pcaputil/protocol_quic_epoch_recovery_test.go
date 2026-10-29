package pcaputil

import (
	"bytes"
	"container/list"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func missedQUICEpochKeys(t *testing.T, key *quicTrafficKeys, count int) []*quicTrafficKeys {
	t.Helper()
	keys := []*quicTrafficKeys{key}
	for len(keys) <= count {
		next, err := quicNextPhase(keys[len(keys)-1])
		require.NoError(t, err)
		require.Equal(t, key.hp, next.hp)
		keys = append(keys, next)
	}
	return keys
}

func assertQUICEpochProbeUnchanged(t *testing.T, q *binQUIC, dir int, wire []byte, want bool) {
	t.Helper()
	keys, spaces, phases := cloneQUICKeyring(q.keys), q.spaces, q.keyPhase
	previous := [2][]quicPriorKey{append([]quicPriorKey(nil), q.previousKeys[0]...), append([]quicPriorKey(nil), q.previousKeys[1]...)}
	floor, floorSet, first, firstSet, last := q.keyFloor, q.keyFloorSet, q.keyFirst, q.keyFirstSet, q.keyLast
	require.Equal(t, want, q.authenticatesExistingWire(dir, wire))
	require.Equal(t, keys, q.keys)
	require.Equal(t, spaces, q.spaces)
	require.Equal(t, phases, q.keyPhase)
	require.Equal(t, previous, q.previousKeys)
	require.Equal(t, floor, q.keyFloor)
	require.Equal(t, floorSet, q.keyFloorSet)
	require.Equal(t, first, q.keyFirst)
	require.Equal(t, firstSet, q.keyFirstSet)
	require.Equal(t, last, q.keyLast)
}

func TestQUICMissedKeyEpochRecoveryRetainsAuthenticatedReordering(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for dir := 0; dir < 2; dir++ {
				for steps := 2; steps <= quicForwardEpochLimit; steps++ {
					t.Run(fmt.Sprintf("v%x/%s/dir%d/steps%d", version, algorithm, dir, steps), func(t *testing.T) {
						q, key := quicConformanceContext(t, version, algorithm)
						epochs := missedQUICEpochKeys(t, key, steps+1)
						old := epochQUICPacket(t, key, nil, 10, 0, 2)
						_, err := q.consume(dir, old, 128)
						require.NoError(t, err)
						wire := epochQUICPacket(t, epochs[steps], nil, 100, steps&1, 2)
						bad := bytes.Clone(wire)
						bad[len(bad)-1] ^= 1
						assertQUICEpochProbeUnchanged(t, q, dir, bad, false)
						before := cloneQUICKeyring(q.keys)
						_, err = q.consume(dir, bad, 128)
						require.ErrorContains(t, err, "authentication failed")
						require.Equal(t, before, q.keys)
						require.EqualValues(t, 10, q.spaces[dir][quicSpaceApplication].largest)
						assertQUICEpochProbeUnchanged(t, q, dir, wire, true)
						info, err := q.consume(dir, wire, 128)
						require.NoError(t, err)
						require.Equal(t, true, info["Authentication Verified"])
						require.EqualValues(t, steps, info["Key Update Steps"])
						require.EqualValues(t, steps-1, info["Unobserved Key Epochs"])
						require.Equal(t, epochs[steps].secret, q.keys.app[dir][steps&1].secret)
						require.Len(t, q.previousKeys[dir], steps)
						// Equal phase bits cannot make an old generation current again.
						info, err = q.consume(dir, old, 128)
						require.NoError(t, err)
						require.Equal(t, true, info["Duplicate"])
						for earlier := 1; earlier < steps; earlier++ {
							info, err = q.consume(dir, epochQUICPacket(t, epochs[earlier], nil, uint64(20+earlier), earlier&1, 2), 128)
							require.NoError(t, err)
							require.Equal(t, true, info["Authentication Verified"])
							require.Equal(t, steps&1, q.keyPhase[dir])
							require.EqualValues(t, 100, q.spaces[dir][quicSpaceApplication].largest)
						}
						// A current-generation reordered packet tightens the old-key bound.
						_, err = q.consume(dir, epochQUICPacket(t, epochs[steps], nil, 90, steps&1, 2), 128)
						require.NoError(t, err)
						info, err = q.consume(dir, epochQUICPacket(t, epochs[steps-1], nil, 95, (steps-1)&1, 2), 128)
						require.ErrorContains(t, err, "packet number order")
						require.Equal(t, true, info["Authentication Verified"])
						require.NotContains(t, info, "Frames")
						require.EqualValues(t, 100, q.spaces[dir][quicSpaceApplication].largest)
						_, err = q.consume(dir, epochQUICPacket(t, epochs[steps+1], nil, 101, (steps+1)&1, 2), 128)
						require.NoError(t, err)
						require.LessOrEqual(t, len(q.previousKeys[dir]), quicPreviousEpochLimit)
					})
				}
			}
		}
	}
}

func TestQUICMissedKeyEpochCandidatesDoNotCommitInvalidPackets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pn       uint64
		reserved byte
		plain    []byte
	}{
		{"reserved-bits", 100, 8, []byte{1}},
		{"invalid-grammar", 100, 0, []byte{0xff}},
		{"client-handshake-done", 100, 0, []byte{0x1e}},
		{"backward-future-pn", 9, 0, []byte{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
			epochs := missedQUICEpochKeys(t, key, 2)
			_, err := q.consume(0, epochQUICPacket(t, key, nil, 10, 0, 2), 128)
			require.NoError(t, err)
			wire := quicConformancePacket(t, epochs[2], "1-RTT", tc.pn, tc.reserved, tc.plain)
			assertQUICEpochProbeUnchanged(t, q, 0, wire, true)
			before := cloneQUICKeyring(q.keys)
			info, err := q.consume(0, wire, 128)
			require.Error(t, err)
			require.Equal(t, true, info["Authentication Verified"])
			require.Equal(t, before, q.keys)
			require.Zero(t, q.keyPhase[0])
			require.Empty(t, q.previousKeys[0])
			require.Empty(t, q.streams)
			require.NotContains(t, info, "Frames")
			_, err = q.consume(0, epochQUICPacket(t, epochs[2], nil, 99, 0, 2), 128)
			require.NoError(t, err, "a refused future epoch cannot poison its next valid packet")
			_, err = q.consume(0, wire, 128)
			require.Error(t, err, "the original refused PN cannot become a clean duplicate after recovery")
		})
	}
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	epochs := missedQUICEpochKeys(t, key, quicForwardEpochLimit+1)
	wire := epochQUICPacket(t, epochs[quicForwardEpochLimit+1], nil, 100, (quicForwardEpochLimit+1)&1, 2)
	assertQUICEpochProbeUnchanged(t, q, 0, wire, false)
	_, err := q.consume(0, wire, 128)
	require.ErrorContains(t, err, "authentication failed")
	require.Empty(t, q.previousKeys[0])
	require.False(t, q.spaces[0][quicSpaceApplication].init)
}

func TestQUICMissedEpochPriorGenerationsConstrainOneAnother(t *testing.T) {
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	epochs := missedQUICEpochKeys(t, key, 2)
	for _, packet := range [][]byte{
		epochQUICPacket(t, key, nil, 57, 0, 2),
		epochQUICPacket(t, epochs[2], nil, 229, 0, 2),
	} {
		_, err := q.consume(0, packet, 128)
		require.NoError(t, err)
	}
	for _, tc := range []struct {
		epoch int
		pn    uint64
		valid bool
	}{{1, 20, false}, {1, 100, true}, {0, 150, false}, {0, 90, true}, {1, 80, false}, {2, 99, false}, {2, 101, true}} {
		packet := epochQUICPacket(t, epochs[tc.epoch], nil, tc.pn, tc.epoch&1, 2)
		assertQUICEpochProbeUnchanged(t, q, 0, packet, true)
		info, err := q.consume(0, packet, 128)
		require.Equal(t, true, info["Authentication Verified"])
		if tc.valid {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "packet number order")
			require.NotContains(t, info, "Frames")
		}
		require.Equal(t, epochs[2].secret, q.keys.app[0][0].secret)
		require.EqualValues(t, 229, q.spaces[0][quicSpaceApplication].largest)
	}
}

func TestQUICMissingPhaseSlotsNeverInstallNilKeys(t *testing.T) {
	for slot := 0; slot < 2; slot++ {
		q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
		q.keyPhase[0] = slot
		q.keys.app[0] = [2]*quicTrafficKeys{}
		q.keys.app[0][slot] = key
		wire := epochQUICPacket(t, key, nil, 10, slot, 2)
		assertQUICEpochProbeUnchanged(t, q, 0, wire, true)
		_, err := q.consume(0, wire, 128)
		require.NoError(t, err)
		before := cloneQUICKeyring(q.keys)
		// The fallback low-level AEAD key is valid, but the opposite phase
		// has no configured key. Keep ownership without promoting nil slots.
		wire = epochQUICPacket(t, key, nil, 11, 1-slot, 2)
		assertQUICEpochProbeUnchanged(t, q, 0, wire, true)
		info, err := q.consume(0, wire, 128)
		require.ErrorContains(t, err, "no configured traffic key")
		require.Equal(t, true, info["Authentication Verified"])
		require.Equal(t, before, q.keys)
		require.Equal(t, slot, q.keyPhase[0])
		require.EqualValues(t, 10, q.spaces[0][quicSpaceApplication].largest)
	}
}

func TestQUICFirstObservedFutureEpochRecoversAfterRefusedLargerPN(t *testing.T) {
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	epochs := missedQUICEpochKeys(t, key, 2)
	bad := quicConformancePacket(t, epochs[2], "1-RTT", 100, 8, []byte{1})
	_, err := q.consume(0, bad, 128)
	require.ErrorContains(t, err, "reserved bits")
	require.False(t, q.keyFirstSet[0])
	require.EqualValues(t, 100, q.spaces[0][quicSpaceApplication].largest)
	good := epochQUICPacket(t, epochs[2], nil, 99, 0, 2)
	assertQUICEpochProbeUnchanged(t, q, 0, good, true)
	info, err := q.consume(0, good, 128)
	require.NoError(t, err)
	require.Equal(t, true, info["Authentication Verified"])
	require.Equal(t, epochs[2].secret, q.keys.app[0][0].secret)
	_, err = q.consume(0, bad, 128)
	require.ErrorContains(t, err, "reserved bits")
}

func TestQUICFutureEpochPreflightPreservesAssemblerMemoryBudget(t *testing.T) {
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	q.byteLimit = 480
	q.streams = map[uint64]*quicStream{0: {id: 0}}
	r := &q.streams[0].reassembly[0]
	r.data = make([]byte, 3, 128)
	copy(r.data, "abc")
	r.pending = []quicPiece{{off: 100, data: make([]byte, 10, 256)}}
	_, err := q.consume(0, epochQUICPacket(t, key, nil, 10, 0, 2), 128)
	require.NoError(t, err)
	epochs := missedQUICEpochKeys(t, key, 2)
	plain := append([]byte{0x0e, 0}, quicPutVarint(200)...)
	plain = append(plain, 50)
	plain = append(plain, make([]byte, 50)...)
	before := cloneQUICKeyring(q.keys)
	info, err := q.consume(0, quicConformancePacket(t, epochs[2], "1-RTT", 100, 0, plain), 128)
	require.ErrorContains(t, err, "retained range byte budget")
	require.Equal(t, true, info["Authentication Verified"])
	require.Equal(t, before, q.keys)
	require.Empty(t, q.previousKeys[0])
	require.EqualValues(t, 416, r.memory())
	require.Len(t, r.pending, 1)
}

func TestQUICFutureEpochTransportStateRefusalIsAtomic(t *testing.T) {
	streamFrame := func(id uint64, value string) []byte {
		out := append([]byte{0x0a}, quicPutVarint(id)...)
		out = append(out, quicPutVarint(uint64(len(value)))...)
		return append(out, []byte(value)...)
	}
	newCID := append([]byte{0x18, 1, 0, 8}, []byte("new-cid8")...)
	newCID = append(newCID, make([]byte, 16)...)
	for _, steps := range []int{1, 2} {
		for _, tc := range []struct {
			name string
			body []byte
			want string
		}{
			{"changed-cid", newCID, "CID sequence changed"},
			{"unobserved-retirement", []byte{0x19, 11}, "retirement of unobserved CID"},
			{"conflicting-stream", streamFrame(0, "xyz"), "conflicting retransmission"},
			{"changed-final-size", []byte{4, 0, 0, 2}, "final size before received bytes"},
			{"conflicting-crypto", []byte{6, 0, 1, 'X'}, "conflicting retransmission"},
		} {
			t.Run(fmt.Sprintf("steps%d/%s", steps, tc.name), func(t *testing.T) {
				q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
				q.byteLimit = 4096
				q.cids = map[string]int{"old-cid8": 0}
				q.cidSequences[0] = map[uint64]string{1: "old-cid8"}
				q.streams = map[uint64]*quicStream{0: {id: 0}}
				q.streams[0].reassembly[0].data = []byte("abc")
				q.cryptoData[0][quicSpaceApplication].data = []byte("abcd")
				_, err := q.consume(0, epochQUICPacket(t, key, nil, 10, 0, 2), 128)
				require.NoError(t, err)
				epochs := missedQUICEpochKeys(t, key, steps)
				// A preceding valid new stream must also remain undelivered.
				plain := append(streamFrame(4, "ok"), tc.body...)
				wire := quicConformancePacket(t, epochs[steps], "1-RTT", 100, byte((steps&1)<<2), plain)
				assertQUICEpochProbeUnchanged(t, q, 0, wire, true)
				before := cloneQUICKeyring(q.keys)
				info, err := q.consume(0, wire, 128)
				require.ErrorContains(t, err, tc.want)
				require.Equal(t, true, info["Authentication Verified"])
				require.Equal(t, before, q.keys)
				require.Empty(t, q.previousKeys[0])
				require.Equal(t, "old-cid8", q.cidSequences[0][1])
				require.NotContains(t, q.streams, uint64(4))
				require.Equal(t, []byte("abc"), q.streams[0].reassembly[0].data)
				require.Equal(t, []byte("abcd"), q.cryptoData[0][quicSpaceApplication].data)
				require.NotContains(t, info, "Frames")
				_, err = q.consume(0, epochQUICPacket(t, epochs[steps], nil, 99, steps&1, 2), 128)
				require.NoError(t, err)
			})
		}
	}
}

func TestQUICMissedKeyEpochZeroCIDReuseStillRequiresUniqueOwnership(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer a.closeUDPSessions()
	store := &binUDPStore{entries: map[binUDPKey]*list.Element{}}
	a.udpSessions = store
	var flows []*binFlow
	var keys []*quicTrafficKeys
	for i := 0; i < 2; i++ {
		q, _ := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
		key, err := quicDeriveTraffic(bytes.Repeat([]byte{byte(20 + i)}, 32), quicAEADAES128GCM)
		require.NoError(t, err)
		next, err := quicNextPhase(key)
		require.NoError(t, err)
		q.keys.app[0], q.keys.app[1] = [2]*quicTrafficKeys{key, next}, [2]*quicTrafficKeys{key, next}
		f := &binFlow{a: a, id: uint64(70 + i), protocol: "quic", quic: q, endpoints: [2]string{"client", "server"}}
		id := binUDPKey{fmt.Sprint(i), "server", CaptureDomain{}}
		store.entries[id] = store.lru.PushBack(&binUDPEntry{id, f, time.Unix(1, 0)})
		flows, keys = append(flows, f), append(keys, key)
	}
	base := &ProtocolEvent{Source: "client", Destination: "server", Timestamp: time.Unix(2, 0)}
	epochs := missedQUICEpochKeys(t, keys[0], 2)
	wire := epochQUICPacket(t, epochs[2], nil, 100, 0, 2)
	events, ok := a.decodeQUICDatagram(base, wire)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, flows[0].id, events[0].FlowID)
	require.Equal(t, true, events[0].Session["Authentication Verified"])
	require.Equal(t, epochs[2].secret, flows[0].quic.keys.app[0][0].secret)
	require.Equal(t, keys[1].secret, flows[1].quic.keys.app[0][0].secret)
	base.Domain.Interface = 1
	_, ok = a.decodeQUICDatagram(base, wire)
	require.False(t, ok)
	base.Domain.Interface = 0
	flows[1].quic.keys.app = flows[0].quic.keys.app
	events, ok = a.decodeQUICDatagram(base, wire)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Zero(t, events[0].FlowID)
	require.Equal(t, "unresolved-zero-CID-generation", events[0].Admission)
}

func FuzzQUICMissedEpochAuthenticationAndStateIsolation(f *testing.F) {
	f.Add(uint16(129), uint8(2), uint8(0), uint8(1))
	f.Add(uint16(768), uint8(4), uint8(5), uint8(255))
	f.Fuzz(func(t *testing.T, offset uint16, count, kind, flip uint8) {
		version := quicVersion1
		if kind&1 != 0 {
			version = quicVersion2
		}
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[(kind>>1)%3]
		q, key := quicConformanceContext(t, version, algorithm)
		dir, steps := int(kind>>3&1), 2+int(count%3)
		epochs := missedQUICEpochKeys(t, key, steps)
		_, err := q.consume(dir, epochQUICPacket(t, key, nil, 100, 0, 2), 128)
		require.NoError(t, err)
		pn := 101 + uint64(offset%768)
		wire := epochQUICPacket(t, epochs[steps], nil, pn, steps&1, 1)
		bad := bytes.Clone(wire)
		bad[len(bad)-1] ^= flip | 1
		assertQUICEpochProbeUnchanged(t, q, dir, bad, false)
		_, err = q.consume(dir, bad, 128)
		require.Error(t, err)
		require.EqualValues(t, 100, q.spaces[dir][quicSpaceApplication].largest)
		assertQUICEpochProbeUnchanged(t, q, dir, wire, true)
		info, err := q.consume(dir, wire, 128)
		require.NoError(t, err)
		require.EqualValues(t, pn, info["Packet Number"])
		require.Equal(t, epochs[steps].secret, q.keys.app[dir][steps&1].secret)
		require.Len(t, q.previousKeys[dir], steps)
	})
}
