package pcaputil

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func nf9NativeSet(id uint16, w []byte) []byte {
	header := binary.BigEndian.AppendUint16(nil, id)
	header = binary.BigEndian.AppendUint16(header, uint16(4+len(w)))
	return append(header, w...)
}

func TestNetFlowV9EarlierDatagramDecoderCannotHideTupleReuse(t *testing.T) {
	var events []*ProtocolEvent
	a, err := NewPacketAnalyzer(WithCaptureReplayClock(), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
	require.NoError(t, err)
	defer a.Close()
	feed := func(payload []byte, second int64) {
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("198.51.100.2")}
		udp := &layers.UDP{SrcPort: 52000, DstPort: 2055}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		buffer := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(payload)))
		frame := buffer.Bytes()
		require.NoError(t, a.Feed(frame, gopacket.CaptureInfo{Timestamp: time.Unix(second, 0), CaptureLength: len(frame), Length: len(frame)}, layers.LinkTypeEthernet))
	}
	feed(nf9NativeMessage(1, 1, 1000, 19, nf9NativeTemplate()), 1)
	feed(mavlinkTestFrame(1, 0, []byte{0, 0, 0, 0, 2, 3, 0, 4, 3}, 0, 50), 2)
	feed(nf9NativeMessage(1, 2, 1001, 19, nf9NativeData()), 3)
	require.Len(t, events, 3)
	require.Equal(t, "mavlink", events[1].Protocol, "an earlier successful decoder must be exercised")
	require.Equal(t, "netflow-v9", events[2].Protocol)
	require.Equal(t, "context-required", events[2].Status, "a codec switch on the same UDP tuple retires old templates before dispatch")
}

func nf9NativeMessage(count uint16, seq, uptime, domain uint32, sets ...[]byte) []byte {
	w := binary.BigEndian.AppendUint16(nil, 9)
	w = binary.BigEndian.AppendUint16(w, count)
	w = binary.BigEndian.AppendUint32(w, uptime)
	w = binary.BigEndian.AppendUint32(w, 1700000000)
	w = binary.BigEndian.AppendUint32(w, seq)
	w = binary.BigEndian.AppendUint32(w, domain)
	for _, s := range sets {
		w = append(w, s...)
	}
	return w
}

func nf9NativeTemplate() []byte {
	return nf9NativeSet(0, []byte{1, 0, 0, 2, 0, 8, 0, 4, 0, 2, 0, 4})
}

func nf9NativeData() []byte {
	return nf9NativeSet(256, []byte{192, 0, 2, 9, 0, 0, 0, 17})
}

func nf9NativeParser(t *testing.T) *binParser {
	t.Helper()
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	t.Cleanup(c.binParser.closeUDPSessions)
	return c.binParser
}

func nf9NativeObserve(t *testing.T, a *binParser, w []byte, now int64, mutate func(*ProtocolEvent)) *ProtocolEvent {
	t.Helper()
	e := &ProtocolEvent{Transport: "udp", Source: "192.0.2.1:52000", Destination: "198.51.100.2:2055", Timestamp: time.Unix(now, 0),
		SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{{Number: uint64(now)}}}}
	if mutate != nil {
		mutate(e)
	}
	require.True(t, a.decodeNetFlowV9Datagram(e, w, 52000, 2055, ""))
	a.emit(e)
	require.Equal(t, "netflow-v9", e.Protocol)
	require.Zero(t, e.FlowID, "reported records are not captured connections")
	require.Zero(t, e.ResponseTo)
	require.Equal(t, len(w), e.Length)
	return e
}

