package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestHTTP3WireObservationCaptionSurvivesDetailEvictionAndWorker(t *testing.T) {
	packets, _ := top30Replay(t, top30Cases()[7])
	for _, p := range packets {
		for _, native := range p.protocolEvents() {
			if native.AuthenticatedQUICApplicationProtocol() != "http3" {
				continue
			}
			native.Session["HTTP3 Wire Only"] = true
			native.Session["HTTP3 Wire Observation"] = "retransmission"
			frozen := presentEvents([]*pcaputil.ProtocolEvent{native})
			require.Equal(t, "authenticated STREAM observation", frozen.scope)
			history, err := pcaputil.NewProtocolInspector(1, 64<<20)
			require.NoError(t, err)
			history.OnEvent(native)
			ref := pcaputil.PacketReference{Number: p.number, Domain: pcaputil.CapturePacketReference(p.ci).Domain}
			for _, r := range native.SourceBytes.PacketRefs {
				if r.Number == p.number {
					ref = r
					break
				}
			}
			raw := &capturedPacket{number: p.number, data: p.data, ci: p.ci, link: p.link,
				history: history, eventIDs: []uint64{native.ID}, reference: ref, physicalRecord: true, presentation: frozen}
			require.Equal(t, "HTTP/3", summarize(raw).Protocol)
			otherRef := ref
			otherRef.Number += 100000
			history.OnEvent(&pcaputil.ProtocolEvent{ID: native.ID + 100000, SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{otherRef}}})
			_, err = history.Details(native.ID)
			require.Error(t, err, "this test must actually lose the original native details")
			for _, got := range []packetDetail{quickDetail(raw), detail(raw), workerMetadataDetailForPacket(t, raw)} {
				require.Equal(t, "HTTP/3", got.protocol)
				require.True(t, got.unavailable, "caption state is distinct from the observed protocol name")
				require.Empty(t, got.sources, "retained identity cannot restore ciphertext or authenticated plaintext sources")
				text := detailLines(got.fields)
				require.Contains(t, text, "detail unavailable")
				require.Contains(t, text, "Protocol detail absent or evicted")
				require.NotContains(t, text, "PDU #0")
				require.NotContains(t, text, "MESSAGE ·")
				require.NotContains(t, text, "Headers:")
				require.NotContains(t, text, "Key Update Steps:")
			}
			return
		}
	}
	t.Fatal("no sealed native HTTP3 fixture event")
}

func TestEvictedDetailMarkersAreContextNotProtocolOrByteSources(t *testing.T) {
	reason := "Protocol detail absent or evicted from bounded message history"
	coverage := "Packet contributor detail coverage incomplete; omitted fields are unavailable"
	events := []*pcaputil.ProtocolEvent{
		{ID: 299, Protocol: "message", Status: "limited", Completeness: "evicted", Summary: reason, Error: reason,
			Raw: []byte("unavailable"), Session: map[string]any{"Headers": "must not recover"}},
		{Protocol: "message", Status: "limited", Completeness: "evicted", Summary: coverage, Error: coverage},
	}
	before, err := json.Marshal(events)
	require.NoError(t, err)
	fields, sources := protocolEventDetail(events)
	for _, got := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, events, false), workerMetadataDetail(t, events, true)} {
		text := detailLines(got.fields)
		require.Contains(t, text, "PDU #299 · detail unavailable")
		require.Contains(t, text, "Contributor coverage · detail unavailable")
		require.Equal(t, 1, strings.Count(text, reason))
		require.Equal(t, 1, strings.Count(text, coverage))
		require.NotContains(t, text, "PDU #0")
		require.NotContains(t, text, "MESSAGE ·")
		require.NotContains(t, text, "must not recover")
		require.Empty(t, got.sources)
		for _, f := range got.fields {
			require.False(t, f.hasRange)
			require.Zero(t, f.source)
		}
	}
	after, err := json.Marshal(events)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestWorkerOwnedDetailAvailabilityDoesNotFollowLaterHistoryEviction(t *testing.T) {
	// The decoder may already own complete fields when the parent loses its
	// cache entry. Availability belongs to that response, not a later lookup.
	ref := pcaputil.PacketReference{Number: 1}
	history, err := pcaputil.NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	history.OnEvent(queryIdentityEvent(1, ref, 4))
	raw := &capturedPacket{number: 1, history: history, reference: ref, eventIDs: []uint64{1}, presentation: packetPresentation{protocol: "TCP"}}
	history.OnEvent(queryIdentityEvent(2, pcaputil.PacketReference{Number: 2}, 2))
	response := decodeResponse{Number: 1, Protocol: "BGP", Fields: []decodeField{{Text: "Owned complete field snapshot"}}}
	wire, err := marshalDecodeResponse(response)
	require.NoError(t, err)
	got, err := decodeWorkerOutput(append([]byte(decoderFrame), wire...), raw)
	require.NoError(t, err)
	require.False(t, got.unavailable)
	require.Equal(t, "BGP", got.protocol, "a complete late response may advance the earlier transport-only caption")
	require.Equal(t, "Owned complete field snapshot\n", detailLines(got.fields))
}

func TestEvictedMarkersCannotPublishOldAuthenticationOrConnectionFaults(t *testing.T) {
	events := []*pcaputil.ProtocolEvent{
		{ID: 1, Protocol: "tls", Completeness: "evicted", Session: map[string]any{"Authentication Verified": true}},
		{ID: 2, Protocol: "tls", Completeness: "evicted", ExpertCode: string(pcaputil.ErrAuthenticationFailed)},
		{ID: 3, Protocol: "http3", Completeness: "evicted", Session: map[string]any{"HTTP3 Connection Error": map[string]any{"Error Name": "H3_FRAME_ERROR"}}},
	}
	fields, sources := protocolEventDetail(events)
	text := detailLines(fields)
	require.NotContains(t, text, "TLS records:")
	require.NotContains(t, text, "H3_FRAME_ERROR")
	require.Empty(t, sources)
}
