package pcaputil

import (
	"bytes"
	"container/list"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type earlyRecoveryProvider struct {
	mu            sync.RWMutex
	ordinary      *TLSKeyLog
	random, early []byte
}

func (p *earlyRecoveryProvider) LookupTLSSecret(label string, random []byte) ([]byte, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if label == "CLIENT_EARLY_TRAFFIC_SECRET" {
		return bytes.Clone(p.early), bytes.Equal(random, p.random) && len(p.early) != 0
	}
	return p.ordinary.LookupTLSSecret(label, random)
}

// These are post-authentication CRYPTO algorithm fixtures. The integration
// tests below separately pass actual protected datagrams through native dispatch.
func earlyRecoveryFixture(t *testing.T, version uint32, suite uint16, offerEarly, lateSecret bool) (*binQUIC, *quicTrafficKeys, *quicTrafficKeys, func(bool)) {
	t.Helper()
	n, algorithm := 32, quicAEADAES128GCM
	if suite == 0x1302 {
		n, algorithm = 48, quicAEADAES256GCM
	}
	if suite == 0x1303 {
		algorithm = quicAEADChaCha20Poly1305
	}
	random := bytes.Repeat([]byte{0x31}, 32)
	var text string
	for i, label := range []string{"CLIENT_HANDSHAKE_TRAFFIC_SECRET", "SERVER_HANDSHAKE_TRAFFIC_SECRET", "CLIENT_TRAFFIC_SECRET_0", "SERVER_TRAFFIC_SECRET_0"} {
		text += fmt.Sprintf("%s %x %x\n", label, random, bytes.Repeat([]byte{byte(i + 1)}, n))
	}
	ordinary, err := ParseTLSKeyLog(text)
	require.NoError(t, err)
	p := &earlyRecoveryProvider{ordinary: ordinary, random: random}
	if !lateSecret {
		p.early = bytes.Repeat([]byte{5}, n)
	}
	initial, err := quicInitialKeysVersion([]byte("seedcid8"), version)
	require.NoError(t, err)
	q := &binQUIC{native: true, version: version, byteLimit: 65536, provider: p, keys: &quicKeyring{version: version, initial: initial}, cids: map[string]int{"seedcid8": 2}}
	share := append([]byte{0, 23, 0, 32}, bytes.Repeat([]byte{1}, 32)...)
	clientShare := append([]byte{0, byte(len(share))}, share...)
	offer := tlsResumptionPSKs([][]byte{[]byte("owned-test-ticket")}, []uint32{0}, []int{n})
	exts := []tlsResumptionExt{{43, []byte{2, 3, 4}}, {10, []byte{0, 2, 0, 23}}, {51, clientShare}, {45, []byte{1, 1}}}
	if offerEarly {
		exts = append(exts, tlsResumptionExt{42, nil})
	}
	exts = append(exts, tlsResumptionExt{41, offer})
	client := tlsResumptionHello(true, nil, suite, exts...)
	require.NoError(t, q.feedCrypto(0, quicSpaceInitial, 0, quicTLSLevelMessage(1, client), map[string]any{}, 128))
	early, err := quicDeriveTrafficVersion(bytes.Repeat([]byte{5}, n), algorithm, version)
	require.NoError(t, err)
	app, err := quicDeriveTrafficVersion(bytes.Repeat([]byte{3}, n), algorithm, version)
	require.NoError(t, err)
	finish := func(accepted bool) {
		server := tlsResumptionHello(false, nil, suite, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, share}, tlsResumptionExt{41, []byte{0, 0}})
		require.NoError(t, q.feedCrypto(1, quicSpaceInitial, 0, quicTLSLevelMessage(2, server), map[string]any{}, 128))
		body := []byte{0, 0}
		if accepted {
			body = []byte{0, 4, 0, 42, 0, 0}
		}
		require.NoError(t, q.feedCrypto(1, quicSpaceHandshake, 0, quicTLSLevelMessage(8, body), map[string]any{}, 128))
	}
	return q, early, app, finish
}

func earlyRecoveryStream(data []byte) []byte {
	w := append([]byte{0x0b, 0}, quicPutVarint(uint64(len(data)))...)
	return append(w, data...)
}

