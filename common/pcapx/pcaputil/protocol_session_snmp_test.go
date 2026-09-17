package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func snmpTLV(tag byte, val []byte) []byte {
	n := len(val)
	var hdr []byte
	switch {
	case n < 128:
		hdr = []byte{tag, byte(n)}
	case n < 256:
		hdr = []byte{tag, 0x81, byte(n)}
	default:
		hdr = []byte{tag, 0x82, byte(n >> 8), byte(n)}
	}
	return append(hdr, val...)
}

func snmpEncInt(v int64) []byte {
	if v < 0 {
		panic("snmp fixture integer must be non-negative")
	}
	if v == 0 {
		return []byte{0x02, 0x01, 0x00}
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	i := 0
	for i < 7 && b[i] == 0 && b[i+1]&0x80 == 0 {
		i++
	}
	return snmpTLV(0x02, b[i:])
}

func snmpEncOctet(b []byte) []byte { return snmpTLV(0x04, b) }

func snmpEncOID(ids ...uint) []byte {
	if len(ids) < 2 {
		panic("oid needs at least two arcs")
	}
	out := []byte{byte(ids[0]*40 + ids[1])}
	for _, n := range ids[2:] {
		if n < 128 {
			out = append(out, byte(n))
			continue
		}
		var tmp [10]byte
		k := 0
		tmp[k] = byte(n & 0x7f)
		n >>= 7
		k++
		for n > 0 {
			tmp[k] = byte(n&0x7f) | 0x80
			n >>= 7
			k++
		}
		for k > 0 {
			k--
			out = append(out, tmp[k])
		}
	}
	return snmpTLV(0x06, out)
}

func snmpVarBind(oid []byte, value []byte) []byte {
	return snmpTLV(0x30, append(append([]byte{}, oid...), value...))
}

func snmpPDU(tag byte, reqID, s1, s2 int64, binds ...[]byte) []byte {
	var list []byte
	for _, b := range binds {
		list = append(list, b...)
	}
	body := append(snmpEncInt(reqID), snmpEncInt(s1)...)
	body = append(body, snmpEncInt(s2)...)
	body = append(body, snmpTLV(0x30, list)...)
	return snmpTLV(tag, body)
}

func snmpUSM(engine []byte, boots, engTime int64, user, auth, priv []byte) []byte {
	inner := append(snmpEncOctet(engine), snmpEncInt(boots)...)
	inner = append(inner, snmpEncInt(engTime)...)
	inner = append(inner, snmpEncOctet(user)...)
	inner = append(inner, snmpEncOctet(auth)...)
	inner = append(inner, snmpEncOctet(priv)...)
	return snmpEncOctet(snmpTLV(0x30, inner))
}

func snmpScoped(engine, name, pdu []byte) []byte {
	inner := append(snmpEncOctet(engine), snmpEncOctet(name)...)
	inner = append(inner, pdu...)
	return snmpTLV(0x30, inner)
}

func snmpV3(msgID, maxSize int64, flags byte, usm, data []byte) []byte {
	header := append(snmpEncInt(msgID), snmpEncInt(maxSize)...)
	header = append(header, snmpEncOctet([]byte{flags})...)
	header = append(header, snmpEncInt(3)...)
	inner := append(snmpEncInt(3), snmpTLV(0x30, header)...)
	inner = append(inner, usm...)
	inner = append(inner, data...)
	return snmpTLV(0x30, inner)
}

func snmpPlainUSM() []byte {
	return snmpUSM([]byte{0x80, 0x00, 0x00, 0x00, 0x01}, 1, 42, []byte("user"), nil, nil)
}

func snmpSysDescr() []byte {
	return snmpEncOID(1, 3, 6, 1, 2, 1, 1, 1, 0)
}

func snmpGet(reqID int64) []byte {
	pdu := snmpPDU(0xa0, reqID, 0, 0, snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00}))
	return snmpV3(reqID, 65507, snmpFlagReportable, snmpPlainUSM(), snmpScoped([]byte{0x80, 0x00, 0x00, 0x00, 0x01}, nil, pdu))
}

func snmpGetResponse(reqID int64, value string) []byte {
	pdu := snmpPDU(0xa2, reqID, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet([]byte(value))))
	return snmpV3(reqID+100, 65507, 0, snmpPlainUSM(), snmpScoped([]byte{0x80, 0x00, 0x00, 0x00, 0x01}, nil, pdu))
}

