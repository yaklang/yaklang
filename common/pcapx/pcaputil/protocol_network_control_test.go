package pcaputil

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
	"testing"
	"time"
)

func TestControlIngressCatalogRulesAndWireFields(t *testing.T) {
	for _, profile := range controlDatagramProfiles {
		found := false
		for _, p := range binparser.ProtocolCatalog {
			if (p.RuleFile == "network_control_fields.yaml" || p.RuleFile == "routing_control.yaml") && p.EntryNode == profile.entry {
				require.NotEmpty(t, p.ProfileOf, profile.name)
				require.Equal(t, "partial", p.Status)
				found = true
			}
		}
		require.True(t, found, "native profile %s must resolve to a catalog entry", profile.name)
	}
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer a.closeUDPSessions()
	wg := make([]byte, 148)
	wg[0] = 1
	wg[4] = 7
	rip := []byte{1, 2, 0, 0}
	rip = append(rip, make([]byte, 20)...)
	rip[len(rip)-1] = 16
	ike := make([]byte, 28)
	ike[7] = 1
	ike[17] = 32
	ike[18] = 34
	ike[19] = 8
	binary.BigEndian.PutUint32(ike[24:], 28)
	rpc := make([]byte, 40)
	rpc[3] = 7
	rpc[11] = 2
	binary.BigEndian.PutUint32(rpc[12:], 100000)
	rpc[19] = 2
	for _, tc := range []struct {
		name, key string
		port      uint16
		wire      []byte
	}{{"wireguard", "Sender Index", 51820, wg}, {"rip", "Command", 520, rip}, {"ike", "Exchange Type", 500, ike}, {"onc-rpc", "XID", 111, rpc}, {"openvpn", "Packet ID", 1194, []byte{56, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 0, 0, 0}}, {"ipmi", "RMCP Class", 623, []byte{6, 0, 255, 6, 0, 0, 17, 190, 128, 0, 0, 0}}} {
		t.Run(tc.name, func(t *testing.T) {
			e := &ProtocolEvent{Transport: "udp", Source: "127.0.0.1:53000", Destination: "127.0.0.1:51820", Timestamp: time.Unix(1, 0)}
			require.True(t, a.decodeControlDatagram(e, tc.wire, 53000, tc.port, ""))
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Equal(t, tc.name, e.Protocol)
			require.Contains(t, e.Session, tc.key)
			catalog := false
			for _, p := range binparser.ProtocolCatalog {
				if p.RuleFile == "network_control_fields.yaml" && p.EntryNode == e.Entry {
					catalog = true
				}
			}
			require.True(t, catalog, "native entry must be in public catalog")
			fields, err := e.GetFields()
			require.NoError(t, err)
			require.Contains(t, fields, tc.key)
			ranges := fields["Wire Field Ranges"].(map[string]any)
			ranges[tc.key].([]any)[0] = uint64(65500)
			require.NotEqual(t, uint64(65500), e.Session["Wire Field Ranges"].(map[string]any)[tc.key].([]any)[0], "field result cannot edit owned wire spans")
			bad := &ProtocolEvent{Transport: "udp", Source: e.Source, Destination: e.Destination, Timestamp: time.Unix(2, 0)}
			require.False(t, a.decodeControlDatagram(bad, []byte("unrelated garbage"), 53000, tc.port, ""))
			require.Zero(t, bad.FlowID)
		})
	}
}

