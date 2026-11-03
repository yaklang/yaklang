package pcaputil

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTP3TunnelProtectedLateContextUpdatesOwnedHistory(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, cipher := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, late := range []string{"method", "response"} {
				t.Run(fmt.Sprintf("v%x/%s/%s", version, cipher, late), func(t *testing.T) {
					a := earlyRecoveryParser(t)
					q, key := quicConformanceContext(t, version, cipher)
					q.alpn, q.byteLimit = "h3", 65536
					domain := CaptureDomain{Section: 2, Interface: 7, Encapsulation: "/vlan:91"}
					f := earlyRecoveryOwner(a, q, domain)
					history, err := NewProtocolInspector(16, 1<<20)
					require.NoError(t, err)
					packet := uint64(10)
					send := func(dir int, pn, off uint64, wire []byte) *ProtocolEvent {
						packet++
						base := earlyRecoveryBase(f, domain, packet)
						if dir == 1 {
							base.Source, base.Destination = base.Destination, base.Source
						}
						es, ok := a.decodeQUICDatagram(base, quicConformancePacket(t, key, "1-RTT", pn, byte(dir), quicStreamFrame(0, off, false, wire)))
						require.True(t, ok)
						require.Len(t, es, 1)
						a.emit(es[0])
						history.OnEvent(es[0])
						return es[0]
					}
					trailer := h3LiteralFields([2]string{"x-trailer", "y"})
					var old, current *ProtocolEvent
					if late == "method" {
						old = send(1, 0, 0, append(h3ResponseHeaders(), trailer...))
					} else {
						send(0, 0, 0, h3TunnelRequest())
						old = send(0, 1, uint64(len(h3TunnelRequest())), trailer)
					}
					require.Equal(t, "decoded", old.Status)
					frozen, err := old.Decode()
					require.NoError(t, err)
					source := cloneSession(map[string]any{"source": old.SourceBytes})
					ref := old.SourceBytes.PacketRefs[0]
					before := history.PacketQueryRevision(ref, []uint64{old.ID}, true)
					if late == "method" {
						current = send(0, 0, 0, h3TunnelRequest())
					} else {
						current = send(1, 0, 0, h3ResponseHeaders())
					}
					require.Equal(t, "decoded", current.Status)
					require.Empty(t, current.Error)
					facts := h3TunnelFacts(t, current.Session)
					require.Equal(t, old.ID, facts[0]["Frame PDU ID"])
					require.Equal(t, current.ID, facts[0]["Observed In PDU ID"])
					require.Equal(t, current.FlowID, facts[0]["Flow ID"])
					require.NotEqual(t, before, history.PacketQueryRevision(ref, []uint64{old.ID}, true))
					view, err := history.Details(old.ID)
					require.NoError(t, err)
					require.Equal(t, old.SourceBytes, view.SourceBytes)
					if late == "method" {
						require.Equal(t, []any{uint64(0x105)}, view.DisplayFieldValues().FieldMap()["http3.tunnel.error_code"])
					} else {
						require.Empty(t, view.DisplayFieldValues().FieldMap()["http3.tunnel.error_code"])
						require.Equal(t, []any{"order-unverified"}, view.DisplayFieldValues().FieldMap()["http3.tunnel.verdict"])
					}
					require.Equal(t, []any{old.ID}, view.DisplayFieldValues().FieldMap()["http3.tunnel.frame_pdu"])
					require.Equal(t, []any{current.ID}, view.DisplayFieldValues().FieldMap()["http3.tunnel.observed_in_pdu"])
					for _, value := range view.DisplayFieldValues().Values {
						if strings.HasPrefix(value.Name, "http3.tunnel.") {
							require.Nil(t, value.Range)
							require.Equal(t, "context", value.Origin)
						}
					}
					current.Session["HTTP3 Tunnel Observations"].([]map[string]any)[0]["Frame PDU ID"] = uint64(999)
					current.Protocol = "ssdp"
					require.Equal(t, []any{old.ID}, current.DisplayFieldValues().FieldMap()["http3.tunnel.frame_pdu"])
					view.Session["HTTP3 Resolved Tunnel Observations"].([]map[string]any)[0]["Error Code"] = uint64(0)
					if late == "method" {
						require.Equal(t, []any{uint64(0x105)}, view.DisplayFieldValues().FieldMap()["http3.tunnel.error_code"])
					} else {
						require.Empty(t, view.DisplayFieldValues().FieldMap()["http3.tunnel.error_code"])
					}
					after, err := old.Decode()
					require.NoError(t, err)
					require.Equal(t, frozen, after)
					require.Equal(t, source, cloneSession(map[string]any{"source": old.SourceBytes}))
					require.Empty(t, old.DisplayFieldValues().FieldMap()["http3.tunnel.error_code"])
				})
			}
		}
	}
}

