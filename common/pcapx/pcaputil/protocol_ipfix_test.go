package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ipfixNativeMessage(seq uint32, sets ...[]byte) []byte {
	w := []byte{0, 10, 0, 0}
	w = binary.BigEndian.AppendUint32(w, 1700000000)
	w = binary.BigEndian.AppendUint32(w, seq)
	w = binary.BigEndian.AppendUint32(w, 19)
	for _, s := range sets {
		w = append(w, s...)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	return w
}
func ipfixNativeTemplate() []byte { return nf9NativeSet(2, []byte{1, 0, 0, 2, 0, 8, 0, 4, 0, 2, 0, 4}) }
func ipfixObserve(t *testing.T, a *binParser, w []byte, now int64, mutate func(*ProtocolEvent)) *ProtocolEvent {
	e := &ProtocolEvent{Transport: "udp", Source: "192.0.2.1:52000", Destination: "198.51.100.2:4739", Timestamp: time.Unix(now, 0), SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: uint64(now)}}}}
	if mutate != nil {
		mutate(e)
	}
	require.True(t, a.decodeIPFIXDatagram(e, w, 52000, 4739, ""))
	a.emit(e)
	require.Equal(t, "ipfix", e.Protocol)
	require.Zero(t, e.FlowID)
	return e
}
func TestIPFIXRecordSequenceLossReorderReuseAndRecovery(t *testing.T) {
	a := nf9NativeParser(t)
	observe := func(seq uint32, now int64, sets ...[]byte) *ProtocolEvent {
		return ipfixObserve(t, a, ipfixNativeMessage(seq, sets...), now, nil)
	}
	template := observe(603, 1, ipfixNativeTemplate())
	require.Equal(t, "decoded", template.Status)
	good := observe(603, 2, nf9NativeData())
	require.Equal(t, "decoded", good.Status, "template-only export MUST NOT increment sequence")
	require.Equal(t, []any{uint64(17)}, good.DisplayFields()["cflow.packets"])
	require.Equal(t, []any{"192.0.2.9"}, good.DisplayFields()["cflow.srcaddr"])
	require.Equal(t, []any{uint64(19)}, good.DisplayFields()["cflow.od_id"])
	require.NotContains(t, good.DisplayFields(), "cflow.source_id")
	require.NotContains(t, good.DisplayFields(), "cflow.count")
	duplicate := observe(603, 3, nf9NativeData())
	require.Equal(t, "decoded", duplicate.Status)
	require.Contains(t, duplicate.Session, "Duplicate Observation")
	gap := observe(605, 4, nf9NativeData())
	require.Equal(t, "context-required", gap.Status)
	require.NotContains(t, gap.DisplayFields(), "cflow.srcaddr")
	require.Equal(t, "decoded", observe(605, 5, ipfixNativeTemplate(), nf9NativeData()).Status)
	require.Equal(t, "decoded", observe(606, 6, nf9NativeData()).Status)
	require.Equal(t, "context-required", observe(605, 7, nf9NativeData()).Status, "reordered data cannot borrow latest schema")
	require.Equal(t, "decoded", observe(0, 8, ipfixNativeTemplate()).Status, "new complete template recovers")
	require.Equal(t, "decoded", observe(0, 9, nf9NativeData()).Status)
	a.observeIPFIXBoundary(&ProtocolEvent{Source: good.Source, Destination: good.Destination}, []byte("unrelated"), false)
	require.Equal(t, "context-required", observe(1, 10, nf9NativeData()).Status)
}
func TestIPFIXDomainDirectionLeaseAndCarrierIsolation(t *testing.T) {
	for _, kind := range []string{"domain", "direction", "source-port", "capture-domain", "expired", "backward-clock", "bad-carrier"} {
		t.Run(kind, func(t *testing.T) {
			a := nf9NativeParser(t)
			ipfixObserve(t, a, ipfixNativeMessage(0, ipfixNativeTemplate()), 1, nil)
			w, now := ipfixNativeMessage(0, nf9NativeData()), int64(2)
			mutate := func(e *ProtocolEvent) {
				switch kind {
				case "domain":
					binary.BigEndian.PutUint32(w[12:], 20)
				case "direction":
					e.Source, e.Destination = e.Destination, e.Source
				case "source-port":
					e.Source = "192.0.2.1:52001"
				case "capture-domain":
					e.Domain.Interface = 1
				case "expired":
					now = 302
					e.Timestamp = time.Unix(now, 0)
				case "backward-clock":
					e.Timestamp = time.Unix(0, 0)
				case "bad-carrier":
					a.observeIPFIXBoundary(e, w, true)
				}
			}
			e := ipfixObserve(t, a, w, now, mutate)
			require.Equal(t, "context-required", e.Status)
			require.NotContains(t, e.DisplayFields(), "cflow.packets")
		})
	}
}
func TestIPFIXEnterpriseScopeDoesNotBecomeStandardReportedAlias(t *testing.T) {
	a := nf9NativeParser(t)
	template := nf9NativeSet(3, []byte{1, 0, 0, 2, 0, 1, 0, 8, 0, 4, 0x80, 8, 0, 4, 0, 0, 0, 0})
	e := ipfixObserve(t, a, ipfixNativeMessage(0, template, nf9NativeSet(256, []byte{192, 0, 2, 1, 198, 51, 100, 1})), 1, nil)
	require.Equal(t, "decoded", e.Status)
	require.NotContains(t, e.DisplayFields(), "cflow.srcaddr")
	require.Equal(t, "limited", e.Completeness)
}
func TestIPFIXFailedGrammarNeverAdmitsOnlyVersionAndPort(t *testing.T) {
	a := nf9NativeParser(t)
	for _, w := range [][]byte{[]byte{0, 10}, ipfixNativeMessage(0, []byte{0, 2, 255, 255}), ipfixNativeMessage(0, nf9NativeSet(2, []byte{1, 0, 0, 1, 0x80, 8, 0, 4}))} {
		require.False(t, a.decodeIPFIXDatagram(&ProtocolEvent{}, w, 52000, 4739, ""))
		e := &ProtocolEvent{}
		require.True(t, a.decodeIPFIXDatagram(e, w, 52000, 4739, "ipfix"))
		a.emit(e)
		require.NotEqual(t, "decoded", e.Status)
	}
}