func earlyRecoveryParser(t *testing.T) *binParser {
	t.Helper()
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	t.Cleanup(c.binParser.closeUDPSessions)
	return c.binParser
}

func earlyRecoveryOwner(a *binParser, q *binQUIC, domain CaptureDomain) *binFlow {
	if a.udpSessions == nil {
		a.udpSessions = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
	}
	f := &binFlow{a: a, id: a.flows.Add(1), protocol: "quic", quic: q, endpoints: [2]string{"192.0.2.1:40000", "192.0.2.2:14443"}}
	key := binUDPKey{"quic/" + f.endpoints[0] + "/" + hex.EncodeToString([]byte("seedcid8")), f.endpoints[1], domain}
	el := a.udpSessions.lru.PushBack(&binUDPEntry{key: key, flow: f, touched: time.Unix(1, 0)})
	a.udpSessions.entries[key] = el
	return f
}

func earlyRecoveryBase(f *binFlow, domain CaptureDomain, number uint64) *ProtocolEvent {
	return &ProtocolEvent{Transport: "udp", Source: f.endpoints[0], Destination: f.endpoints[1], Timestamp: time.Unix(int64(number), 0), Domain: domain, SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: number, Domain: domain}}}}
}

func TestQUICEarlyProtectedPacketRecoversWithOwnedOriginalSource(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
			for _, late := range []bool{false, true} {
				t.Run(fmt.Sprintf("v%x/suite%x/late%v", version, suite, late), func(t *testing.T) {
					a := earlyRecoveryParser(t)
					q, key, app, finish := earlyRecoveryFixture(t, version, suite, true, late)
					domain := CaptureDomain{Interface: 7, Encapsulation: "/vlan:9"}
					f := earlyRecoveryOwner(a, q, domain)
					payload := []byte("owned-early-body")
					raw := quicConformancePacket(t, key, "0-RTT", 1, 0, earlyRecoveryStream(payload))
					base := earlyRecoveryBase(f, domain, 10)
					first, ok := a.decodeQUICDatagram(base, raw)
					require.True(t, ok)
					require.Len(t, first, 1)
					require.Equal(t, "context-required", first[0].Status)
					require.Nil(t, q.streams)
					originalID := first[0].ID
					first[0].Raw[0] ^= 1
					first[0].SourceBytes.PacketRefs[0].Number = 999
					first[0].Session["Early Data Decision"] = "accepted"
					base.SourceBytes.PacketRefs[0].Number = 888
					finish(true)
					if late {
						p := q.provider.(*earlyRecoveryProvider)
						p.mu.Lock()
						p.early = bytes.Clone(key.secret)
						p.mu.Unlock()
						require.Nil(t, q.keys.early[0], "complete ordinary keys did not previously include this later provision")
					}
					trigger := epochQUICPacket(t, app, []byte("seedcid8"), 3, 0, 4)
					events, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, domain, 11), trigger)
					require.True(t, ok)
					require.Len(t, events, 2, "new context must publish exactly one owned recovery, alongside its triggering packet")
					var recovered *ProtocolEvent
					for _, e := range events {
						a.emit(e)
						if bytes.Equal(e.Raw, raw) {
							recovered = e
						}
					}
					require.NotNil(t, recovered)
					require.Equal(t, "decoded", recovered.Status)
					require.Equal(t, f.id, recovered.FlowID)
					require.NotEqual(t, originalID, recovered.ID)
					require.Equal(t, []PacketReference{{Number: 10, Domain: domain}}, recovered.SourceBytes.PacketRefs)
					require.NotNil(t, recovered.SourceBytes.CompletedIn)
					require.Equal(t, uint64(10), recovered.SourceBytes.CompletedIn.Number, "later context is not a byte contributor")
					require.Equal(t, originalID, recovered.Session["Reprocessed From PDU"])
					require.Equal(t, PacketReference{Number: 11, Domain: domain}, recovered.Session["Reprocessed In Packet"])
					require.Equal(t, payload, q.streams[0].reassembly[0].data)
					again, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, domain, 12), epochQUICPacket(t, app, []byte("seedcid8"), 4, 0, 4))
					require.True(t, ok)
					require.Len(t, again, 1)
					require.Equal(t, payload, q.streams[0].reassembly[0].data)
				})
			}
		}
	}
}

