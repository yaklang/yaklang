package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

func snmpPDUWithRawIntegers(tag byte, requestID, status, index []byte, binds ...[]byte) []byte {
	var list []byte
	for _, bind := range binds {
		list = append(list, bind...)
	}
	body := append([]byte{}, requestID...)
	body = append(body, status...)
	body = append(body, index...)
	body = append(body, snmpTLV(0x30, list)...)
	return snmpTLV(tag, body)
}

func snmpV1TrapWithRawIntegers(generic, specific []byte) []byte {
	body := append(snmpEncOID(1, 3, 6, 1, 4, 1, 999), snmpTLV(0x40, []byte{192, 0, 2, 10})...)
	body = append(body, generic...)
	body = append(body, specific...)
	body = append(body, snmpTLV(0x43, []byte{1})...)
	body = append(body, snmpTLV(0x30, nil)...)
	return snmpCommunityMessage(0, []byte("public"), snmpTLV(0xa4, body))
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

func snmpV3GetWithSecurityContext(reqID, msgID int64, engineID, user []byte) []byte {
	pdu := snmpPDU(0xa0, reqID, 0, 0, snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00}))
	usm := snmpUSM(engineID, 1, 42, user, nil, nil)
	return snmpV3(msgID, 65507, snmpFlagReportable, usm, snmpScoped(engineID, nil, pdu))
}

func snmpV3ResponseWithSecurityContext(reqID, msgID int64, engineID, user []byte) []byte {
	pdu := snmpPDU(0xa2, reqID, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet([]byte("router"))))
	usm := snmpUSM(engineID, 1, 42, user, nil, nil)
	return snmpV3(msgID, 65507, 0, usm, snmpScoped(engineID, nil, pdu))
}

func snmpGetResponse(reqID int64, value string) []byte {
	pdu := snmpPDU(0xa2, reqID, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet([]byte(value))))
	return snmpV3(reqID, 65507, 0, snmpPlainUSM(), snmpScoped([]byte{0x80, 0x00, 0x00, 0x00, 0x01}, nil, pdu))
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
	return snmpV1GetWithID(1)
}

func snmpV1GetWithID(id int64) []byte {
	pdu := snmpPDU(0xa0, id, 0, 0, snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00}))
	return snmpCommunityMessage(0, []byte("public"), pdu)
}

func snmpCommunityMessage(version int64, community, pdu []byte) []byte {
	inner := append(snmpEncInt(version), snmpEncOctet(community)...)
	inner = append(inner, pdu...)
	return snmpTLV(0x30, inner)
}

func snmpV1Response(id int64, value string) []byte {
	pdu := snmpPDU(0xa2, id, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet([]byte(value))))
	return snmpCommunityMessage(0, []byte("public"), pdu)
}

func snmpV2Get(id int64, community string) []byte {
	pdu := snmpPDU(0xa0, id, 0, 0, snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00}))
	return snmpCommunityMessage(1, []byte(community), pdu)
}

func snmpV2Response(id int64, community, value string) []byte {
	pdu := snmpPDU(0xa2, id, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet([]byte(value))))
	return snmpCommunityMessage(1, []byte(community), pdu)
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

