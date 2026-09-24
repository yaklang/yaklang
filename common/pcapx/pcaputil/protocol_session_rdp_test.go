package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func rdpTPKT(tpdu []byte) []byte {
	n := 4 + len(tpdu)
	return append([]byte{3, 0, byte(n >> 8), byte(n)}, tpdu...)
}

func rdpCR(user string, protocols uint32) []byte {
	var v []byte
	if user != "" {
		v = []byte("Cookie: mstshash=" + user + "\r\n")
	}
	neg := make([]byte, 8)
	neg[0] = rdpNegReq
	binary.LittleEndian.PutUint16(neg[2:], 8)
	binary.LittleEndian.PutUint32(neg[4:], protocols)
	v = append(v, neg...)
	hdr := append([]byte{0, rdpTPDUCR, 0, 0, 0, 0, 0}, v...)
	hdr[0] = byte(len(hdr) - 1)
	return rdpTPKT(hdr)
}

func rdpCC(negType byte, value uint32) []byte {
	neg := make([]byte, 8)
	neg[0] = negType
	binary.LittleEndian.PutUint16(neg[2:], 8)
	binary.LittleEndian.PutUint32(neg[4:], value)
	hdr := append([]byte{0, rdpTPDUCC, 0, 0, 0, 0, 0}, neg...)
	hdr[0] = byte(len(hdr) - 1)
	return rdpTPKT(hdr)
}

func rdpDT(payload []byte) []byte {
	return rdpTPKT(append([]byte{2, rdpTPDUData, 0x80}, payload...))
}

func rdpUD(typ uint16, payload []byte) []byte {
	n := 4 + len(payload)
	h := make([]byte, 4)
	binary.LittleEndian.PutUint16(h[0:], typ)
	binary.LittleEndian.PutUint16(h[2:], uint16(n))
	return append(h, payload...)
}

func rdpU32LE(v uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return b[:]
}

func rdpU16LE(v uint16) []byte {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	return b[:]
}

func rdpDomainParams() []byte {
	inner := snmpEncInt(34)
	inner = append(inner, snmpEncInt(2)...)
	inner = append(inner, snmpEncInt(0)...)
	inner = append(inner, snmpEncInt(1)...)
	inner = append(inner, snmpEncInt(0)...)
	inner = append(inner, snmpEncInt(1)...)
	inner = append(inner, snmpEncInt(65535)...)
	inner = append(inner, snmpEncInt(2)...)
	return snmpTLV(0x30, inner)
}

func rdpApp(tag byte, val []byte) []byte {
	n := len(val)
	var hdr []byte
	switch {
	case n < 128:
		hdr = []byte{0x7f, tag, byte(n)}
	case n < 256:
		hdr = []byte{0x7f, tag, 0x81, byte(n)}
	default:
		hdr = []byte{0x7f, tag, 0x82, byte(n >> 8), byte(n)}
	}
	return append(hdr, val...)
}

func rdpGCC(key string, blocks ...[]byte) []byte {
	var user []byte
	for _, b := range blocks {
		user = append(user, b...)
	}
	out := append([]byte(key), byte(len(user)>>8), byte(len(user)))
	return append(out, user...)
}

func rdpConnectInitial(channels ...string) []byte {
	core := rdpUD(0xC001, append(append(rdpU32LE(0x00080004), rdpU16LE(1024)...), rdpU16LE(768)...))
	net := rdpU32LE(uint32(len(channels)))
	for _, ch := range channels {
		name := make([]byte, 8)
		copy(name, ch)
		net = append(net, name...)
		net = append(net, rdpU32LE(0)...)
	}
	gcc := rdpGCC("Duca", core, rdpUD(0xC003, net))
	params := rdpDomainParams()
	inner := snmpEncOctet([]byte{1})
	inner = append(inner, snmpEncOctet([]byte{1})...)
	inner = append(inner, []byte{0x01, 0x01, 0xff}...)
	inner = append(inner, params...)
	inner = append(inner, params...)
	inner = append(inner, params...)
	inner = append(inner, snmpEncOctet(gcc)...)
	return rdpDT(rdpApp(0x65, inner))
}

func rdpConnectResponse(channels ...string) []byte {
	core := rdpUD(0x0C01, append(rdpU32LE(0x00080004), rdpU32LE(0)...))
	net := append(rdpU16LE(1003), rdpU16LE(uint16(len(channels)))...)
	for i := range channels {
		net = append(net, rdpU16LE(uint16(1004+i))...)
	}
	if len(channels)%2 == 1 {
		net = append(net, 0, 0)
	}
	gcc := rdpGCC("McDn", core, rdpUD(0x0C03, net))
	inner := []byte{0x0a, 0x01, 0x00}
	inner = append(inner, snmpEncInt(0)...)
	inner = append(inner, rdpDomainParams()...)
	inner = append(inner, snmpEncOctet(gcc)...)
	return rdpDT(rdpApp(0x66, inner))
}