func TestQUICEarlyRejectedServerDoesNotDeliverBufferedStreams(t *testing.T) {
	a := earlyRecoveryParser(t)
	q, key, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
	f := earlyRecoveryOwner(a, q, CaptureDomain{})
	raw := quicConformancePacket(t, key, "0-RTT", 1, 0, earlyRecoveryStream([]byte("not-admitted")))
	first, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 1), raw)
	require.True(t, ok)
	require.Len(t, first, 1)
	finish(false)
	events, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 2), epochQUICPacket(t, app, []byte("seedcid8"), 3, 0, 4))
	require.True(t, ok)
	require.Len(t, events, 2)
	require.Nil(t, q.streams)
	for _, e := range events {
		if bytes.Equal(e.Raw, raw) {
			require.Equal(t, "context-required", e.Status)
			require.Contains(t, e.Error, "rejected")
			require.NotEqual(t, true, e.Session["Authentication Verified"])
			require.NotContains(t, e.Session, "Frames")
			require.Equal(t, "quic", e.Protocol)
		}
	}
	again, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 3), epochQUICPacket(t, app, []byte("seedcid8"), 4, 0, 4))
	require.True(t, ok)
	require.Len(t, again, 1)
}

func TestQUICEarlyStreamsAwaitKnownAcceptanceEvenWithKeys(t *testing.T) {
	q, key, _, _ := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
	// Admit just the server hello, which loads keys without making an EE decision.
	share := append([]byte{0, 23, 0, 32}, bytes.Repeat([]byte{1}, 32)...)
	server := tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, share}, tlsResumptionExt{41, []byte{0, 0}})
	require.NoError(t, q.feedCrypto(1, quicSpaceInitial, 0, quicTLSLevelMessage(2, server), map[string]any{}, 128))
	require.NotNil(t, q.keys.early[0])
	info, err := q.consume(0, quicConformancePacket(t, key, "0-RTT", 1, 0, earlyRecoveryStream([]byte("wait"))), 128)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrContextRequired, typed.Kind)
	require.Equal(t, true, info["Authentication Verified"])
	require.NotContains(t, info, "Frames")
	require.Nil(t, q.streams)
	require.False(t, q.spaces[0][quicSpaceApplication].init)
}

func TestQUICEarlyPendingCannotBorrowAnotherCaptureDomain(t *testing.T) {
	a := earlyRecoveryParser(t)
	qA, key, app, finishA := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
	qB, _, _, finishB := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
	dA := CaptureDomain{Interface: 3, Encapsulation: "/vlan:20"}
	dB := CaptureDomain{Interface: 4, Encapsulation: "/vlan:21"}
	fA, fB := earlyRecoveryOwner(a, qA, dA), earlyRecoveryOwner(a, qB, dB)
	raw := quicConformancePacket(t, key, "0-RTT", 1, 0, earlyRecoveryStream([]byte("domain-owned")))
	first, ok := a.decodeQUICDatagram(earlyRecoveryBase(fA, dA, 1), raw)
	require.True(t, ok)
	require.Len(t, first, 1)
	finishB(true)
	foreign, ok := a.decodeQUICDatagram(earlyRecoveryBase(fB, dB, 2), epochQUICPacket(t, app, []byte("seedcid8"), 3, 0, 4))
	require.True(t, ok)
	require.Len(t, foreign, 1)
	require.Len(t, qA.earlyPending, 1)
	require.Nil(t, qA.streams)
	require.Nil(t, qB.streams)
	finishA(true)
	local, ok := a.decodeQUICDatagram(earlyRecoveryBase(fA, dA, 3), epochQUICPacket(t, app, []byte("seedcid8"), 3, 0, 4))
	require.True(t, ok)
	require.Len(t, local, 2)
	require.Equal(t, fA.id, local[1].FlowID)
	require.Equal(t, []PacketReference{{Number: 1, Domain: dA}}, local[1].SourceBytes.PacketRefs)
	require.Equal(t, []byte("domain-owned"), qA.streams[0].reassembly[0].data)
	require.Nil(t, qB.streams)
}