func TestProtocolSessionSNMPResponseFirstDirectionAndPendingExpiry(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	newParser := func() *binSNMP {
		return &binSNMP{pending: map[snmpPendingKey]snmpPendingRequest{}}
	}

	t.Run("response-first-and-same-direction-response-do-not-consume-request", func(t *testing.T) {
		parser := newParser()
		fields, err := parser.consumeAt(snmpV1Response(7, "early"), 16, 1, t0)
		require.NoError(t, err)
		require.Equal(t, "missing-request", fields["Association Status"])
		require.Equal(t, true, fields["Unmatched"])

		fields, err = parser.consumeAt(snmpV1Get(), 16, 0, t0.Add(time.Second))
		require.NoError(t, err)
		require.Equal(t, true, fields["Outstanding"])
		require.Len(t, parser.pending, 1)

		fields, err = parser.consumeAt(snmpV1Response(1, "wrong direction"), 16, 0, t0.Add(2*time.Second))
		require.NoError(t, err)
		require.Equal(t, "missing-request", fields["Association Status"])
		require.Len(t, parser.pending, 1, "a response from the request direction must not consume its context")

		fields, err = parser.consumeAt(snmpV1Response(1, "matched"), 16, 1, t0.Add(3*time.Second))
		require.NoError(t, err)
		require.Equal(t, "GetRequest", fields["Matched Request"])
		require.Empty(t, parser.pending)
	})

	t.Run("expired-id-is-evicted-before-budget-check-and-reuse", func(t *testing.T) {
		parser := newParser()
		const maxPending = 16
		fields, err := parser.consumeAt(snmpV1Get(), maxPending, 0, t0)
		require.NoError(t, err)
		require.Equal(t, true, fields["Outstanding"])

		fields, err = parser.consumeAt(snmpV1Get(), maxPending, 0, t0.Add(snmpPendingTTL-time.Second))
		require.NoError(t, err)
		require.Equal(t, true, fields["Retransmission"], "retransmission keeps the original transaction key")
		require.Len(t, parser.pending, 1)

		fields, err = parser.consumeAt(snmpV1Response(1, "still in retry window"), maxPending, 1, t0.Add(snmpPendingTTL))
		require.NoError(t, err)
		require.Equal(t, "GetRequest", fields["Matched Request"], "a retry refreshes the timeout")
		require.Empty(t, parser.pending)

		for id := int64(1); id <= maxPending; id++ {
			_, err = parser.consumeAt(snmpV1GetWithID(id), maxPending, 0, t0.Add(2*snmpPendingTTL))
			require.NoError(t, err)
		}
		require.Len(t, parser.pending, maxPending)
		fields, err = parser.consumeAt(snmpV1GetWithID(maxPending+1), maxPending, 0, t0.Add(3*snmpPendingTTL))
		require.NoError(t, err, "expired unmatched requests must not permanently consume the pending budget")
		require.NotContains(t, fields, "Retransmission")
		require.Len(t, parser.pending, 1)

		fields, err = parser.consumeAt(snmpV1GetWithID(maxPending+1), maxPending, 0, t0.Add(4*snmpPendingTTL))
		require.NoError(t, err)
		require.NotContains(t, fields, "Retransmission", "ID reuse after expiry starts a new context")
	})

	t.Run("v3-message-id-also-requires-authoritative-engine-and-user-context", func(t *testing.T) {
		parser := newParser()
		engineID := []byte{0x80, 0, 0, 0, 1}
		otherEngine := []byte{0x80, 0, 0, 0, 2}
		user := []byte("observer")
		otherUser := []byte("different-observer")
		fields, err := parser.consumeAt(snmpV3GetWithSecurityContext(25, 25, engineID, user), 64, 0, t0)
		require.NoError(t, err)
		require.Equal(t, "GetRequest", fields["Packet Name"])

		for _, tc := range []struct {
			name     string
			engineID []byte
			user     []byte
		}{
			{name: "different-authoritative-engine", engineID: otherEngine, user: user},
			{name: "different-user", engineID: engineID, user: otherUser},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fields, err := parser.consumeAt(snmpV3ResponseWithSecurityContext(25, 25, tc.engineID, tc.user), 64, 1, t0.Add(time.Second))
				require.NoError(t, err)
				require.Equal(t, "missing-request", fields["Association Status"])
				require.Len(t, parser.pending, 1, "mismatched USM identity must not consume the matching request context")
			})
		}
		fields, err = parser.consumeAt(snmpV3ResponseWithSecurityContext(25, 25, engineID, user), 64, 1, t0.Add(2*time.Second))
		require.NoError(t, err)
		require.Equal(t, "GetRequest", fields["Matched Request"])
		require.Empty(t, parser.pending)
	})
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

	require.Equal(t, ProbeAccept, s.Probe(snmpV1Get()).Verdict)
	require.Equal(t, "1", s.Probe(snmpV1Get()).Version)
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
	require.Equal(t, ErrMalformedMessage, r.Err.Kind, "the privacy bit requires an encrypted OCTET STRING; malformed plaintext must not be relabeled as ciphertext")
	require.NotEqual(t, "decoded", r.Events[0].Status)
	require.NotEqual(t, "GetRequest", r.Events[0].Session["Packet Name"])
	require.NotEqual(t, true, r.Events[0].Session["Encrypted"])

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