func snmpGetBulk(reqID, nonRep, maxRep int64) []byte {
	pdu := snmpPDU(0xa5, reqID, nonRep, maxRep, snmpVarBind(snmpEncOID(1, 3, 6, 1, 2, 1, 1), []byte{0x05, 0x00}))
	return snmpV3(reqID, 65507, snmpFlagReportable, snmpPlainUSM(), snmpScoped(nil, nil, pdu))
}

func snmpSet(reqID int64, value string) []byte {
	pdu := snmpPDU(0xa3, reqID, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet([]byte(value))))
	return snmpV3(reqID, 65507, snmpFlagReportable, snmpPlainUSM(), snmpScoped(nil, nil, pdu))
}

func snmpTrap() []byte {
	sysUp := snmpVarBind(snmpEncOID(1, 3, 6, 1, 2, 1, 1, 3, 0), snmpTLV(0x43, []byte{0x01, 0x2c}))
	trapOID := snmpVarBind(snmpEncOID(1, 3, 6, 1, 6, 3, 1, 1, 4, 1, 0), snmpEncOID(1, 3, 6, 1, 6, 3, 1, 1, 5, 1))
	pdu := snmpPDU(0xa7, 99, 0, 0, sysUp, trapOID)
	return snmpV3(99, 65507, 0, snmpPlainUSM(), snmpScoped(nil, nil, pdu))
}

func snmpInform(reqID int64) []byte {
	sysUp := snmpVarBind(snmpEncOID(1, 3, 6, 1, 2, 1, 1, 3, 0), snmpTLV(0x43, []byte{0x00}))
	trapOID := snmpVarBind(snmpEncOID(1, 3, 6, 1, 6, 3, 1, 1, 4, 1, 0), snmpEncOID(1, 3, 6, 1, 6, 3, 1, 1, 5, 3))
	pdu := snmpPDU(0xa6, reqID, 0, 0, sysUp, trapOID)
	return snmpV3(reqID, 65507, snmpFlagReportable, snmpPlainUSM(), snmpScoped(nil, nil, pdu))
}

func snmpReport(reqID int64) []byte {
	pdu := snmpPDU(0xa8, reqID, 0, 0, snmpVarBind(snmpEncOID(1, 3, 6, 1, 6, 3, 15, 1, 1, 4, 0), snmpTLV(0x41, []byte{0x01})))
	return snmpV3(reqID, 65507, 0, snmpPlainUSM(), snmpScoped(nil, nil, pdu))
}

func snmpEncrypted(reqID int64) []byte {
	cipher := []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04, 0x11, 0x22, 0x33, 0x44}
	usm := snmpUSM([]byte{0x80, 0x00, 0x00, 0x00, 0x01}, 1, 42, []byte("user"), make([]byte, 12), make([]byte, 8))
	return snmpV3(reqID, 65507, snmpFlagAuth|snmpFlagPriv|snmpFlagReportable, usm, snmpEncOctet(cipher))
}

func snmpPrivPlaintext(reqID int64) []byte {
	// Adversarial: priv flag set, but scopedPDU is still a plaintext SEQUENCE.
	pdu := snmpPDU(0xa0, reqID, 0, 0, snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00}))
	usm := snmpUSM([]byte{0x80, 0x00, 0x00, 0x00, 0x01}, 1, 42, []byte("user"), make([]byte, 12), make([]byte, 8))
	return snmpV3(reqID, 65507, snmpFlagAuth|snmpFlagPriv, usm, snmpScoped(nil, nil, pdu))
}

func snmpV1Get() []byte {
	community := snmpEncOctet([]byte("public"))
	pdu := snmpPDU(0xa0, 1, 0, 0, snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00}))
	inner := append(snmpEncInt(0), community...)
	inner = append(inner, pdu...)
	return snmpTLV(0x30, inner)
}