func TestQUICEarlyPendingBudgetsAndLifecycle(t *testing.T) {
	for _, mode := range []string{"count", "hard-count", "bytes", "global", "provenance", "retire", "close", "eof"} {
		t.Run(mode, func(t *testing.T) {
			a := earlyRecoveryParser(t)
			q, key, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
			f := earlyRecoveryOwner(a, q, CaptureDomain{})
			raw := quicConformancePacket(t, key, "0-RTT", 1, 0, earlyRecoveryStream([]byte("bounded")))
			base := earlyRecoveryBase(f, CaptureDomain{}, 1)
			if mode == "count" {
				a.budget.MaxCollectionElements = 1
			}
			if mode == "hard-count" {
				a.budget.MaxCollectionElements = 10000
				q.byteLimit = 1 << 20
			}
			if mode == "global" {
				a.config.MaxBufferedBytes = 1
			}
			if mode == "bytes" {
				q.byteLimit = 1024
			}
			if mode == "provenance" {
				base.SourceBytes.PacketRefs = make([]PacketReference, 2048)
			}
			first, ok := a.decodeQUICDatagram(base, raw)
			require.True(t, ok)
			require.Len(t, first, 1)
			if mode == "bytes" || mode == "global" || mode == "provenance" {
				require.NotNil(t, first[0].sessionError)
				require.Equal(t, ErrResourceExceeded, first[0].sessionError.Kind)
				require.True(t, q.retired)
				require.Empty(t, q.earlyPending)
				return
			}
			require.Len(t, q.earlyPending, 1)
			require.Greater(t, q.earlyPendingBytes, int64(len(raw)))
			switch mode {
			case "hard-count":
				for i := uint64(2); i <= 65; i++ {
					more, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, i), raw)
					require.True(t, ok)
					require.Len(t, more, 1)
					if i <= 64 {
						require.Equal(t, "context-required", more[0].Status)
						require.Len(t, q.earlyPending, int(i))
					} else {
						require.Equal(t, ErrResourceExceeded, more[0].sessionError.Kind)
						require.True(t, q.retired)
						require.Empty(t, q.earlyPending)
					}
				}
			case "count":
				more, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 2), raw)
				require.True(t, ok)
				require.Len(t, more, 1)
				require.Equal(t, ErrResourceExceeded, more[0].sessionError.Kind)
				require.True(t, q.retired)
				require.Empty(t, q.earlyPending)
			case "retire":
				f.retireQUICUDP()
				require.True(t, q.retired)
				require.Empty(t, q.earlyPending)
				require.Zero(t, q.earlyPendingBytes)
			case "close":
				finish(true)
				// This helper's CID-less short packets cannot address seedcid8.
				// Seal the close with the actual owned destination CID.
				header := binary.BigEndian.AppendUint32(append([]byte{0x43}, []byte("seedcid8")...), 3)
				block, err := aes.NewCipher(app.key)
				require.NoError(t, err)
				packetCipher, err := cipher.NewGCM(block)
				require.NoError(t, err)
				closePacket := append(bytes.Clone(header), packetCipher.Seal(nil, quicNonce(app.iv, 3), []byte{0x1c, 0, 0, 0}, header)...)
				mask, err := quicHeaderMask(app, closePacket[len(header):len(header)+16])
				require.NoError(t, err)
				closePacket[0] ^= mask[0] & 0x1f
				for i := 0; i < 4; i++ {
					closePacket[9+i] ^= mask[1+i]
				}
				closed, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 2), closePacket)
				require.True(t, ok)
				require.Len(t, closed, 1)
				require.True(t, q.closed)
				require.Empty(t, q.earlyPending)
				require.Nil(t, q.streams)
			case "eof":
				a.closeUDPSessions()
				require.Nil(t, f.quic)
				require.Zero(t, a.buffered.Load())
			}
		})
	}
}