func TestFirstBatchT18(t *testing.T) {
	t.Run("net-snmp-v1-real-agent-capture-and-tshark-oracle", func(t *testing.T) {
		capture, err := os.ReadFile("testdata/protocol-sessions/first-batch-oracles/net-snmp-v1-live.pcap")
		require.NoError(t, err)
		sum := sha256.Sum256(capture)
		require.Equal(t, "e75ce61c2bd6d62c8169f45bdf21e70602b61724f34b4c8fe7cbde82b33fb5d7", hex.EncodeToString(sum[:]))
		oracle, err := os.ReadFile("testdata/protocol-sessions/first-batch-oracles/net-snmp-v1-tshark-oracle.tsv")
		require.NoError(t, err)
		require.Contains(t, string(oracle), "snmp.version\tsnmp.request_id\tsnmp.error_status")
		require.Contains(t, string(oracle), "\"0\"\t\"1374073356\"\t\"2\"\t\"1\"")
		require.Contains(t, string(oracle), "\"1.3.6.1.4.1.8072.9999\"")
		tshark := csv.NewReader(bytes.NewReader(oracle))
		tshark.Comma = '\t'
		records, err := tshark.ReadAll()
		require.NoError(t, err)
		require.Len(t, records, 8, "seven captured PDUs plus a TSV header")
		columns := make(map[string]int, len(records[0]))
		for i, name := range records[0] {
			columns[name] = i
		}
		oracleByFrame := make(map[string][]string, len(records)-1)
		for _, row := range records[1:] {
			oracleByFrame[row[columns["frame.number"]]] = row
		}
		oraclePDUName := func(row []string) string {
			switch {
			case row[columns["snmp.get_request_element"]] != "":
				return "GetRequest"
			case row[columns["snmp.get_next_request_element"]] != "":
				return "GetNextRequest"
			case row[columns["snmp.get_response_element"]] != "":
				return "Response"
			case row[columns["snmp.trap_element"]] != "":
				return "TrapV1"
			default:
				return ""
			}
		}
		for _, deferred := range []bool{false, true} {
			for _, workers := range []int{1, 2, 4} {
				t.Run(fmt.Sprintf("deferred=%v/workers=%d", deferred, workers), func(t *testing.T) {
					var events []*ProtocolEvent
					err := ReplayPcap(bytes.NewReader(capture), WithTCPReassemblyWorkers(workers), WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }}))
					require.NoError(t, err)
					var snmp []*ProtocolEvent
					for _, event := range events {
						if event.Protocol == "snmp" {
							snmp = append(snmp, event)
						}
					}
					require.Len(t, snmp, 7)
					counts := map[string]int{}
					for _, event := range snmp {
						require.Equal(t, "udp", event.Transport)
						require.Len(t, event.SourceBytes.PacketRefs, 1)
						frame := strconv.FormatUint(event.SourceBytes.PacketRefs[0].Number, 10)
						row, ok := oracleByFrame[frame]
						require.True(t, ok, "frame %s is present in independent TShark output", frame)
						wantStatus := "decoded"
						if deferred {
							wantStatus = "deferred"
						}
						require.Equal(t, wantStatus, event.Status)
						require.NotEmpty(t, event.SourceBytes.PacketRefs)
						fields, decodeErr := event.GetFields()
						require.NoError(t, decodeErr)
						require.Equal(t, "1", fields["Version Name"])
						name := fmt.Sprint(fields["Packet Name"])
						require.Equal(t, oraclePDUName(row), name, "frame %s PDU", frame)
						version, parseErr := strconv.ParseInt(row[columns["snmp.version"]], 10, 64)
						require.NoError(t, parseErr)
						require.Equal(t, version, fields["Version"])
						if requestID := row[columns["snmp.request_id"]]; requestID != "" {
							wantID, parseErr := strconv.ParseInt(requestID, 10, 64)
							require.NoError(t, parseErr)
							require.Equal(t, wantID, fields["Request ID"], "frame %s request-id", frame)
						}
						for column, field := range map[string]string{"snmp.error_status": "Error Status", "snmp.error_index": "Error Index"} {
							if value := row[columns[column]]; value != "" {
								want, parseErr := strconv.ParseInt(value, 10, 64)
								require.NoError(t, parseErr)
								require.Equal(t, want, fields[field], "frame %s %s", frame, field)
							}
						}
						if oids := row[columns["snmp.name"]]; oids != "" {
							bindings := fields["Variable Bindings"].([]map[string]any)
							actualOIDs := make([]string, 0, len(bindings))
							for _, binding := range bindings {
								actualOIDs = append(actualOIDs, fmt.Sprint(binding["OID"]))
							}
							require.Equal(t, strings.Split(oids, ","), actualOIDs, "frame %s OIDs", frame)
						}
						if responseTo := row[columns["snmp.response_to"]]; responseTo != "" {
							requestRow, exists := oracleByFrame[responseTo]
							require.True(t, exists)
							require.Equal(t, oraclePDUName(requestRow), fields["Matched Request"], "frame %s response pairing", frame)
						}
						counts[name]++
						switch name {
						case "GetRequest":
							if fmt.Sprint(fields["Request ID"]) == "1374073356" {
								binds := fields["Variable Bindings"].([]map[string]any)
								require.Len(t, binds, 1)
								require.Equal(t, "1.3.6.1.2.1.1.99.0", binds[0]["OID"])
							}
						case "Response":
							if fmt.Sprint(fields["Request ID"]) == "1374073356" {
								require.Equal(t, int64(2), fields["Error Status"])
								require.Equal(t, int64(1), fields["Error Index"])
								require.Equal(t, "GetRequest", fields["Matched Request"])
							}
						case "TrapV1":
							require.Equal(t, int64(6), fields["Generic Trap"])
							require.Equal(t, int64(7), fields["Specific Trap"])
							require.Equal(t, "1.3.6.1.4.1.8072.9999", fields["Enterprise OID"])
						}
					}
					require.Equal(t, 2, counts["GetRequest"])
					require.Equal(t, 1, counts["GetNextRequest"])
					require.Equal(t, 3, counts["Response"])
					require.Equal(t, 1, counts["TrapV1"])
				})
			}
		}
	})

	t.Run("real-upstream-udp-161-162-capture", func(t *testing.T) {
		capture := binCorpusBytes(t, "ndpi/ndpi-snmp.pcap")
		sum := sha256.Sum256(capture)
		require.Equal(t, "4e6820e58cbd04f057b6c6ad7bbccd1f5535e23c88c97c1b69ad96ce1c9677ab", hex.EncodeToString(sum[:]))
		events, stats, err := binReplay(t, capture, 2)
		require.NoError(t, err)
		require.Zero(t, stats.BufferedBytes)
		var packets, decoded, malformed int
		versions := map[string]int{}
		for _, event := range events {
			if event.Protocol != "snmp" || event.Transport != "udp" {
				continue
			}
			packets++
			require.NotEmpty(t, event.SourceBytes.PacketRefs)
			switch event.Status {
			case "decoded":
				decoded++
			case "malformed":
				malformed++
			}
			if event.Session != nil {
				versions[fmt.Sprint(event.Session["Version Name"])]++
				if event.Session["Community Status"] == "present-unverified-cleartext" {
					require.NotContains(t, event.Session, "Community", "community text is sensitive and must not be exposed")
					require.Equal(t, false, event.Session["Verified"])
				}
			}
		}
		require.Equal(t, 72, packets, "the upstream corpus has 72 SNMP UDP messages on ports 161/162")
		require.NotEmpty(t, versions)
		t.Logf("tshark 4.4.8 oracle: 72 SNMP UDP packets; event version metadata=%v decoded=%d malformed=%d", versions, decoded, malformed)
	})
	t.Run("net-snmp-live-agent-trap-inform-capture", func(t *testing.T) {
		capture, err := os.ReadFile("testdata/protocol-sessions/first-batch-oracles/net-snmp-rsyslog-interop-v2c.pcap")
		require.NoError(t, err)
		sum := sha256.Sum256(capture)
		require.Equal(t, "10087cb684d98acad8a2da7e95620edd8e9787176a84344d5b5ff69daa0dfb18", hex.EncodeToString(sum[:]))
		for _, deferred := range []bool{false, true} {
			for _, workers := range []int{1, 2, 4} {
				t.Run(fmt.Sprintf("deferred=%v/workers=%d", deferred, workers), func(t *testing.T) {
					var events []*ProtocolEvent
					var stats BinParserStats
					err := ReplayPcap(bytes.NewReader(capture), WithTCPReassemblyWorkers(workers), WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }, OnStats: func(s BinParserStats) { stats = s }}))
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					var snmpEvents []*ProtocolEvent
					for _, event := range events {
						if event.Protocol == "snmp" {
							snmpEvents = append(snmpEvents, event)
						}
					}
					require.Len(t, snmpEvents, 5, "GET/response, trap, inform/response from a live Net-SNMP agent and client")
					wantStatus := "decoded"
					if deferred {
						wantStatus = "deferred"
					}
					packetNames := map[string]int{}
					var getRequestID any
					for _, event := range snmpEvents {
						require.Equal(t, "udp", event.Transport)
						require.Equal(t, wantStatus, event.Status)
						require.NotEmpty(t, event.SourceBytes.PacketRefs)
						fields, decodeErr := event.GetFields()
						require.NoError(t, decodeErr)
						name := fmt.Sprint(fields["Packet Name"])
						packetNames[name]++
						require.Equal(t, "2c", fields["Version Name"])
						require.Equal(t, "community-cleartext-unverified", fields["Security Level"])
						require.Equal(t, false, fields["Verified"])
						require.NotContains(t, fields, "Community")
						if name == "GetRequest" {
							getRequestID = fields["Request ID"]
						}
						if name == "Response" && fields["Matched Request"] == "GetRequest" {
							require.Equal(t, getRequestID, fields["Request ID"])
						}
						if name == "Response" && fields["Matched Request"] == "InformRequest" {
							require.Equal(t, "InformRequest", fields["Matched Request"])
						}
						if name == "InformRequest" {
							require.NotContains(t, fields, "Matched Request")
						}
					}
					require.Equal(t, 1, packetNames["GetRequest"])
					require.Equal(t, 2, packetNames["Response"], "agent GET response plus inform acknowledgment")
					require.Equal(t, 1, packetNames["TrapV2"])
					require.Equal(t, 1, packetNames["InformRequest"])
				})
			}
		}
	})
	t.Run("net-snmp-v3-noauth-visible-authpriv-opaque-capture", func(t *testing.T) {
		capture, err := os.ReadFile("testdata/protocol-sessions/first-batch-oracles/net-snmp-v3-security-boundaries.pcap")
		require.NoError(t, err)
		sum := sha256.Sum256(capture)
		require.Equal(t, "248aa80cf33f4b4b4f2ca8f42d9bd3d3468844c8cc98b2ad9909b898bad7f067", hex.EncodeToString(sum[:]))
		for _, deferred := range []bool{false, true} {
			for _, workers := range []int{1, 2, 4} {
				t.Run(fmt.Sprintf("deferred=%v/workers=%d", deferred, workers), func(t *testing.T) {
					var events []*ProtocolEvent
					var stats BinParserStats
					err := ReplayPcap(bytes.NewReader(capture), WithTCPReassemblyWorkers(workers), WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }, OnStats: func(s BinParserStats) { stats = s }}))
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					wantDecoded := "decoded"
					if deferred {
						wantDecoded = "deferred"
					}
					var decoded, encrypted int
					packetNames := map[string]int{}
					for _, event := range events {
						if event.Protocol != "snmp" {
							continue
						}
						require.Equal(t, "udp", event.Transport)
						require.NotEmpty(t, event.SourceBytes.PacketRefs)
						fields, decodeErr := event.GetFields()
						require.NoError(t, decodeErr)
						require.Equal(t, "v3", fields["Version Name"])
						require.Equal(t, false, fields["Verified"], "capture contains no supplied USM keys")
						name := fmt.Sprint(fields["Packet Name"])
						packetNames[name]++
						if fields["Security Level"] == "authPriv" {
							encrypted++
							require.Equal(t, "context-required", event.Status)
							require.NotNil(t, event.sessionError)
							require.Equal(t, ErrEncrypted, event.sessionError.Kind)
							require.Equal(t, true, fields["Encrypted"])
							require.Equal(t, "EncryptedScopedPDU", name)
							require.NotContains(t, fields, "Variable Bindings", "authPriv scopedPDU must not be guessed as plaintext")
							require.Contains(t, []string{"test-authpriv"}, fields["User Name"])
							continue
						}
						decoded++
						require.Equal(t, wantDecoded, event.Status)
						require.Equal(t, "noAuthNoPriv", fields["Security Level"])
						require.Equal(t, "not-verified-no-key-material", fields["Authentication Status"])
						if fields["User Name"] == "test-noauth" && name == "GetRequest" {
							binds, ok := fields["Variable Bindings"].([]map[string]any)
							require.True(t, ok)
							require.NotEmpty(t, binds)
							require.Equal(t, "1.3.6.1.2.1.1.3.0", binds[0]["OID"])
						}
						if name == "Report" {
							binds, ok := fields["Variable Bindings"].([]map[string]any)
							require.True(t, ok)
							require.NotEmpty(t, binds)
							require.Equal(t, "1.3.6.1.6.3.15.1.1.4.0", binds[0]["OID"])
						}
					}
					require.Equal(t, 6, decoded, "discovery reports and noAuthNoPriv request/response are structurally visible")
					require.Equal(t, 2, encrypted, "authPriv GetRequest and Response retain only visible USM metadata")
					require.Equal(t, 2, packetNames["Report"])
					require.Equal(t, 3, packetNames["GetRequest"], "two engine discovery probes and one noAuthNoPriv request")
					require.Equal(t, 1, packetNames["Response"])
					require.Equal(t, 2, packetNames["EncryptedScopedPDU"])
				})
			}
		}
	})

	t.Run("v1-v2c-typed-pdu-and-response-pairing", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		r := s.Feed(0, ts, snmpV1Get())
		require.Nil(t, r.Err)
		require.Equal(t, "1", r.Events[0].Session["Version Name"])
		require.Equal(t, "GetRequest", r.Events[0].Session["Packet Name"])
		require.Equal(t, "community-cleartext-unverified", r.Events[0].Session["Security Level"])
		require.NotContains(t, r.Events[0].Session, "Community")
		require.Equal(t, "1.3.6.1.2.1.1.1.0", r.Events[0].Session["Variable Bindings"].([]map[string]any)[0]["OID"])
		r = s.Feed(1, ts, snmpV1Response(1, "router"))
		require.Nil(t, r.Err)
		require.Equal(t, "GetRequest", r.Events[0].Session["Matched Request"])

		r = s.Feed(0, ts, snmpV2Get(7, "private"))
		require.Nil(t, r.Err)
		require.Equal(t, "2c", r.Events[0].Session["Version Name"])
		r = s.Feed(1, ts, snmpV2Response(7, "public", "wrong-community"))
		require.Nil(t, r.Err)
		require.Equal(t, true, r.Events[0].Session["Unmatched"])
		r = s.Feed(1, ts, snmpV2Response(7, "private", "right-community"))
		require.Nil(t, r.Err)
		require.Equal(t, "GetRequest", r.Events[0].Session["Matched Request"])
	})

	t.Run("v1-trap-oid-and-list-budgets", func(t *testing.T) {
		trapBody := append(snmpEncOID(1, 3, 6, 1, 4, 1, 999), snmpTLV(0x40, []byte{192, 0, 2, 10})...)
		trapBody = append(trapBody, snmpEncInt(6)...)
		trapBody = append(trapBody, snmpEncInt(1)...)
		trapBody = append(trapBody, snmpTLV(0x43, []byte{1, 2, 3})...)
		trapBody = append(trapBody, snmpTLV(0x30, snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00}))...)
		fields, err := (&binSNMP{}).consume(snmpCommunityMessage(0, []byte("public"), snmpTLV(0xa4, trapBody)), 64, 0)
		require.NoError(t, err)
		require.Equal(t, "TrapV1", fields["Packet Name"])
		require.Equal(t, true, fields["Unsolicited"])
		require.Equal(t, "192.0.2.10", fields["Agent Address"])
		_, err = snmpParseVarBinds(snmpConcatVarBinds(t, 3), 2)
		require.Error(t, err)
		_, err = snmpDecodeOIDChecked([]byte{0x2b, 0x86}, 64)
		require.Error(t, err, "unterminated base-128 OID arc")
	})

	t.Run("nonstandard-port-explicit-decode-as", func(t *testing.T) {
		var events []*ProtocolEvent
		capture := sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: snmpV2Get(11, "monitor")}, {dir: 1, wire: snmpV2Response(11, "monitor", "edge")}}, 1161)
		err := ReplayPcap(bytes.NewReader(capture), WithProtocolDecodeAs("udp", 1161, "snmp"), WithTCPReassemblyWorkers(2), WithBinParser(func(event *ProtocolEvent) { events = append(events, event) }))
		require.NoError(t, err)
		require.Len(t, events, 2)
		require.Equal(t, "snmp", events[0].Protocol)
		require.Equal(t, "explicit-decode-as", events[0].Admission)
		require.Equal(t, "GetRequest", events[1].Session["Matched Request"])
	})

	t.Run("v3-no-key-remains-opaque-and-msgid-matches", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(2, 0)
		r := s.Feed(0, ts, snmpGet(25))
		require.Nil(t, r.Err)
		require.Equal(t, false, r.Events[0].Session["Verified"])
		r = s.Feed(1, ts, snmpGetResponse(25, "router"))
		require.Nil(t, r.Err)
		require.Equal(t, "GetRequest", r.Events[0].Session["Matched Request"])
		_, err = (&binSNMP{}).consume(snmpEncrypted(26), 64, 0)
		require.Error(t, err)
		require.Equal(t, ErrEncrypted, err.(*ProtocolError).Kind)
	})
}

