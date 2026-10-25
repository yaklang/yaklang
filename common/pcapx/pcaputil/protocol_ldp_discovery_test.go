package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func ldpDiscoveryControl() []byte {
	return []byte{0, 1, 0, 30, 192, 0, 2, 10, 0, 7, 1, 0, 0, 20, 0, 0, 0, 9, 4, 0, 0, 4, 0, 15, 0, 0, 4, 1, 0, 4, 192, 0, 2, 11}
}
func TestLDPDiscoveryNativeFieldsOwnershipAndNoSessionInheritance(t *testing.T) {
	a := routingParser(t)
	decode := func(w []byte, explicit string) *ProtocolEvent {
		e := &ProtocolEvent{Transport: "udp", Source: "192.0.2.1:646", Destination: "224.0.0.2:646", Timestamp: time.Unix(1, 0)}
		require.True(t, a.decodeControlDatagram(e, w, 646, 646, explicit))
		return e
	}
	w := ldpDiscoveryControl()
	first := decode(w, "")
	require.Equal(t, "decoded", first.Status, first.Error)
	require.Equal(t, "LDP Hello", first.Summary)
	require.Zero(t, first.FlowID, "Hello cannot create a TCP/session identity")
	for cut := 0; cut < len(w); cut++ {
		e := &ProtocolEvent{Transport: "udp"}
		require.False(t, a.decodeControlDatagram(e, w[:cut], 646, 646, ""), "cut %d must not inherit prior Hello", cut)
	}
	fields, err := first.GetFields()
	require.NoError(t, err)
	fields["LSR ID"] = "changed"
	first.Session["LSR ID"] = "changed"
	fields, err = first.GetFields()
	require.NoError(t, err)
	require.Equal(t, "192.0.2.10", fields["LSR ID"])
	// Reused sender tuple, changed LSR/label space, absent optional address.
	w = append([]byte(nil), w[:26]...)
	w[3] = 22
	w[13] = 12
	w[7] = 20
	w[9] = 8
	second := decode(w, "")
	require.Equal(t, "decoded", second.Status, second.Error)
	require.Equal(t, "192.0.2.20", second.Session["LSR ID"])
	require.Zero(t, second.FlowID)
	tlv := second.Session["Messages"].([]map[string]any)[0]["TLVs"].([]map[string]any)
	require.Len(t, tlv, 1, "absent transport address must not carry over")
	filter, err := CompileDisplayFilter("ldp.hdr.ldpid.lsid == 8 and ldp.msg.tlv.hello.hold == 15")
	require.NoError(t, err)
	result := filter.EvaluateEvents([]*ProtocolEvent{second})
	require.True(t, result.Complete)
	require.True(t, result.Matched, result.Reason)
	bad := decode([]byte("port 646 is not protocol proof"), "ldp")
	require.NotEqual(t, "decoded", bad.Status)
	require.Empty(t, bad.Session)
	nonstandard := &ProtocolEvent{Transport: "udp"}
	require.False(t, a.decodeControlDatagram(nonstandard, ldpDiscoveryControl(), 53000, 53001, ""))
	require.True(t, a.decodeControlDatagram(nonstandard, ldpDiscoveryControl(), 53000, 53001, "ldp"))
	require.Equal(t, "explicit-decode-as", nonstandard.Admission)
	for _, deferred := range []bool{false, true} {
		c := NewDefaultConfig()
		require.NoError(t, WithProtocolDeferred(deferred)(c))
		require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
		require.NoError(t, c.prepareBinParser())
		e := &ProtocolEvent{Transport: "udp"}
		require.True(t, c.binParser.decodeControlDatagram(e, ldpDiscoveryControl(), 646, 646, ""))
		_, err = e.Decode()
		require.NoError(t, err)
		c.binParser.closeUDPSessions()
	}
}

func FuzzLDPDiscoveryCompleteEnvelope(f *testing.F) {
	f.Add(ldpDiscoveryControl())
	f.Add([]byte{0, 1, 0, 6, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 8192 {
			return
		}
		if m, err := stream_parser.DecodeLDPDiscovery(w, 128); err == nil {
			require.NotEmpty(t, m["Messages"])
			require.Equal(t, false, m["Authentication Verified"])
		}
	})
}