func TestQUICEarlyBadBufferedTagDoesNotPoisonValidPacket(t *testing.T) {
	a := earlyRecoveryParser(t)
	q, key, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
	f := earlyRecoveryOwner(a, q, CaptureDomain{})
	valid := quicConformancePacket(t, key, "0-RTT", 1, 0, earlyRecoveryStream([]byte("valid-after-bad-tag")))
	bad := bytes.Clone(valid)
	bad[len(bad)-1] ^= 1
	first, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 1), bad)
	require.True(t, ok)
	require.Len(t, first, 1)
	finish(true)
	trigger, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 2), epochQUICPacket(t, app, []byte("seedcid8"), 3, 0, 4))
	require.True(t, ok)
	require.Len(t, trigger, 2)
	require.Equal(t, ErrAuthenticationFailed, trigger[1].sessionError.Kind)
	require.NotEqual(t, true, trigger[1].Session["Authentication Verified"])
	require.Nil(t, q.streams)
	require.Empty(t, q.earlyPending)
	good, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 3), valid)
	require.True(t, ok)
	require.Len(t, good, 1)
	require.Equal(t, "decoded", good[0].Status)
	require.Equal(t, []byte("valid-after-bad-tag"), q.streams[0].reassembly[0].data)
}

func TestQUICEarlyRecoveredCloseDoesNotReplayFollowingStreams(t *testing.T) {
	a := earlyRecoveryParser(t)
	q, key, app, finish := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
	f := earlyRecoveryOwner(a, q, CaptureDomain{})
	for i, body := range [][]byte{{0x1c, 0, 0, 0}, earlyRecoveryStream([]byte("after-close"))} {
		first, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, uint64(i+1)), quicConformancePacket(t, key, "0-RTT", uint64(i+1), 0, body))
		require.True(t, ok)
		require.Len(t, first, 1)
		require.Len(t, q.earlyPending, i+1)
	}
	finish(true)
	events, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 3), epochQUICPacket(t, app, []byte("seedcid8"), 5, 0, 4))
	require.True(t, ok)
	require.Len(t, events, 2, "the close is observed; the following pending application bytes are discarded")
	require.Equal(t, "decoded", events[1].Status)
	require.True(t, q.closed)
	require.Empty(t, q.earlyPending)
	require.Nil(t, q.streams)
}

// These AES-128 fixtures include the actual observed receiver CID in both
// coalesced packet headers; the CID-less conformance helper cannot route them.
func earlyRecoveryCIDPacket(t *testing.T, key *quicTrafficKeys, handshake bool, pn uint32, cid, plain []byte) []byte {
	t.Helper()
	header := append([]byte{0x43}, cid...)
	if handshake {
		header = binary.BigEndian.AppendUint32([]byte{0xe3}, quicVersion1)
		header = append(header, byte(len(cid)))
		header = append(header, cid...)
		header = append(header, 0)
		header = append(header, quicPutVarint(uint64(4+len(plain)+16))...)
	}
	pnAt := len(header)
	header = binary.BigEndian.AppendUint32(header, pn)
	block, err := aes.NewCipher(key.key)
	require.NoError(t, err)
	packetCipher, err := cipher.NewGCM(block)
	require.NoError(t, err)
	wire := append(bytes.Clone(header), packetCipher.Seal(nil, quicNonce(key.iv, uint64(pn)), plain, header)...)
	mask, err := quicHeaderMask(key, wire[pnAt+4:pnAt+20])
	require.NoError(t, err)
	bits := byte(0x1f)
	if handshake {
		bits = 0x0f
	}
	wire[0] ^= mask[0] & bits
	for i := 0; i < 4; i++ {
		wire[pnAt+i] ^= mask[1+i]
	}
	return wire
}