func BenchmarkFirstBatchSNMPv1Replay(b *testing.B) {
	capture, err := os.ReadFile("testdata/protocol-sessions/first-batch-oracles/net-snmp-v1-live.pcap")
	if err != nil {
		b.Fatal(err)
	}
	for _, mode := range []struct {
		name     string
		deferred bool
		decode   bool
	}{
		{name: "full"},
		{name: "deferred-capture", deferred: true},
		{name: "deferred-on-demand", deferred: true, decode: true},
	} {
		for _, workers := range []int{1, 2, 4} {
			b.Run(fmt.Sprintf("%s/workers-%d", mode.name, workers), func(b *testing.B) {
				b.SetBytes(int64(len(capture)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var count atomic.Int64
					var eventMu sync.Mutex
					var eventErr error
					err := ReplayPcap(bytes.NewReader(capture), WithTCPReassemblyWorkers(workers), WithBinParserConfig(BinParserConfig{
						Deferred: mode.deferred,
						OnEvent: func(event *ProtocolEvent) {
							if event.Protocol != "snmp" {
								return
							}
							count.Add(1)
							if mode.decode {
								if _, decodeErr := event.GetFields(); decodeErr != nil {
									eventMu.Lock()
									if eventErr == nil {
										eventErr = decodeErr
									}
									eventMu.Unlock()
								}
							}
						},
					}))
					if err != nil {
						b.Fatal(err)
					}
					eventMu.Lock()
					callbackErr := eventErr
					eventMu.Unlock()
					if callbackErr != nil {
						b.Fatal(callbackErr)
					}
					if count.Load() != 7 {
						b.Fatalf("expected 7 SNMP PDUs, got %d", count.Load())
					}
				}
				b.ReportMetric(7, "pdus/op")
			})
		}
	}
}

func TestProtocolSessionSNMPRejectsNonCanonicalSignedIntegersAndOIDArcs(t *testing.T) {
	parser := &binSNMP{}
	canonicalOne := []byte{0x02, 0x01, 0x01}
	canonicalZero := []byte{0x02, 0x01, 0x00}
	nonCanonicalPositive := []byte{0x02, 0x02, 0x00, 0x01}
	nonCanonicalNegative := []byte{0x02, 0x02, 0xff, 0xff}
	bind := snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00})

	t.Run("pdu-request-id-error-status-error-index", func(t *testing.T) {
		cases := []struct {
			name              string
			requestID, status []byte
			index             []byte
		}{
			{name: "request-id", requestID: nonCanonicalPositive, status: canonicalZero, index: canonicalZero},
			{name: "error-status-positive", requestID: canonicalOne, status: nonCanonicalPositive, index: canonicalZero},
			{name: "error-status-negative", requestID: canonicalOne, status: nonCanonicalNegative, index: canonicalZero},
			{name: "error-index", requestID: canonicalOne, status: canonicalZero, index: nonCanonicalPositive},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				wire := snmpCommunityMessage(1, []byte("public"), snmpPDUWithRawIntegers(0xa0, tc.requestID, tc.status, tc.index, bind))
				_, err := parser.consume(wire, 64, 0)
				require.Error(t, err, "non-minimal BER INTEGER must be rejected")
			})
		}
	})

	t.Run("canonical-integer-sign-boundary-is-accepted", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			value []byte
			want  int64
		}{
			{name: "positive-needs-leading-zero", value: []byte{0x02, 0x02, 0x00, 0x80}, want: 128},
			{name: "negative-needs-leading-ones", value: []byte{0x02, 0x02, 0xff, 0x7f}, want: -129},
		} {
			t.Run(tc.name, func(t *testing.T) {
				wire := snmpCommunityMessage(1, []byte("public"), snmpPDUWithRawIntegers(0xa0, tc.value, canonicalZero, canonicalZero, bind))
				fields, err := parser.consume(wire, 64, 0)
				require.NoError(t, err)
				require.Equal(t, tc.want, fields["Request ID"])
			})
		}
	})

	t.Run("v1-trap-specific-integers", func(t *testing.T) {
		for _, tc := range []struct {
			name              string
			generic, specific []byte
		}{
			{name: "generic-trap", generic: nonCanonicalPositive, specific: canonicalOne},
			{name: "specific-trap", generic: canonicalOne, specific: nonCanonicalPositive},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := parser.consume(snmpV1TrapWithRawIntegers(tc.generic, tc.specific), 64, 0)
				require.Error(t, err, "non-minimal v1 Trap INTEGER must be rejected")
			})
		}
	})

	t.Run("signed-varbind-integer", func(t *testing.T) {
		pdu := snmpPDU(0xa0, 1, 0, 0, snmpVarBind(snmpSysDescr(), nonCanonicalPositive))
		_, err := parser.consume(snmpCommunityMessage(1, []byte("public"), pdu), 64, 0)
		require.Error(t, err, "non-minimal universal INTEGER values must be rejected too")
	})

	t.Run("oid-subidentifier-starts", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			wire []byte
		}{
			{name: "second-arc", wire: []byte{0x2b, 0x80, 0x01}},
			{name: "later-arc", wire: []byte{0x2b, 0x01, 0x80, 0x01}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := snmpDecodeOIDChecked(tc.wire, 64)
				require.Error(t, err, "each base-128 subidentifier must use its shortest encoding")
			})
		}

		got, err := snmpDecodeOIDChecked([]byte{0x88, 0x37}, 64)
		require.NoError(t, err, "the first combined subidentifier for 2.999 is valid multi-octet BER")
		require.Equal(t, "2.999", got)
	})
}