func TestProtocolSessionSNMPGetBulkSetTrapInform(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	get := snmpGet(1)
	p := s.Probe(get)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "snmp", p.Protocol)
	require.Equal(t, "3", p.Version)

	r := s.Feed(0, ts, get)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "GetRequest", r.Events[0].Session["Packet Name"])
	require.Equal(t, int64(1), r.Events[0].Session["Request ID"])
	require.Equal(t, "user", r.Events[0].Session["User Name"])
	require.Equal(t, "noAuthNoPriv", r.Events[0].Session["Security Level"])
	binds, _ := r.Events[0].Session["Variable Bindings"].([]map[string]any)
	require.NotEmpty(t, binds)
	require.Equal(t, "1.3.6.1.2.1.1.1.0", binds[0]["OID"])
	require.Equal(t, "NULL", binds[0]["Value Type"])

	r = s.Feed(1, ts, snmpGetResponse(1, "router"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "GetRequest", r.Events[0].Session["Matched Request"])
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])
	binds, _ = r.Events[0].Session["Variable Bindings"].([]map[string]any)
	require.Equal(t, []byte("router"), binds[0]["Value"])

	r = s.Feed(0, ts, snmpGetBulk(2, 0, 10))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "GetBulkRequest", r.Events[0].Session["Packet Name"])
	require.Equal(t, int64(0), r.Events[0].Session["Non-Repeaters"])
	require.Equal(t, int64(10), r.Events[0].Session["Max-Repetitions"])

	r = s.Feed(1, ts, snmpGetResponse(2, "bulk"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "GetBulkRequest", r.Events[0].Session["Matched Request"])

	r = s.Feed(0, ts, snmpSet(3, "name"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SetRequest", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, snmpGetResponse(3, "name"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SetRequest", r.Events[0].Session["Matched Request"])

	r = s.Feed(1, ts, snmpTrap())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "TrapV2", r.Events[0].Session["Packet Name"])
	require.Equal(t, true, r.Events[0].Session["Unsolicited"])
	binds, _ = r.Events[0].Session["Variable Bindings"].([]map[string]any)
	require.Equal(t, "TimeTicks", binds[0]["Value Type"])
	require.Equal(t, "1.3.6.1.6.3.1.1.5.1", binds[1]["Value"])

	r = s.Feed(0, ts, snmpInform(4))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "InformRequest", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, snmpGetResponse(4, "ack"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "InformRequest", r.Events[0].Session["Matched Request"])

	r = s.Feed(1, ts, snmpReport(5))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Report", r.Events[0].Session["Packet Name"])
	require.Equal(t, true, r.Events[0].Session["Unmatched"])
	require.Equal(t, "missing-request", r.Events[0].Session["Association Status"])
	binds, _ = r.Events[0].Session["Variable Bindings"].([]map[string]any)
	require.Equal(t, "1.3.6.1.6.3.15.1.1.4.0", binds[0]["OID"])
	require.Equal(t, "Counter32", binds[0]["Value Type"])
}

func TestProtocolSessionSNMPFailClosedAndProbe(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)

	bindBody := append([]byte{2, 1, 3}, ldapSessionTLV(4, nil)...)
	bindBody = append(bindBody, 0x80, 0)
	bind := ldapSessionMsg(0x60, bindBody)
	require.Equal(t, "ldap", s.Probe(bind).Protocol)
	bind[4] = 3
	sLDAP, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, "ldap", sLDAP.Probe(bind).Protocol)

	require.Equal(t, ProbeReject, s.Probe(snmpV1Get()).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{0x30}).Verdict)
	require.NotEqual(t, "snmp", s.Probe([]byte{0x30, 0x05, 0x02, 0x01, 0x00}).Protocol)

	enc := snmpEncrypted(7)
	p := s.Probe(enc)
	require.Equal(t, ProbeAccept, p.Verdict)
	r := s.Feed(0, ts, enc)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
	require.NotEmpty(t, r.Events)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "EncryptedScopedPDU", r.Events[0].Session["Packet Name"])
	require.Equal(t, "authPriv", r.Events[0].Session["Security Level"])
	require.NotEqual(t, "decoded", r.Events[0].Status)
	require.Nil(t, r.Events[0].Session["Variable Bindings"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s2.Feed(0, ts, snmpPrivPlaintext(8))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.NotEqual(t, "GetRequest", r.Events[0].Session["Packet Name"])

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	cut := s3.Feed(0, ts, snmpGet(1)[:8])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)

	s4, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s4.Feed(0, ts, []byte{0x30, 0x06, 0x02, 0x01, 0x03, 0x01, 0x00})
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")
}

func TestProtocolSessionSNMPFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, snmpGet(1)},
		{1, snmpGetResponse(1, "router")},
		{0, snmpGetBulk(2, 0, 10)},
		{1, snmpGetResponse(2, "bulk")},
		{0, snmpSet(3, "name")},
		{1, snmpGetResponse(3, "name")},
		{1, snmpTrap()},
		{0, snmpInform(4)},
		{1, snmpGetResponse(4, "ack")},
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
