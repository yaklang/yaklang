package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This fixture supplies a captured native owner with known traffic secrets.
// All tested inputs are real AEAD/header-protected packets delivered through
// decodeQUICDatagram, not applyFrames or a declared-plaintext stream API.
func n09AuditOwner(t *testing.T, version uint32, algorithm string) (*binParser, *binQUIC) {
	t.Helper()
	domain := CaptureDomain{Section: 2, Interface: 1, Encapsulation: "/vlan:7/pppoe:02:00:00:00:01:01~02:00:00:00:01:02:30"}
	n := n09ObservedCIDOwner(t, version, algorithm, [2]bool{}, true, domain)
	a, q := n.a, n.q
	q.byteLimit, q.alpn = 4096, "" // transport-role fixture, without an HTTP/3 message claim
	t.Cleanup(func() { a.closeUDPSessions(); require.Zero(t, a.buffered.Load()) })
	// Both packet directions already have an authenticated current epoch.
	for dir := 0; dir < 2; dir++ {
		e := n09AuditDatagram(t, a, q, dir, 0, 10, []byte{1})
		require.Equal(t, "decoded", e.Status, e.Error)
	}
	return a, q
}

func n09AuditDatagram(t *testing.T, a *binParser, q *binQUIC, dir, phase int, pn uint64, plain []byte) *ProtocolEvent {
	t.Helper()
	base := &ProtocolEvent{Transport: "udp", Timestamp: time.Unix(2, int64(pn)), Domain: CaptureDomain{Section: 2, Interface: 1, Encapsulation: "/vlan:7/pppoe:02:00:00:00:01:01~02:00:00:00:01:02:30"}, Source: "192.0.2.1:40000", Destination: "192.0.2.2:443"}
	if dir == 1 {
		base.Source, base.Destination = base.Destination, base.Source
	}
	base.SourceBytes = ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: 100 + a.ids.Load(), Domain: base.Domain}}}
	key := q.keys.app[dir][phase]
	cid := []byte("server!!")
	if dir == 1 {
		cid = []byte("client!!")
	}
	wire := n09ObservedCIDPacket(t, key, cid, pn, phase, plain)
	events, handled := a.decodeQUICDatagram(base, wire)
	require.True(t, handled, "captured tuple/key owner must be selected by the native datagram entry")
	require.Len(t, events, 1)
	require.Equal(t, wire, events[0].Raw)
	require.Equal(t, uint64(91), events[0].FlowID)
	require.Equal(t, true, events[0].Session["Authentication Verified"])
	// Native ingress publishes after datagram decoding. ExpertCode is filled
	// by that real publication step, while the private refusal already exists.
	a.emit(events[0])
	return events[0]
}

func n09AuditFrame(kind string, sid uint64) []byte {
	switch kind {
	case "STREAM":
		return quicStreamFrame(sid, 0, true, []byte{0x11})
	case "RESET_STREAM":
		return quicResetStream(sid, 0, 0)
	case "STOP_SENDING":
		return quicStopSending(sid, 0)
	}
	panic("unexpected audit frame")
}

