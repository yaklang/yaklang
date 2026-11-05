package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/chacha20poly1305"
)

type n09ObservedCID struct {
	a      *binParser
	f      *binFlow
	q      *binQUIC
	key    *quicTrafficKeys
	cid    [2][]byte
	domain CaptureDomain
	start  []*ProtocolEvent
}

// This constructor never assigns zeroCID. Actual native long-header Initials
// create and select the capture owner; their literal SCID length bytes alone
// establish the asymmetric zero choices. Application keys/ALPN are then an
// explicit post-handshake algorithm fixture, not a claim of real-peer TLS proof.
func n09ObservedCIDOwner(t *testing.T, version uint32, algorithm string, zero [2]bool, serverObserved bool, domain ...CaptureDomain) *n09ObservedCID {
	t.Helper()
	n := &n09ObservedCID{a: earlyRecoveryParser(t), domain: CaptureDomain{Section: 2, Interface: 7, Encapsulation: "/vlan:91"}, cid: [2][]byte{[]byte("client!!"), []byte("server!!")}}
	if len(domain) > 0 {
		n.domain = domain[0]
	}
	// Keep the pre-existing role fixture's observation namespace while the
	// actual native Initial path, rather than a hand-built flow, assigns it.
	n.a.flows.Store(90)
	for dir := 0; dir < 2; dir++ {
		if zero[dir] {
			n.cid[dir] = nil
		}
	}
	seed := []byte("seedcid8")
	clientWire := versionedTestInitial(t, version, seed, seed, n.cid[0], nil, 0, 0)
	require.Equal(t, byte(len(n.cid[0])), clientWire[6+len(seed)])
	base := &ProtocolEvent{Source: "192.0.2.1:40000", Destination: "192.0.2.2:443", Transport: "udp", Domain: n.domain}
	base.Timestamp, base.SourceBytes = earlyRecoveryBase(&binFlow{}, n.domain, 1).Timestamp, ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: 1, Domain: n.domain}}}
	events, handled := n.a.decodeQUICDatagram(base, clientWire)
	require.True(t, handled)
	require.Len(t, events, 1)
	n.a.emit(events[0])
	require.Equal(t, "decoded", events[0].Status)
	for _, entry := range n.a.udpSessions.entries {
		if flow := entry.Value.(*binUDPEntry).flow; flow.id == events[0].FlowID {
			n.f, n.q = flow, flow.quic
		}
	}
	require.NotNil(t, n.f)
	require.Equal(t, zero[0], n.q.zeroCID[0])
	n.start = append(n.start, events[0])
	if serverObserved {
		serverWire := versionedTestInitial(t, version, seed, n.cid[0], n.cid[1], nil, 0, 1)
		require.Equal(t, byte(len(n.cid[1])), serverWire[6+len(n.cid[0])])
		response := n.dispatch(t, 1, 2, serverWire)
		require.Equal(t, "decoded", response.Status)
		require.Equal(t, n.f.id, response.FlowID)
		require.Equal(t, zero, n.q.zeroCID)
		n.start = append(n.start, response)
	} else {
		require.False(t, n.q.zeroCID[1])
		require.Nil(t, n.q.cidSequences[1], "server SCID was never observed")
	}
	length := 32
	if algorithm == quicAEADAES256GCM {
		length = 48
	}
	key, err := quicDeriveTrafficVersion(bytes.Repeat([]byte{0x71}, length), algorithm, version)
	require.NoError(t, err)
	next, err := quicNextPhase(key)
	require.NoError(t, err)
	n.key = key
	for dir := 0; dir < 2; dir++ {
		n.q.keys.app[dir][0], n.q.keys.app[dir][1] = key, next
	}
	n.q.alpn, n.q.state = "h3", "application"
	return n
}

func (n *n09ObservedCID) dispatch(t *testing.T, dir int, frame uint64, wire []byte) *ProtocolEvent {
	t.Helper()
	base := earlyRecoveryBase(n.f, n.domain, frame)
	if dir == 1 {
		base.Source, base.Destination = base.Destination, base.Source
	}
	events, handled := n.a.decodeQUICDatagram(base, wire)
	require.True(t, handled)
	require.Len(t, events, 1)
	n.a.emit(events[0])
	return events[0]
}

func n09ObservedCIDPacket(t *testing.T, key *quicTrafficKeys, cid []byte, pn uint64, phase int, plain []byte) []byte {
	t.Helper()
	header := append([]byte{0x43 | byte(phase<<2)}, cid...)
	pnAt := len(header)
	header = binary.BigEndian.AppendUint32(header, uint32(pn))
	var packetCipher cipher.AEAD
	var err error
	if key.aead == quicAEADChaCha20Poly1305 {
		packetCipher, err = chacha20poly1305.New(key.key)
	} else {
		block, e := aes.NewCipher(key.key)
		require.NoError(t, e)
		packetCipher, err = cipher.NewGCM(block)
	}
	require.NoError(t, err)
	wire := append(bytes.Clone(header), packetCipher.Seal(nil, quicNonce(key.iv, pn), plain, header)...)
	mask, err := quicHeaderMask(key, wire[pnAt+4:pnAt+20])
	require.NoError(t, err)
	wire[0] ^= mask[0] & 31
	for i := 0; i < 4; i++ {
		wire[pnAt+i] ^= mask[i+1]
	}
	return wire
}

func n09ObservedNewCID(seq uint64, cid []byte) []byte {
	frame := append([]byte{0x18}, quicPutVarint(seq)...)
	frame = append(frame, 0, byte(len(cid)))
	frame = append(frame, cid...)
	return append(frame, bytes.Repeat([]byte{0x21}, 16)...)
}