// FuzzSNMPBoundedBER covers both admission and the version-specific decoders.
// SNMP fields are untrusted capture bytes: fuzzing must not panic, retain an
// unbounded request table, or let the BER walker escape the supplied message.
func FuzzSNMPBoundedBER(f *testing.F) {
	for _, wire := range [][]byte{
		snmpGet(1),
		snmpGetBulk(2, 1, 4),
		snmpGetResponse(3, "router"),
		snmpTrap(),
		snmpInform(4),
		snmpEncrypted(5),
		{0x30, 0x81, 0xff, 0x02, 0x01, 0x03},
		{0x30, 0x80, 0, 0},
	} {
		f.Add(wire)
	}
	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) > 1<<16 {
			t.Skip()
		}
		_ = probeSNMP(wire, 4096)
		decoder := &binSNMP{}
		_, _ = decoder.consume(wire, 128, 0)
		if len(decoder.pending) > 128 {
			t.Fatalf("SNMP pending table exceeded decode budget: %d", len(decoder.pending))
		}
	})
}

func snmpConcatVarBinds(t *testing.T, count int) []byte {
	t.Helper()
	var out []byte
	for i := 0; i < count; i++ {
		out = append(out, snmpVarBind(snmpSysDescr(), []byte{0x05, 0x00})...)
	}
	return out
}