func TestNBSSControlsEverySplitHaveSemanticFields(t *testing.T) {
	name := append([]byte{32}, bytes.Repeat([]byte{'E', 'B'}, 16)...)
	name = append(name, 0)
	request := append([]byte{0x81, 0, 0, 68}, name...)
	request = append(request, name...)
	for split := 1; split <= 8; split++ {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
		require.NoError(t, err)
		var es []*ProtocolEvent
		for _, step := range []sessionStep{{0, request}, {1, []byte{0x82, 0, 0, 0, 0x85, 0, 0, 0}}} {
			w := step.wire
			for len(w) > 0 {
				n := min(split, len(w))
				r := s.Feed(step.dir, time.Unix(1, 0), w[:n])
				if r.Err != nil {
					require.Equal(t, ErrNeedMore, r.Err.Kind, "split %d: %s state %s", split, r.Err.Message, r.State)
				}
				es = append(es, r.Events...)
				w = w[n:]
			}
		}
		require.Len(t, es, 3)
		for _, e := range es {
			require.Equal(t, "nbss", e.Protocol)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.NotEmpty(t, e.Session["Packet Name"])
		}
		s.Close("test")
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestSSHInitialTailAndGuessedExchangeValidation(t *testing.T) {
	s := &binSSH{client: 0, clientKnown: true}
	_, err := s.consume(0, sshIdent("SSH-2.0-AuditClient"))
	require.NoError(t, err)
	_, err = s.consume(1, sshIdent("SSH-2.0-AuditServer"))
	require.NoError(t, err)
	w := sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
	payload := w[5 : len(w)-int(w[4])]
	_, err = s.consume(0, sshPkt(payload[:len(payload)-3]))
	require.Error(t, err)
	require.False(t, s.haveKEX[0])
	guessed := sshKex("diffie-hellman-group14-sha256,curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
	guessedPayload := bytes.Clone(guessed[5 : len(guessed)-int(guessed[4])])
	guessedPayload[len(guessedPayload)-5] = 1
	_, err = s.consume(0, sshPkt(guessedPayload))
	require.NoError(t, err)
	_, err = s.consume(1, w)
	require.NoError(t, err)
	info, err := s.consume(0, sshPkt([]byte{30, 1, 2, 3, 4}))
	require.NoError(t, err)
	require.Equal(t, "Ignored guessed key-exchange packet", info["Packet Name"])
	_, err = s.consume(0, sshValidCurveInit())
	require.NoError(t, err)
	_, err = s.consume(0, sshPkt([]byte{21, 0}))
	require.Error(t, err)
	require.False(t, s.newkeys[0])
	require.Equal(t, "curve25519-sha256", sshNegotiate(sshKexLists{Kex: "ext-info-c,curve25519-sha256"}, sshKexLists{Kex: "ext-info-c,curve25519-sha256"})["Kex Algorithm"])
}

func TestWireGuardReceiverIndexDomainAndExpiry(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer a.closeUDPSessions()
	base := func(reverse bool, at int64, domain int) *ProtocolEvent {
		e := &ProtocolEvent{Transport: "udp", Source: "127.0.0.1:53000", Destination: "127.0.0.1:51820", Timestamp: time.Unix(at, 0), Domain: CaptureDomain{Interface: domain}}
		if reverse {
			e.Source, e.Destination = e.Destination, e.Source
		}
		return e
	}
	init := make([]byte, 148)
	init[0] = 1
	init[4] = 3
	response := make([]byte, 92)
	response[0] = 2
	response[4] = 7
	response[8] = 3
	data := make([]byte, 32)
	data[0] = 4
	data[4] = 7
	e := base(false, 1, 0)
	require.False(t, a.decodeControlDatagram(e, data, 53000, 51820, ""))
	e = base(false, 2, 0)
	require.True(t, a.decodeControlDatagram(e, init, 53000, 51820, ""))
	id := e.FlowID
	r := base(true, 3, 0)
	require.True(t, a.decodeControlDatagram(r, response, 51820, 53000, ""))
	require.Equal(t, id, r.FlowID)
	e = base(false, 4, 0)
	require.True(t, a.decodeControlDatagram(e, data, 53000, 51820, ""))
	require.Equal(t, id, e.FlowID)
	e = base(false, 5, 1)
	require.False(t, a.decodeControlDatagram(e, data, 53000, 51820, ""))
	data[4] = 8
	e = base(false, 6, 0)
	require.False(t, a.decodeControlDatagram(e, data, 53000, 51820, ""))
	data[4] = 7
	e = base(false, 130, 0)
	require.False(t, a.decodeControlDatagram(e, data, 53000, 51820, ""))
	require.Empty(t, a.udpSessions.entries)
}

func TestOpenVPNHardResetReuseCannotInheritServerEvidence(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	defer a.closeUDPSessions()
	decode := func(w []byte, reverse bool) (*ProtocolEvent, bool) {
		e := &ProtocolEvent{Transport: "udp", Source: "127.0.0.1:53000", Destination: "127.0.0.1:1194", Timestamp: time.Unix(1, 0)}
		src, dst := uint16(53000), uint16(1194)
		if reverse {
			e.Source, e.Destination, src, dst = e.Destination, e.Source, dst, src
		}
		return e, a.decodeControlDatagram(e, w, src, dst, "")
	}
	client := []byte{56, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 0, 0, 0}
	server := bytes.Clone(client)
	server[0], server[1] = 64, 9
	data := []byte{72, 0, 0, 1, 1, 2, 3, 4}
	first, ok := decode(client, false)
	require.True(t, ok)
	_, ok = decode(server, true)
	require.True(t, ok)
	_, ok = decode(data, false)
	require.True(t, ok)
	_, ok = decode(client, false) // retransmission preserves the exchange
	require.True(t, ok)
	_, ok = decode(data, false)
	require.True(t, ok)
	client[1] = 10
	second, ok := decode(client, false)
	require.True(t, ok)
	require.NotEqual(t, first.FlowID, second.FlowID)
	_, ok = decode(data, false)
	require.False(t, ok)
	_, ok = decode(server, true)
	require.True(t, ok)
	_, ok = decode(data, false)
	require.True(t, ok)
	// Acknowledgment naming a session not observed on this tuple is rejected.
	ack := append([]byte{40}, server[1:9]...)
	ack = append(ack, 1, 0, 0, 0, 0)
	ack = append(ack, bytes.Repeat([]byte{99}, 8)...)
	_, ok = decode(ack, true)
	require.False(t, ok)
}

func TestTelnetFramingEverySplitAndEscapedSubnegotiation(t *testing.T) {
	for split := 1; split < 12; split++ {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
		require.NoError(t, err)
		wire := []byte{255, 253, 24, 255, 251, 31, 255, 250, 24, 0, 'A', 255, 255, 'B', 255, 240, 'o', 'k', 255, 255}
		var events []*ProtocolEvent
		for len(wire) > 0 {
			n := min(split, len(wire))
			r := s.Feed(0, time.Unix(1, 0), wire[:n])
			if r.Err != nil {
				require.Equal(t, ErrNeedMore, r.Err.Kind)
			}
			events = append(events, r.Events...)
			wire = wire[n:]
		}
		require.NotEmpty(t, events)
		for _, e := range events {
			require.Equal(t, "telnet", e.Protocol)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.NotEmpty(t, e.Session["Packet Name"], "the native dispatcher must consume the semantic codec")
		}
		s.Close("test")
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestTelnetSubnegotiationAdmissionAndBounds(t *testing.T) {
	// Actual telnetlib3 prefix: DO TERMINAL-TYPE, SB TERMINAL-TYPE SEND,
	// WILL SGA. The SB prevents adjacent-three-byte signature matching.
	w := []byte{255, 253, 24, 255, 250, 24, 1, 255, 240, 255, 251, 3}
	require.True(t, telnetInitialEvidence(w))
	require.True(t, telnetMatchingNegotiation(w, []byte{255, 253, 31, 255, 251, 24}))
	for split := 1; split <= len(w); split++ {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
		require.NoError(t, err)
		var es []*ProtocolEvent
		for at := 0; at < len(w); at += split {
			r := s.Feed(0, time.Unix(1, 0), w[at:min(at+split, len(w))])
			es = append(es, r.Events...)
		}
		require.Len(t, es, 3)
		for _, e := range es {
			require.Equal(t, "telnet", e.Protocol)
			require.Equal(t, "decoded", e.Status, e.Error)
		}
		s.Close("test")
	}
	long := append([]byte{255, 253, 24, 255, 250, 24}, bytes.Repeat([]byte{'x'}, 100)...)
	require.True(t, needsMoreTelnetPrefix(long))
	require.False(t, telnetInitialEvidence(long))
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	require.Equal(t, ProbeNeedMore, s.Probe(long).Verdict)
	require.Equal(t, "telnet", s.Probe(long).Protocol)
	s.Close("test")
	require.True(t, telnetInitialEvidence(append(long, 255, 240)))
	for _, bad := range [][]byte{
		append([]byte("data"), w...),
		{255, 253, 24, 'a', 'b', 'c'},
		{255, 253, 24, 255, 250, 31, 1, 255, 240}, // unrelated option
		{255, 253, 24, 255, 250, 24, 1, 255, 251}, // invalid SB escape
		append([]byte{255, 253, 24, 255, 250, 24}, bytes.Repeat([]byte{'x'}, 1024)...),
	} {
		require.False(t, telnetInitialEvidence(bad))
		require.False(t, needsMoreTelnetPrefix(bad))
	}
}

func TestNBSSTransitionToSMB2KeepsCarrier(t *testing.T) {
	name := append([]byte{32}, bytes.Repeat([]byte{'E', 'B'}, 16)...)
	name = append(name, 0)
	request := append([]byte{0x81, 0, 0, 68}, name...)
	request = append(request, name...)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	defer s.Close("test")
	for _, step := range []sessionStep{{0, request}, {1, []byte{0x82, 0, 0, 0}}, {0, smb2NegotiateReq(0x0202)}, {1, smb2NegotiateResp(0x0202)}, {1, []byte{0x85, 0, 0, 0}}} {
		r := s.Feed(step.dir, time.Unix(1, 0), step.wire)
		require.NotEmpty(t, r.Events)
		for _, e := range r.Events {
			require.Equal(t, "decoded", e.Status, e.Error)
			if e.Protocol == "smb2" {
				require.Equal(t, "NetBIOS Session Service", e.Session["Carrier"])
			}
		}
	}
}