func TestN09UniStreamRoleRefusalPrecedesNativeStateAndEpochCommit(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, dir := range []int{0, 1} {
				for _, phase := range []int{0, 1} {
					for _, kind := range []string{"STREAM", "RESET_STREAM", "STOP_SENDING"} {
						t.Run(fmt.Sprintf("v%x/%s/dir%d/phase%d/%s", version, algorithm, dir, phase, kind), func(t *testing.T) {
							a, q := n09AuditOwner(t, version, algorithm)
							sid := uint64(2 + 1 - dir)
							if kind == "STOP_SENDING" {
								sid = uint64(2 + dir)
							}
							// A valid earlier CID instruction must not commit
							// before a statically invalid role later in packet.
							prefix := []byte{0x18, 1, 0, 8, 'r', 'o', 'l', 'e', 'c', 'i', 'd', '8'}
							prefix = append(prefix, make([]byte, 16)...)
							plain := append(prefix, n09AuditFrame(kind, sid)...)
							plain = append(plain, quicStreamFrame(uint64(6+dir), 0, false, []byte{0x22})...)
							keyring := cloneQUICKeyring(q.keys)
							peerPN, peerPhase := q.spaces[1-dir][quicSpaceApplication], q.keyPhase[1-dir]
							cidCounts := [2]int{len(q.cids), len(q.cidSequences[dir])}
							beforeOrder := q.earlyOrder
							e := n09AuditDatagram(t, a, q, dir, phase, 11, plain)
							require.Equal(t, "malformed", e.Status, "authenticated uni-stream frame role must refuse")
							require.Equal(t, string(ErrMalformedMessage), e.ExpertCode)
							require.Contains(t, e.Session["Payload Delivery"], "not delivered")
							require.Equal(t, plain, e.Session["Rejected Packet Payload"])
							require.NotContains(t, e.Session, "Frames")
							require.Empty(t, q.streams, "no invalid/suffix stream allocation or reset/stop state")
							require.Equal(t, cidCounts, [2]int{len(q.cids), len(q.cidSequences[dir])}, "earlier CID frame cannot commit")
							require.Equal(t, keyring, q.keys, "candidate future epoch must remain uncommitted")
							require.Zero(t, q.keyPhase[dir])
							require.Equal(t, peerPhase, q.keyPhase[1-dir])
							require.Equal(t, peerPN, q.spaces[1-dir][quicSpaceApplication])
							require.Equal(t, beforeOrder, q.earlyOrder, "a refused packet cannot become admitted early/application order evidence")
							require.Contains(t, q.spaces[dir][quicSpaceApplication].rejected, uint64(11))
							require.Len(t, q.spaces[dir][quicSpaceApplication].seen, 2, "baseline10 plus refused11, each once")
							_, err := e.GetFields()
							require.Error(t, err)
							repeated := n09AuditDatagram(t, a, q, dir, phase, 11, plain)
							require.Equal(t, "malformed", repeated.Status)
							require.Len(t, q.spaces[dir][quicSpaceApplication].seen, 2)
							require.Empty(t, q.streams)
							good := n09AuditDatagram(t, a, q, dir, phase, 12, []byte{1})
							require.Equal(t, "decoded", good.Status, good.Error)
							require.Equal(t, phase, q.keyPhase[dir], "later valid packet can select the unpoisoned future epoch")
							peer := n09AuditDatagram(t, a, q, 1-dir, 0, 11, []byte{1})
							require.Equal(t, "decoded", peer.Status, peer.Error)
							require.Zero(t, q.keyPhase[1-dir])
						})
					}
				}
			}
		}
	}
}

func TestN09UniAndBidirectionalFrameRoleControls(t *testing.T) {
	for _, dir := range []int{0, 1} {
		for _, phase := range []int{0, 1} {
			for _, kind := range []string{"STREAM", "RESET_STREAM", "STOP_SENDING"} {
				for _, uni := range []bool{false, true} {
					t.Run(fmt.Sprintf("dir%d/phase%d/%s/uni%t", dir, phase, kind, uni), func(t *testing.T) {
						a, q := n09AuditOwner(t, quicVersion1, quicAEADAES128GCM)
						sid := uint64(0)
						if uni {
							sid = uint64(2 + dir)
							if kind == "STOP_SENDING" {
								sid = uint64(3 - dir)
							}
						}
						initiator := int(sid & 1)
						open := n09AuditDatagram(t, a, q, initiator, 0, 11, quicStreamFrame(sid, 0, false, nil))
						require.Equal(t, "decoded", open.Status, open.Error)
						plain := n09AuditFrame(kind, sid)
						e := n09AuditDatagram(t, a, q, dir, phase, 12, plain)
						require.Equal(t, "decoded", e.Status, e.Error)
						require.NotNil(t, q.streams[sid])
						require.Equal(t, phase, q.keyPhase[dir])
					})
				}
			}
		}
	}
}

func TestN09ForeignDomainCannotSupplyUniStreamRoleOwner(t *testing.T) {
	a, q := n09AuditOwner(t, quicVersion1, quicAEADAES128GCM)
	wire := n09ObservedCIDPacket(t, q.keys.app[0][0], []byte("server!!"), 11, 0, quicResetStream(3, 0, 0))
	original := bytes.Clone(wire)
	before := q.spaces
	events, handled := a.decodeQUICDatagram(&ProtocolEvent{Transport: "udp", Timestamp: time.Unix(3, 0), Source: "192.0.2.1:40000", Destination: "192.0.2.2:443", Domain: CaptureDomain{Interface: 9}}, wire)
	require.False(t, handled)
	require.Empty(t, events)
	require.Equal(t, before, q.spaces)
	require.Empty(t, q.streams)
	require.Equal(t, original, wire)
}