func TestN09ObservedZeroCIDModeRefusesBeforeNativeCommit(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, dir := range []int{0, 1} {
				for _, frameName := range []string{"NEW_CONNECTION_ID", "RETIRE_CONNECTION_ID"} {
					for _, future := range []bool{false, true} {
						t.Run(fmt.Sprintf("v%x/%s/dir%d/%s/future%t", version, algorithm, dir, frameName, future), func(t *testing.T) {
							choice := [2]bool{}
							zeroEndpoint := dir
							if frameName == "RETIRE_CONNECTION_ID" {
								zeroEndpoint = 1 - dir
							}
							choice[zeroEndpoint] = true // deliberately asymmetric: prove the correct index
							n := n09ObservedCIDOwner(t, version, algorithm, choice, true)
							if dir == 1 {
								for i, sid := range []uint64{0, 4} {
									request := quicStreamFrame(sid, 0, false, h3RequestHeaders())
									e := n.dispatch(t, 0, uint64(3+i), n09ObservedCIDPacket(t, n.key, n.cid[1], uint64(i+1), 0, request))
									require.Equal(t, "decoded", e.Status)
								}
							}
							message := h3RequestHeaders()
							if dir == 1 {
								message = h3ResponseHeaders()
							}
							badFrame := n09ObservedNewCID(1, []byte("freshcid"))
							if frameName == "RETIRE_CONNECTION_ID" {
								badFrame = []byte{0x19, 0}
							}
							plain := quicStreamFrame(0, 0, false, message)
							plain = append(plain, badFrame...)
							plain = append(plain, quicStreamFrame(4, 0, false, message)...)
							key, phase := n.key, 0
							if future {
								key, phase = n.q.keys.app[dir][1], 1
							}
							beforeKeys := cloneQUICKeyring(n.q.keys)
							peerLargest := n.q.spaces[1-dir][quicSpaceApplication].largest
							wire := n09ObservedCIDPacket(t, key, n.cid[1-dir], 100, phase, plain)
							bad := n.dispatch(t, dir, 11, wire)
							require.Equal(t, "malformed", bad.Status)
							require.Equal(t, string(ErrMalformedMessage), bad.ExpertCode)
							require.Contains(t, bad.Error, "zero-length CID choice")
							require.Equal(t, true, bad.Session["Authentication Verified"])
							require.Equal(t, beforeKeys, n.q.keys)
							require.Zero(t, n.q.keyPhase[dir])
							require.Empty(t, n.q.previousKeys[dir])
							require.Equal(t, peerLargest, n.q.spaces[1-dir][quicSpaceApplication].largest)
							require.NotContains(t, n.q.cids, "freshcid")
							require.Empty(t, n.q.cidRetired[0])
							require.Empty(t, n.q.cidRetired[1])
							for _, sid := range []uint64{0, 4} {
								if dir == 0 {
									require.NotContains(t, n.q.streams, sid)
								} else {
									require.Zero(t, n.q.streams[sid].h3.headers[1])
									require.Empty(t, n.q.streams[sid].reassembly[1].data)
								}
							}
							bad.Session["SCID"] = []byte("caller!!")
							again := n.dispatch(t, dir, 12, wire)
							require.Equal(t, "malformed", again.Status)
							require.Equal(t, string(ErrMalformedMessage), again.ExpertCode)
							require.Equal(t, true, again.Session["Duplicate"])
							good := n.dispatch(t, dir, 13, n09ObservedCIDPacket(t, n.key, n.cid[1-dir], 99, 0, []byte{1}))
							require.Equal(t, "decoded", good.Status)
							peer := n.dispatch(t, 1-dir, 14, n09ObservedCIDPacket(t, n.key, n.cid[dir], 90, 0, []byte{1}))
							require.Equal(t, "decoded", peer.Status)
						})
					}
				}
			}
		}
	}
}

func TestN09ObservedNonzeroCIDNewAndRetireUseActualRoutes(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, dir := range []int{0, 1} {
			t.Run(fmt.Sprintf("v%x/dir%d", version, dir), func(t *testing.T) {
				n := n09ObservedCIDOwner(t, version, quicAEADAES128GCM, [2]bool{}, true)
				newCID := []byte("freshcid")
				issued := n.dispatch(t, 1-dir, 10, n09ObservedCIDPacket(t, n.key, n.cid[dir], 1, 0, n09ObservedNewCID(1, newCID)))
				require.Equal(t, "decoded", issued.Status)
				require.Equal(t, 1<<(1-dir), n.q.cids[string(newCID)])
				// Retire sequence1 while this packet uses the issuer's distinct
				// original CID0, satisfying the current-DCID prohibition.
				retired := n.dispatch(t, dir, 11, n09ObservedCIDPacket(t, n.key, n.cid[1-dir], 1, 0, []byte{0x19, 1}))
				require.Equal(t, "decoded", retired.Status)
				require.NotContains(t, n.q.cids, string(newCID))
				require.Equal(t, string(newCID), n.q.cidRetired[1-dir][1])
				require.Contains(t, n.q.cids, string(n.cid[1-dir]))
				require.Equal(t, [2]bool{}, n.q.zeroCID)
			})
		}
	}
}

func TestN09UnobservedServerSCIDDoesNotImplyZeroCIDChoice(t *testing.T) {
	n := n09ObservedCIDOwner(t, quicVersion1, quicAEADAES128GCM, [2]bool{}, false)
	require.Nil(t, n.q.cidSequences[1])
	newCID := []byte("freshcid")
	issued := n.dispatch(t, 1, 10, n09ObservedCIDPacket(t, n.key, n.cid[0], 1, 0, n09ObservedNewCID(1, newCID)))
	require.Equal(t, "decoded", issued.Status)
	require.Equal(t, 2, n.q.cids[string(newCID)])
	require.False(t, n.q.zeroCID[1])
}
