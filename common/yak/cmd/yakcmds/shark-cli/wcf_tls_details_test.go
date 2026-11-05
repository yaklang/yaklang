package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func wcfTLSDisplayTestSession(t *testing.T) pcaputil.ProtocolSession {
	t.Helper()
	s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	via := []byte("net.tcp://example.test/owned")
	wire := append([]byte{0, 1, 0, 1, 2, 2, byte(len(via))}, via...)
	wire = append(wire, 3, 7, 9, 19)
	wire = append(wire, []byte("application/ssl-tls")...)
	require.Nil(t, s.Feed(0, time.Unix(1, 0), wire).Err)
	require.Nil(t, s.Feed(1, time.Unix(2, 0), []byte{10}).Err)
	return s
}

func TestWCFTLSDisplayKeepsNativeCarrierAndRecordScopes(t *testing.T) {
	s := wcfTLSDisplayTestSession(t)
	raw := []byte{23, 3, 3, 0, 3, 1, 2, 3}
	r := s.Feed(0, time.Unix(3, 0), raw)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.Protocol, e.Status, e.Summary = "grpc", "decoded", "forged"
	e.CarrierLayers = []string{"HTTP/3"}
	e.Fields = map[string]any{"GRPC Messages": []any{[]byte("forged")}}
	e.Session = map[string]any{"Authentication Verified": true, "Observed WCF Carrier": map[string]any{"Observed Session Index": 999, "Authenticated Plaintext": []byte("forged")}}
	require.Equal(t, []string{"WCF/MC-NMF"}, eventCarriers(e))
	projected := protocolDisplayEvent(e)
	require.Equal(t, "tls", projected.Protocol)
	require.Equal(t, uint64(1), projected.Session["Observed WCF Carrier"].(map[string]any)["Observed Session Index"])
	local, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for _, got := range []packetDetail{{fields: local, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
		require.Len(t, got.sources, 1)
		require.Equal(t, raw, got.sources[0].Data)
		inContext, seen, header := false, false, false
		for _, f := range got.fields {
			if strings.HasPrefix(f.text, "Observed WCF upgrade ·") {
				inContext, seen = true, true
			} else if inContext && f.depth == 0 {
				inContext = false
			}
			if inContext {
				require.False(t, f.hasRange, f.text)
				require.Zero(t, f.source, f.text)
			}
			if strings.HasPrefix(f.text, "Record Version:") {
				header = true
				require.True(t, f.hasRange)
				require.Equal(t, []byte{3, 3}, got.sources[f.source-1].Data[f.start:f.end])
			}
		}
		require.True(t, seen)
		require.True(t, header)
		require.NotContains(t, detailLines(got.fields), "forged")
		require.NotContains(t, detailLines(got.fields), "999")
		require.NotContains(t, detailLines(got.fields), "TLS? candidate record")
		require.Contains(t, detailLines(got.fields), "TLS · PDU #")
	}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "TLS", row.Protocol)
	require.NotContains(t, row.Info, "forged")
	encoded, err := json.Marshal(projected)
	require.NoError(t, err)
	var dto pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &dto))
	_, ok := dto.NativeWCFTLSCarrier()
	require.False(t, ok)
	publicRow := presentEvents([]*pcaputil.ProtocolEvent{&dto}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "TLS?", publicRow.Protocol, "a display DTO cannot manufacture native carrier identity")
}

func TestWCFTLSDisplayChangedRawCannotReuseRecordRanges(t *testing.T) {
	s := wcfTLSDisplayTestSession(t)
	r := s.Feed(0, time.Unix(3, 0), []byte{23, 3, 3, 0, 3, 1, 2, 3})
	require.Nil(t, r.Err)
	e := r.Events[0]
	e.Raw[0] ^= 0xff
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.Empty(t, sources)
	for _, f := range fields {
		require.False(t, f.hasRange, f.text)
	}
	require.Contains(t, detailLines(fields), "owned Raw changed")
}

