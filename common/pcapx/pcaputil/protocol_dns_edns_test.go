package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func reviewDNSOPT(n int, owner []byte, ttl uint32) []byte {
	w := make([]byte, 12)
	binary.BigEndian.PutUint16(w[2:], 0x8000)
	binary.BigEndian.PutUint16(w[4:], 1)
	binary.BigEndian.PutUint16(w[10:], uint16(n))
	w = append(w, 0, 0, 1, 0, 1)
	for i := 0; i < n; i++ {
		w = append(w, owner...)
		w = append(w, 0, 41, 4, 208)
		w = binary.BigEndian.AppendUint32(w, ttl)
		w = append(w, 0, 0)
	}
	return w
}
func TestDNSSharedEDNSSemantics(t *testing.T) {
	w := reviewDNSOPT(1, []byte{0}, 0x01008000)
	fields, err := DecodeDNSMessage(w, 32)
	require.NoError(t, err)
	require.Equal(t, uint16(16), fields["RCODE"])
	opt := fields["Additional"].([]map[string]any)[0]
	require.Equal(t, uint16(0x8000), opt["EDNS Flags"])
	require.Equal(t, true, opt["DNSSEC OK"])
	doh, err := (&binDoH{pending: map[uint64]dohPending{}}).attachDNS(map[string]any{}, w, true, 32)
	require.NoError(t, err)
	require.Equal(t, uint16(16), doh["RCODE"])
	dot, err := (&binDoT{pending: map[uint16]string{}}).consume(append([]byte{0, byte(len(w))}, w...), 32)
	require.NoError(t, err)
	require.Equal(t, uint16(16), dot["RCODE"])
	for _, tc := range []struct {
		name string
		wire []byte
	}{
		{"duplicate", reviewDNSOPT(2, []byte{0}, 0)}, {"non-root", reviewDNSOPT(1, []byte{1, 'a', 0}, 0)}, {"compressed-root", reviewDNSOPT(1, []byte{0xc0, 12}, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeDNSMessage(tc.wire, 32)
			require.Error(t, err)
			require.Contains(t, err.Error(), "OPT")
		})
	}
	wrong := reviewDNSOPT(1, []byte{0}, 0)
	binary.BigEndian.PutUint16(wrong[6:], 1)
	binary.BigEndian.PutUint16(wrong[10:], 0)
	_, err = DecodeDNSMessage(wrong, 32)
	require.Error(t, err)
	for _, deferred := range []bool{false, true} {
		for _, port := range []uint16{53, 4053} {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			s.(*captureSession).f.ports = [2]uint16{40000, port}
			s.(*captureSession).f.a.config.Deferred = deferred
			prefix := binary.BigEndian.AppendUint16(nil, uint16(len(w)))
			var events []*ProtocolEvent
			for _, b := range append(prefix, w...) {
				events = append(events, s.Feed(1, time.Unix(1, 0), []byte{b}).Events...)
			}
			require.Len(t, events, 1)
			require.Empty(t, events[0].Error)
			require.Equal(t, uint16(16), events[0].Session["DNS"].(map[string]any)["RCODE"])
			s.Close("test")
		}
	}
}