func TestQUICEarlyRecoveryPrecedesCoalescedResponseWithoutStealingWireBudget(t *testing.T) {
	for _, mode := range []string{"associate", "wire-budget", "recovery-resource"} {
		t.Run(mode, func(t *testing.T) {
			a := earlyRecoveryParser(t)
			a.budget.MaxCollectionElements = 3
			if mode == "wire-budget" {
				a.budget.MaxCollectionElements = 2
			}
			q, early, app, _ := earlyRecoveryFixture(t, quicVersion1, 0x1301, true, false)
			f := earlyRecoveryOwner(a, q, CaptureDomain{})
			request := earlyRecoveryStream([]byte("request"))
			if mode == "recovery-resource" {
				request = append([]byte{0x0f, 0}, quicPutVarint(1<<20)...)
				request = append(request, 1, 'x')
			}
			first, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 1), quicConformancePacket(t, early, "0-RTT", 1, 0, request))
			require.True(t, ok)
			require.Len(t, first, 1)
			share := append([]byte{0, 23, 0, 32}, bytes.Repeat([]byte{1}, 32)...)
			server := tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, share}, tlsResumptionExt{41, []byte{0, 0}})
			require.NoError(t, q.feedCrypto(1, quicSpaceInitial, 0, quicTLSLevelMessage(2, server), map[string]any{}, 128))
			cid := []byte("clientid")
			q.cids[string(cid)] = 1
			ee := quicTLSLevelMessage(8, []byte{0, 4, 0, 42, 0, 0})
			crypto := append([]byte{6, 0}, quicPutVarint(uint64(len(ee)))...)
			crypto = append(crypto, ee...)
			hs := earlyRecoveryCIDPacket(t, q.keys.handshake[1], true, 1, cid, crypto)
			response := earlyRecoveryCIDPacket(t, q.keys.app[1][0], false, 2, cid, earlyRecoveryStream([]byte("response")))
			base := earlyRecoveryBase(f, CaptureDomain{}, 2)
			base.Source, base.Destination = base.Destination, base.Source
			events, ok := a.decodeQUICDatagram(base, append(hs, response...))
			require.True(t, ok)
			if mode == "wire-budget" {
				require.Len(t, events, 2)
				require.Equal(t, response, events[1].Raw, "both original wire packets retain their budget")
				require.Len(t, q.earlyPending, 1)
				later, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, CaptureDomain{}, 3), epochQUICPacket(t, app, []byte("seedcid8"), 5, 0, 4))
				require.True(t, ok)
				require.Len(t, later, 2)
				require.Empty(t, q.earlyPending)
				return
			}
			require.Len(t, events, 3)
			require.Equal(t, response, events[2].Raw)
			if mode == "recovery-resource" {
				require.True(t, q.retired)
				require.Nil(t, q.keys)
				require.Nil(t, q.streams)
				require.Equal(t, ErrResourceExceeded, events[1].sessionError.Kind)
				require.Equal(t, ErrResourceExceeded, events[2].sessionError.Kind)
				require.NotEqual(t, true, events[2].Session["Authentication Verified"])
			} else {
				require.Equal(t, "decoded", events[1].Status)
				require.Equal(t, "decoded", events[2].Status)
				require.NotZero(t, events[1].ID)
				require.Equal(t, events[1].ID, events[2].ResponseTo)
				require.Equal(t, events[1].ID, events[2].TransactionID)
			}
		})
	}
}

func TestTLSEarlyAcceptanceExtensionMustBeEmpty(t *testing.T) {
	for n := 0; n <= 5; n++ {
		t.Run(fmt.Sprintf("bytes%d", n), func(t *testing.T) {
			body := append([]byte{0, byte(4 + n), 0, 42, 0, byte(n)}, bytes.Repeat([]byte{1}, n)...)
			encoded := quicTLSLevelMessage(8, body)
			_, decodeErr := DecodeTLSEncryptedExtensions(encoded, 0x304, 1)
			tls := &binTLS{version: 0x304, client: 0}
			admissionErr := tls.admitEncryptedExtensions(1, body, 1)
			if n == 0 {
				require.NoError(t, decodeErr)
				require.NoError(t, admissionErr)
			} else {
				for _, err := range []error{decodeErr, admissionErr} {
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
				}
			}
		})
	}
}