func TestNetFlowV9NativeSequenceLossReorderReuseAndRecovery(t *testing.T) {
	a := nf9NativeParser(t)
	observe := func(seq uint32, sets ...[]byte) *ProtocolEvent {
		return nf9NativeObserve(t, a, nf9NativeMessage(1, seq, 1000+seq, 19, sets...), int64(seq)+1, nil)
	}
	require.Equal(t, "decoded", observe(10, nf9NativeTemplate()).Status)
	good := observe(11, nf9NativeData())
	require.Equal(t, "decoded", good.Status)
	require.Equal(t, true, good.Session["Record Count Verified"])
	values := good.DisplayFields()
	require.Equal(t, []any{"192.0.2.9"}, values["cflow.srcaddr"])
	require.Equal(t, []any{uint64(17)}, values["cflow.packets"])
	row := netflowV9Rows(good.Session["FlowSets"])[0]
	require.Equal(t, "observed caller-supplied context", row["Template Scope"])
	require.Equal(t, uint64(11), row["Template Packet References"].([]map[string]any)[0]["Packet"])
	for _, seq := range []uint32{13, 12, 14} {
		e := observe(seq, nf9NativeData())
		require.Equal(t, "context-required", e.Status)
		require.Equal(t, "context-required", e.Completeness)
		require.True(t, e.DisplayFieldValues().Incomplete)
		fields, err := e.GetFields()
		require.NoError(t, err, "known partial header remains queryable")
		require.Equal(t, true, fields["Fixed Header Complete"])
		require.Equal(t, false, fields["Record Count Verified"])
		require.NotContains(t, netflowV9Rows(fields["FlowSets"])[0], "Records")
	}
	require.Equal(t, "decoded", observe(15, nf9NativeTemplate()).Status)
	require.Equal(t, "decoded", observe(16, nf9NativeData()).Status)
	// A reused sequence with different bytes is an independent observation,
	// not permission to borrow its previous schema.
	bad := observe(16, nf9NativeData())
	require.Equal(t, "decoded", bad.Status, "an identical immediate duplicate uses original input context")
	require.Contains(t, bad.Session["Duplicate Observation"], "no lease refresh")
	w := nf9NativeMessage(1, 16, 1016, 19, nf9NativeData())
	w[len(w)-1] = 99
	e := nf9NativeObserve(t, a, w, 18, nil)
	require.Equal(t, "context-required", e.Status)
	require.Equal(t, "decoded", observe(17, nf9NativeTemplate()).Status)
	require.Equal(t, "decoded", observe(18, nf9NativeData()).Status)
	badInput := &ProtocolEvent{Transport: "udp", Source: good.Source, Destination: good.Destination, Timestamp: time.Unix(50, 0)}
	require.False(t, a.decodeNetFlowV9Datagram(badInput, []byte("unrelated protocol reusing the collector port"), 52000, 2055, ""))
	require.Equal(t, "context-required", observe(19, nf9NativeData()).Status)
	good.Session["Record Count Verified"] = false
	netflowV9Rows(netflowV9Rows(good.Session["FlowSets"])[0]["Records"])[0]["Fields"].([]map[string]any)[0]["Value"] = "203.0.113.200"
	fields, err := good.GetFields()
	require.NoError(t, err)
	require.Equal(t, true, fields["Record Count Verified"], "public fields cannot rewrite retained protocol evidence")
	require.Equal(t, values, good.DisplayFields(), "typed queries use the same owned accepted values")
	a.closeUDPSessions()
	require.Zero(t, a.stats().BufferedBytes)
}

func TestNetFlowV9NativeCaptureDomainExporterAndCollectorIsolation(t *testing.T) {
	a := nf9NativeParser(t)
	nf9NativeObserve(t, a, nf9NativeMessage(1, 1, 1000, 19, nf9NativeTemplate()), 1, nil)
	for _, mutate := range []func(*ProtocolEvent){
		func(e *ProtocolEvent) { e.Source = "192.0.2.3:52000" },
		func(e *ProtocolEvent) { e.Source = "192.0.2.1:52001" },
		func(e *ProtocolEvent) { e.Destination = "198.51.100.3:2055" },
		func(e *ProtocolEvent) { e.Destination = "198.51.100.2:9995" },
		func(e *ProtocolEvent) { e.Domain.Interface = 1 },
		func(e *ProtocolEvent) { e.Domain.Section = 1 },
		func(e *ProtocolEvent) { e.Domain.Encapsulation = "/vlan:19" },
	} {
		e := nf9NativeObserve(t, a, nf9NativeMessage(1, 2, 1001, 19, nf9NativeData()), 2, mutate)
		require.Equal(t, "context-required", e.Status)
	}
	e := nf9NativeObserve(t, a, nf9NativeMessage(1, 2, 1001, 20, nf9NativeData()), 2, nil)
	require.Equal(t, "context-required", e.Status)
	// Foreign observation time must not age this exporter's schema.
	nf9NativeObserve(t, a, nf9NativeMessage(1, 1, 1000, 22, nf9NativeTemplate()), 86400, nil)
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 2, 1001, 19, nf9NativeData()), 2, nil)
	require.Equal(t, "decoded", e.Status)
}