func TestProtocolSessionRDPCookieNegotiationAndMCS(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	cr := rdpCR("eltons", rdpProtoRDP)
	p := s.Probe(cr)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "rdp", p.Protocol)
	require.Equal(t, "bcgr", p.Version)

	r := s.Feed(0, ts, cr)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "ConnectionRequest", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Cookie: mstshash=eltons", r.Events[0].Session["Cookie"])
	require.Equal(t, "eltons", r.Events[0].Session["Username"])
	require.Equal(t, "RDP_NEG_REQ", r.Events[0].Session["Negotiation Type"])
	require.Equal(t, uint32(rdpProtoRDP), r.Events[0].Session["Requested Protocols"])

	r = s.Feed(1, ts, rdpCC(rdpNegRsp, rdpProtoRDP))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "ConnectionConfirm", r.Events[0].Session["Packet Name"])
	require.Equal(t, "ConnectionRequest", r.Events[0].Session["Matched Request"])
	require.Equal(t, "PROTOCOL_RDP", r.Events[0].Session["Selected Protocol Name"])
	require.Equal(t, "RDP", r.Events[0].Session["Security"])
	require.Equal(t, "rdp", r.State)

	r = s.Feed(0, ts, rdpConnectInitial("rdpdr", "cliprdr", "rdpsnd"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Connect-Initial", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Duca", r.Events[0].Session["GCC Key"])
	require.Equal(t, []string{"rdpdr", "cliprdr", "rdpsnd"}, r.Events[0].Session["Channels"])
	require.Equal(t, uint32(0x00080004), r.Events[0].Session["RDP Version"])

	r = s.Feed(1, ts, rdpConnectResponse("rdpdr", "cliprdr", "rdpsnd"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Connect-Response", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Connect-Initial", r.Events[0].Session["Matched Request"])
	require.Equal(t, "McDn", r.Events[0].Session["GCC Key"])
}

func TestProtocolSessionRDPTLSCredSSPAndFailure(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	cr := rdpCR("admin", rdpProtoSSL|rdpProtoHybrid)
	p := s.Probe(cr)
	require.Equal(t, ProbeAccept, p.Verdict)
	r := s.Feed(0, ts, cr)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, []string{"PROTOCOL_SSL", "PROTOCOL_HYBRID"}, r.Events[0].Session["Requested Protocol Names"])

	r = s.Feed(1, ts, rdpCC(rdpNegRsp, rdpProtoHybrid))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "PROTOCOL_HYBRID", r.Events[0].Session["Selected Protocol Name"])
	require.Equal(t, "CredSSP", r.Events[0].Session["Security"])
	require.Equal(t, "rdp->tls", r.Events[0].Session["Protocol Transition"])
	require.Equal(t, "tls", r.State)

	hello := []byte{0x16, 0x03, 0x01, 0x00, 0x04, 0x01, 0x00, 0x00, 0x00}
	r = s.Feed(0, ts, hello)
	require.NotEqual(t, "rdp", r.State)
	if len(r.Events) > 0 {
		require.NotEqual(t, "Connect-Initial", r.Events[0].Session["Packet Name"])
	}

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, rdpCR("x", rdpProtoSSL)).Err)
	r = s2.Feed(1, ts, rdpCC(rdpNegRsp, rdpProtoSSL))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "TLS", r.Events[0].Session["Security"])
	require.Equal(t, "tls", r2State(r))

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, rdpCR("x", rdpProtoSSL)).Err)
	r = s3.Feed(1, ts, rdpCC(rdpNegFailure, 5))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "RDP_NEG_FAILURE", r.Events[0].Session["Negotiation Type"])
	require.Equal(t, "HYBRID_REQUIRED_BY_SERVER", r.Events[0].Session["Failure Reason"])
	require.Equal(t, "rdp", r.State)
}

func r2State(r FeedResult) string { return r.State }

func TestProtocolSessionRDPFailClosedAndProbe(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Equal(t, ProbeReject, s.Probe([]byte{3, 0, 0, 11, 6, rdpTPDUCR, 0, 0, 0, 0, 0}).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{3, 0}).Verdict)
	require.Equal(t, ProbeReject, s.Probe([]byte{0x16, 0x03, 0x01}).Verdict)
	require.NotEqual(t, "rdp", s.Probe([]byte("GET / HTTP/1.1\r\n")).Protocol)

	cut := s.Feed(0, ts, rdpCR("eltons", 0)[:6])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r := s2.Feed(0, ts, []byte{3, 0, 0, 7, 2, 0x00, 0x00})
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s3.Feed(1, ts, rdpCC(rdpNegRsp, rdpProtoRDP))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Unmatched"])
}

func TestProtocolSessionRDPFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, rdpCR("eltons", rdpProtoRDP)},
		{1, rdpCC(rdpNegRsp, rdpProtoRDP)},
		{0, rdpConnectInitial("rdpdr", "cliprdr")},
		{1, rdpConnectResponse("rdpdr", "cliprdr")},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		var names []string
		for _, st := range steps {
			for w := st.wire; len(w) > 0; {
				n := len(w)
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(st.dir, ts, w[:n])
				for _, e := range r.Events {
					if e.Status == "decoded" || e.Status == "deferred" {
						names = append(names, fmt.Sprint(e.Session["Packet Name"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}

func TestProtocolSessionRDPSpecCookieHex(t *testing.T) {
	// [MS-RDPBCGR] 4.1.1 Client X.224 Connection Request PDU.
	raw := mustHexSession(t, "0300002c27e00000000000436f6f6b69653a206d737473686173683d656c746f6e730d0a0100080000000000")
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	p := s.Probe(raw)
	require.Equal(t, ProbeAccept, p.Verdict)
	r := s.Feed(0, time.Unix(1, 0), raw)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "eltons", r.Events[0].Session["Username"])
	require.Equal(t, uint32(0), r.Events[0].Session["Requested Protocols"])
}