func TestWCFTLSIncompleteUpgradeKeepsContextWithoutPDUClaim(t *testing.T) {
	s := wcfTLSDisplayTestSession(t)
	r := s.Feed(0, time.Unix(3, 0), []byte{22, 3})
	require.True(t, r.NeedMore)
	events := s.Close("FIN")
	require.Len(t, events, 1)
	e := events[0]
	observation, ok := e.NativeWCFTLSCarrier()
	require.True(t, ok)
	require.Equal(t, "incomplete", observation.Status)
	row := presentEvents(events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "TLS", row.Protocol)
	fields, sources := protocolEventDetail(events)
	for _, got := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, events, false), workerMetadataDetail(t, events, true)} {
		text := detailLines(got.fields)
		require.Contains(t, text, "observation #")
		require.Contains(t, text, "no complete record fields")
		require.NotContains(t, text, "complete record header/body")
		require.NotContains(t, text, "TLS record and observed context")
		require.NotContains(t, text, "TLS · PDU #")
		for _, source := range got.sources {
			require.Contains(t, source.Name, "stream bytes")
			require.NotContains(t, source.Name, "PDU")
		}
	}
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	var dto pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &dto))
	_, ok = dto.NativeWCFTLSCarrier()
	require.False(t, ok)
}

func TestWCFTLSLateDiagnosticDoesNotKeepFrozenVersionOrEvictionNotice(t *testing.T) {
	s := wcfTLSDisplayTestSession(t)
	require.True(t, s.Feed(0, time.Unix(3, 0), []byte{22, 3}).NeedMore)
	events := s.Close("FIN")
	require.Len(t, events, 1)
	history, err := pcaputil.NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	history.OnEvent(events[0])
	retained := retainedProtocolDetail(history, events[0].ID)
	require.NotNil(t, retained)
	require.NotEqual(t, "detail-unavailable", retained.Completeness)
	_, err = retained.GetFields()
	require.Error(t, err, "retaining context cannot grant record-query admission")
	for _, tc := range []struct{ name, caption string }{{"known-version", "TLSv1.2"}, {"no-capture-event", ""}, {"prior-framing", "WCF/MC-NMF"}} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &capturedPacket{number: 1, events: events, presentation: packetPresentation{protocol: tc.caption}}
			local := detail(raw)
			require.Equal(t, "TLS", local.protocol, "retained native diagnosis also replaces the in-process detail caption")
			request, err := marshalDecodeRequest(packetDecodeRequest(raw))
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
			detail, err := decodeWorkerOutput(output.Bytes(), raw)
			require.NoError(t, err)
			require.Equal(t, "TLS", detail.protocol, "late native framing diagnostic supersedes the original version caption")
			require.Contains(t, detailLines(detail.fields), "no complete record fields")
			require.NotContains(t, detailLines(detail.fields), "complete record header/body")
		})
	}
}