func TestNetFlowV9NativeLeaseDoesNotRefreshOnDuplicatesOrData(t *testing.T) {
	a := nf9NativeParser(t)
	definition := nf9NativeMessage(1, 10, 1000, 19, nf9NativeTemplate())
	nf9NativeObserve(t, a, definition, 1, nil)
	duplicate := nf9NativeObserve(t, a, definition, 250, nil)
	require.Contains(t, duplicate.Session["Duplicate Observation"], "no lease refresh")
	e := nf9NativeObserve(t, a, nf9NativeMessage(1, 11, 1001, 19, nf9NativeData()), 300, nil)
	require.Equal(t, "decoded", e.Status)
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 12, 1002, 19, nf9NativeData()), 301, nil)
	require.Equal(t, "context-required", e.Status, "only template observations refresh schema leases")
	nf9NativeObserve(t, a, nf9NativeMessage(1, 13, 1003, 19, nf9NativeTemplate()), 302, nil)
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 14, 1004, 19, nf9NativeData()), 303, nil)
	require.Equal(t, "decoded", e.Status)
}

func TestNetFlowV9NativeBudgetAndUptimeWrap(t *testing.T) {
	a := nf9NativeParser(t)
	nf9NativeObserve(t, a, nf9NativeMessage(1, ^uint32(0), ^uint32(0)-15, 19, nf9NativeTemplate()), 1, nil)
	e := nf9NativeObserve(t, a, nf9NativeMessage(1, 0, 16, 19, nf9NativeData()), 2, nil)
	require.Equal(t, "decoded", e.Status, "sequence and uptime wrap are independent modulo32 observations")
	refusal := &ProtocolEvent{Transport: "udp", Source: e.Source, Destination: e.Destination}
	oversize := nf9NativeMessage(1, 1, 17, 19, nf9NativeTemplate())
	oversize = append(oversize, make([]byte, 4096)...)
	require.True(t, a.refuseNetFlowV9Oversize(refusal, oversize, 52000, 2055))
	require.Equal(t, len(oversize), refusal.Length)
	require.Empty(t, refusal.Raw)
	require.Equal(t, "limited", refusal.Status)
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 1, 17, 19, nf9NativeData()), 3, nil)
	require.Equal(t, "context-required", e.Status)
	a.config.MaxBufferedBytes = 4096
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 2, 18, 19, nf9NativeTemplate()), 4, nil)
	require.Equal(t, "decoded", e.Status, "wire observations do not become malformed on cache refusal")
	require.Contains(t, e.Session["Context Retention Warning"], "budget exceeded")
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 3, 19, 19, nf9NativeData()), 5, nil)
	require.Equal(t, "context-required", e.Status)
	a.closeUDPSessions()
	require.Zero(t, a.stats().BufferedBytes)
}

func TestNetFlowV9NativeRedefinitionAndFailedUpdateDoNotBorrowOldFields(t *testing.T) {
	a := nf9NativeParser(t)
	nf9NativeObserve(t, a, nf9NativeMessage(1, 1, 1000, 19, nf9NativeTemplate()), 1, nil)
	replacement := nf9NativeSet(0, []byte{1, 0, 0, 2, 0, 12, 0, 4, 0, 2, 0, 4})
	badUpdate := nf9NativeMessage(2, 2, 1001, 19, replacement, nf9NativeSet(256, []byte{1}))
	bad := nf9NativeObserve(t, a, badUpdate, 2, nil)
	require.Equal(t, "context-required", bad.Status)
	e := nf9NativeObserve(t, a, nf9NativeMessage(1, 3, 1002, 19, nf9NativeData()), 3, nil)
	require.Equal(t, "context-required", e.Status, "neither a rejected update nor the retired original may survive")
	nf9NativeObserve(t, a, nf9NativeMessage(1, 4, 1003, 19, replacement), 4, nil)
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 5, 1004, 19, nf9NativeData()), 5, nil)
	require.Equal(t, "decoded", e.Status)
	require.NotContains(t, e.DisplayFields(), "cflow.srcaddr")
	require.Equal(t, []any{"192.0.2.9"}, e.DisplayFields()["cflow.dstaddr"])
	require.Equal(t, "192.0.2.1:52000", e.Source, "reported record addresses do not rewrite captured endpoints")
}