func TestHTTP3TunnelBadTagCannotSupplyLateMethod(t *testing.T) {
	a := earlyRecoveryParser(t)
	q, key := quicConformanceContext(t, quicVersion1, quicAEADAES128GCM)
	q.alpn, q.byteLimit = "h3", 65536
	domain := CaptureDomain{Interface: 4, Encapsulation: "/vlan:7"}
	f := earlyRecoveryOwner(a, q, domain)
	base := earlyRecoveryBase(f, domain, 11)
	base.Source, base.Destination = base.Destination, base.Source
	_, ok := a.decodeQUICDatagram(base, quicConformancePacket(t, key, "1-RTT", 0, 1, quicStreamFrame(0, 0, false, append(h3ResponseHeaders(), h3LiteralFields([2]string{"x-trailer", "y"})...))))
	require.True(t, ok)
	wire := quicConformancePacket(t, key, "1-RTT", 0, 0, quicStreamFrame(0, 0, false, h3TunnelRequest()))
	bad := append([]byte(nil), wire...)
	bad[len(bad)-1] ^= 1
	es, _ := a.decodeQUICDatagram(earlyRecoveryBase(f, domain, 12), bad)
	for _, e := range es {
		require.NotContains(t, e.Session, "HTTP3 Tunnel Observations")
	}
	require.Nil(t, q.h3Fault)
	es, ok = a.decodeQUICDatagram(earlyRecoveryBase(f, domain, 13), wire)
	require.True(t, ok)
	require.Len(t, es, 1)
	require.Equal(t, "decoded", es[0].Status)
	require.Equal(t, uint64(0x105), h3TunnelFacts(t, es[0].Session)[0]["Error Code"])
}

func TestHTTP3TunnelPendingTypeReleasedOnResetAndNotReused(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	wire := append(h3ResponseHeaders(), h3LiteralFields([2]string{"x-trailer", "y"})...)
	_, prior, err := h3BodyTestFeed(q, 1, 0, 0, wire, false)
	require.NoError(t, err)
	frozen, _ := json.Marshal(prior)
	before := q.initialMemory()
	for dir := 0; dir < 2; dir++ {
		reset := map[string]any{"Frame Type": "RESET_STREAM", "Stream ID": uint64(0), "Final Size": uint64(0)}
		if dir == 1 {
			reset["Final Size"] = uint64(len(wire))
		}
		require.NoError(t, q.applyFrames(dir, quicSpaceApplication, []map[string]any{reset}, map[string]any{}, 1024))
	}
	require.Less(t, q.initialMemory(), before)
	_, info, err := h3BodyTestFeed(q, 0, 0, 0, h3TunnelRequest(), false)
	require.Error(t, err, "a nonempty request cannot reopen the reset final-size-zero direction")
	require.NotContains(t, info, "HTTP3 Tunnel Observations")
	require.Nil(t, q.h3Fault)
	after, _ := json.Marshal(prior)
	require.Equal(t, frozen, after)
	other := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, _, err = h3BodyTestFeed(other, 0, 0, 0, h3TunnelRequest(), false)
	require.NoError(t, err)
	_, info, err = h3BodyTestFeed(other, 1, 0, 0, append(h3ResponseHeaders(), h3Data([]byte("legal"))...), true)
	require.NoError(t, err)
	require.NotContains(t, info, "HTTP3 Connection Error")
}