func TestWCFTLSConnectionCacheUsesCurrentNativeFramingEpoch(t *testing.T) {
	session := wcfTLSDisplayTestSession(t)
	store := newStreamStore(8, 4096)
	defer store.finish()
	first := tcpPacket(t, 1, 100, "", false, true, false, false)
	store.add(first)
	payload := tcpPacket(t, 2, 101, "owned", false, false, false, false)
	store.add(payload)
	base := trustedStreamEvent(payload, "wcf")
	route := func(events []*pcaputil.ProtocolEvent) {
		for _, e := range events {
			e.Source, e.Destination = base.Source, base.Destination
			e.Transport = "tcp"
			e.Domain = base.Domain
			e.SourceBytes.PacketRefs = base.SourceBytes.PacketRefs
			store.observeProtocol(e)
		}
	}
	record := func(typ byte, body []byte) []byte {
		return append([]byte{typ, 3, 3, byte(len(body) >> 8), byte(len(body))}, body...)
	}
	hello := func(client bool) []byte {
		b := append([]byte{3, 3}, bytes.Repeat([]byte{1}, 32)...)
		b = append(b, 0)
		typ := byte(2)
		if client {
			typ = 1
			b = append(b, 0, 2, 0, 156, 1, 0)
		} else {
			b = append(b, 0, 156, 0)
		}
		b = append(b, 0, 0)
		hs := append([]byte{typ, 0, 0, byte(len(b))}, b...)
		return record(22, hs)
	}
	var oldTLS *pcaputil.ProtocolEvent
	for _, step := range []struct {
		dir  int
		wire []byte
	}{{0, hello(true)}, {1, hello(false)}, {0, record(23, bytes.Repeat([]byte{3}, 32))}, {1, record(23, bytes.Repeat([]byte{4}, 32))}} {
		r := session.Feed(step.dir, time.Unix(4, 0), step.wire)
		require.Nil(t, r.Err)
		route(r.Events)
		if step.dir == 1 {
			oldTLS = r.Events[0]
		}
	}
	old := store.snapshot(first.streamID)
	require.Equal(t, "TLSv1.2", old.protocol)
	// Simulate the retained previous application's independently admitted
	// identity. The reset policy must discard it only after a new native epoch.
	prior := store.records[first.streamID]
	prior.protocol = "gRPC"
	prior.protocolStack = append(prior.protocolStack, "HTTP/2", "gRPC")
	via := []byte("net.tcp://example.test/owned")
	prefix := append([]byte{0, 1, 0, 1, 2, 2, byte(len(via))}, via...)
	prefix = append(prefix, 3, 7, 9, 19)
	prefix = append(prefix, []byte("application/ssl-tls")...)
	r := session.Feed(0, time.Unix(5, 0), prefix)
	require.True(t, r.NeedMore)
	require.Empty(t, r.Events)
	r = session.Feed(1, time.Unix(6, 0), []byte{10})
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 6)
	// Wrong capture domains and physical generations cannot consume the epoch.
	candidate := r.Events[0]
	savedDomain := candidate.Domain
	candidate.Domain.Interface++
	candidate.Source, candidate.Destination = base.Source, base.Destination
	candidate.SourceBytes.PacketRefs = base.SourceBytes.PacketRefs
	store.observeProtocol(candidate)
	require.Equal(t, "gRPC", store.snapshot(first.streamID).protocol)
	candidate.Domain = savedDomain
	prior.nativeFlowID = 99
	candidate.TCPFlowID = 100
	store.observeProtocol(candidate)
	require.Equal(t, "gRPC", store.snapshot(first.streamID).protocol)
	prior.nativeFlowID = 0
	candidate.TCPFlowID = 0
	route(r.Events)
	newFraming := store.snapshot(first.streamID)
	require.Equal(t, "WCF/MC-NMF", newFraming.protocol)
	require.Equal(t, []string{"WCF/MC-NMF"}, newFraming.protocolStack, "previous TLS version/application cannot describe the new clear framing epoch")
	r = session.Feed(0, time.Unix(7, 0), hello(true))
	require.Nil(t, r.Err)
	route(r.Events)
	newTLS := store.snapshot(first.streamID)
	require.Equal(t, "TLS", newTLS.protocol, "new ClientHello has not established the new version")
	require.Equal(t, []string{"WCF/MC-NMF", "TLS"}, newTLS.protocolStack)
	store.observeProtocol(oldTLS)
	require.Equal(t, "TLS", store.snapshot(first.streamID).protocol)
	require.Equal(t, []string{"WCF/MC-NMF", "TLS"}, store.snapshot(first.streamID).protocolStack)
	require.Equal(t, uint64(2), prior.wcfFramingEpoch)
	// A cache first seeing a native diagnostic still retains the known TLS
	// upgrade identity, without treating the refused message as decoded.
	r = session.Feed(0, time.Unix(8, 0), hello(true))
	require.NotNil(t, r.Err)
	require.Len(t, r.Events, 1)
	fresh := newStreamStore(8, 4096)
	defer fresh.finish()
	opening := tcpPacket(t, 1, 100, "", false, true, false, false)
	fresh.add(opening)
	arrival := tcpPacket(t, 2, 101, "owned", false, false, false, false)
	fresh.add(arrival)
	diagnostic := r.Events[0]
	diagnostic.Source, diagnostic.Destination, diagnostic.Domain = base.Source, base.Destination, base.Domain
	diagnostic.SourceBytes.PacketRefs = base.SourceBytes.PacketRefs
	fresh.observeProtocol(diagnostic)
	require.Equal(t, "TLS", fresh.snapshot(opening.streamID).protocol)
	require.Equal(t, []string{"WCF/MC-NMF", "TLS"}, fresh.snapshot(opening.streamID).protocolStack)
	require.NotEmpty(t, fresh.records[opening.streamID].decodeDiagnostic)
	require.Equal(t, []string{"WCF/MC-NMF", "TLSv1.2"}, old.protocolStack, "published snapshot cannot be rewritten by the later epoch")
}