func TestNetFlowV9NativeClockAndOptionsScopeRemainExplicit(t *testing.T) {
	a := nf9NativeParser(t)
	e := nf9NativeObserve(t, a, nf9NativeMessage(1, 1, 1000, 19, nf9NativeTemplate()), 1, func(e *ProtocolEvent) { e.Timestamp = time.Time{} })
	require.Contains(t, e.Session["Context Retention Warning"], "clock unavailable")
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 2, 1001, 19, nf9NativeData()), 2, nil)
	require.Equal(t, "context-required", e.Status)
	f, err := CompileDisplayFilter("cflow.version == 9")
	require.NoError(t, err)
	result := f.EvaluateEvents([]*ProtocolEvent{e})
	require.True(t, result.Matched)
	require.True(t, result.Complete)
	require.True(t, result.Partial)
	f, err = CompileDisplayFilter("not cflow.srcaddr")
	require.NoError(t, err)
	require.False(t, f.EvaluateEvents([]*ProtocolEvent{e}).Complete)
	nf9NativeObserve(t, a, nf9NativeMessage(1, 3, 1002, 19, nf9NativeTemplate()), 3, nil)
	e = nf9NativeObserve(t, a, nf9NativeMessage(1, 4, 900, 19, nf9NativeData()), 4, nil)
	require.Equal(t, "context-required", e.Status)
	require.Contains(t, e.Session["Context Reset"], "restart not authenticated")
	options := nf9NativeSet(1, []byte{1, 44, 0, 4, 0, 4, 0, 1, 0, 0, 0, 2, 0, 4})
	e = nf9NativeObserve(t, a, nf9NativeMessage(2, 5, 901, 19, options, nf9NativeSet(300, []byte{0, 0, 0, 43})), 5, nil)
	require.Equal(t, "decoded", e.Status)
	values := e.DisplayFields()
	require.Equal(t, []any{uint64(1)}, values["cflow.scope_field_type"])
	require.Equal(t, []any{uint64(2)}, values["cflow.template_field_type"])
	require.Equal(t, []any{uint64(0), uint64(4)}, values["cflow.template_field_length"])
	require.NotContains(t, values, "cflow.octets", "scope type1 is not ordinary information element1")
	require.Equal(t, []any{uint64(43)}, values["cflow.packets"])
}

func TestNetFlowV9ReportedFlowIDAndDirectionAreNotCapturedSessionFacts(t *testing.T) {
	a := nf9NativeParser(t)
	template := nf9NativeSet(0, []byte{1, 0, 0, 3, 0, 61, 0, 1, 0, 148, 0, 8, 0, 233, 0, 1})
	data := nf9NativeSet(256, []byte{1, 0x10, 0, 0, 0, 0, 0, 0, 3, 3})
	e := nf9NativeObserve(t, a, nf9NativeMessage(2, 1, 1000, 19, template, data), 1, nil)
	require.Equal(t, "decoded", e.Status)
	require.False(t, e.DisplayFieldValues().Incomplete)
	values := e.DisplayFields()
	require.Equal(t, []any{uint64(1)}, values["cflow.direction"])
	require.Equal(t, []any{uint64(1)<<60 | 3}, values["cflow.flow_id"])
	require.Equal(t, []any{uint64(3)}, values["cflow.firewall_event"])
	require.Equal(t, []any{uint64(0)}, values["flow.id"])
	require.Zero(t, e.Direction)
	fields := netflowV9Rows(netflowV9Rows(e.Session["FlowSets"])[1]["Records"])[0]["Fields"].([]map[string]any)
	require.Contains(t, fields[0]["Value Meaning"], "not captured connection direction")
	require.Contains(t, fields[1]["Value Meaning"], "not a captured TCP flow")
	require.Equal(t, "Flow Denied (reported)", fields[2]["Value Meaning"])
	for _, v := range e.DisplayFieldValues().Values {
		if v.Name == "cflow.flow_id" {
			require.Equal(t, "1152921504606846979", v.ExactValue)
			require.Equal(t, &DisplayFieldRange{Offset: 45, Length: 8}, v.Range)
		}
	}
}