func TestQUICEarlyDecisionQualifiedAndTransactional(t *testing.T) {
	for _, mode := range []string{"no-offer", "no-selected-PSK", "TLS-HRR", "duplicate-in-batch"} {
		t.Run(mode, func(t *testing.T) {
			q, _, _, _ := earlyRecoveryFixture(t, quicVersion1, 0x1301, mode != "no-offer", false)
			share := append([]byte{0, 23, 0, 32}, bytes.Repeat([]byte{1}, 32)...)
			extensions := []tlsResumptionExt{{43, []byte{3, 4}}, {51, share}}
			if mode != "no-selected-PSK" {
				extensions = append(extensions, tlsResumptionExt{41, []byte{0, 0}})
			}
			if mode == "TLS-HRR" {
				extensions = []tlsResumptionExt{{43, []byte{3, 4}}, {44, []byte{0, 1, 9}}}
			}
			server := tlsResumptionHello(false, nil, 0x1301, extensions...)
			if mode == "TLS-HRR" {
				copy(server[2:34], tlsRetryRandom[:])
			}
			require.NoError(t, q.feedCrypto(1, quicSpaceInitial, 0, quicTLSLevelMessage(2, server), map[string]any{}, 128))
			if mode == "TLS-HRR" {
				require.True(t, q.earlyRejected())
				require.False(t, q.earlyAccepted())
				return
			}
			encoded := quicTLSLevelMessage(8, []byte{0, 4, 0, 42, 0, 0})
			if mode == "duplicate-in-batch" {
				encoded = append(encoded, encoded...)
			}
			err := q.feedCrypto(1, quicSpaceHandshake, 0, encoded, map[string]any{}, 128)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.False(t, q.earlyContext.decisionKnown)
			require.False(t, q.earlyAccepted())
			require.Zero(t, q.cryptoRead[1][quicSpaceHandshake])
		})
	}
}

func FuzzQUICEarlyRecoveryOwnership(f *testing.F) {
	for _, seed := range [][]byte{{0, 0, 0}, {1, 1, 1}, {0, 2, 2}, {1, 0, 4, 9}, {0, 1, 8, 3}, {1, 2, 15, 1, 2}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 || len(data) > 259 {
			return
		}
		version := []uint32{quicVersion1, quicVersion2}[data[0]&1]
		suite := []uint16{0x1301, 0x1302, 0x1303}[int(data[1])%3]
		accepted, late, badTag := data[2]&1 == 0, data[2]&2 != 0, data[2]&4 != 0
		a := earlyRecoveryParser(t)
		q, key, app, finish := earlyRecoveryFixture(t, version, suite, true, late)
		domain := CaptureDomain{Interface: 1, Encapsulation: "/vlan:17"}
		owner := earlyRecoveryOwner(a, q, domain)
		payload := bytes.Clone(data[3:])
		raw := quicConformancePacket(t, key, "0-RTT", 1, 0, earlyRecoveryStream(payload))
		if badTag {
			raw[len(raw)-1] ^= 1
		}
		first, ok := a.decodeQUICDatagram(earlyRecoveryBase(owner, domain, 1), raw)
		require.True(t, ok)
		require.Len(t, first, 1)
		first[0].Raw[0] ^= 1
		first[0].SourceBytes.PacketRefs[0].Domain.Interface = 999
		finish(accepted)
		if late {
			provider := q.provider.(*earlyRecoveryProvider)
			provider.mu.Lock()
			provider.early = bytes.Clone(key.secret)
			provider.mu.Unlock()
		}
		trigger := epochQUICPacket(t, app, []byte("seedcid8"), 3, 0, 4)
		if data[2]&8 != 0 {
			foreign := domain
			foreign.Interface++
			events, matched := a.decodeQUICDatagram(earlyRecoveryBase(owner, foreign, 2), trigger)
			require.False(t, matched)
			require.Empty(t, events)
			require.Len(t, q.earlyPending, 1)
		}
		events, ok := a.decodeQUICDatagram(earlyRecoveryBase(owner, domain, 3), trigger)
		require.True(t, ok)
		require.Len(t, events, 2)
		require.Equal(t, raw, events[1].Raw)
		require.Equal(t, []PacketReference{{Number: 1, Domain: domain}}, events[1].SourceBytes.PacketRefs)
		require.Empty(t, q.earlyPending)
		if accepted && !badTag {
			require.Equal(t, "decoded", events[1].Status)
			require.True(t, bytes.Equal(payload, q.streams[0].reassembly[0].data), "delivered stream bytes must match, including an empty FIN")
		} else {
			require.NotEqual(t, "decoded", events[1].Status)
			require.Nil(t, q.streams)
		}
		a.closeUDPSessions()
		require.Zero(t, a.buffered.Load())
	})
}
