package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func n14CookedOwnerPresentationPacket(t *testing.T, link layers.LinkType) *capturedPacket {
	t.Helper()
	original := tcpPacket(t, 1, 100, "GET /unproved HTTP/1.1\r\nHost: owner.invalid\r\n\r\n", false, false, false, true)
	body := original.data[14:]
	var h []byte
	if link == layers.LinkTypeLinuxSLL {
		h = make([]byte, 16)
		binary.BigEndian.PutUint16(h[2:4], 1)
		binary.BigEndian.PutUint16(h[4:6], 6)
		copy(h[6:12], original.data[6:12])
		binary.BigEndian.PutUint16(h[14:16], 0x8864)
	} else {
		h = make([]byte, 20)
		binary.BigEndian.PutUint16(h[:2], 0x8864)
		binary.BigEndian.PutUint32(h[4:8], 1)
		binary.BigEndian.PutUint16(h[8:10], 1)
		h[11] = 6
		copy(h[12:18], original.data[6:12])
	}
	poe := []byte{0x11, 0, 0, 30, 0, 0, 0, 0x57}
	binary.BigEndian.PutUint16(poe[4:6], uint16(2+len(body)))
	wire := append(append(h, poe...), body...)
	ci := gopacket.CaptureInfo{CaptureLength: len(wire), Length: len(wire)}
	domain := pcaputil.PacketDomain(wire, ci, link)
	reference := pcaputil.PacketReference{Number: 1, Domain: domain}
	e, ok := pcaputil.ObservePPPoEHeaderAtCapture(wire, ci, link, reference)
	require.True(t, ok, "an actual physical cooked header creates the private observation")
	e.ID = 1
	return &capturedPacket{number: 1, data: wire, ci: ci, link: link, reference: e.SourceBytes.PacketRefs[0], events: []*pcaputil.ProtocolEvent{e}, presentation: presentEvents([]*pcaputil.ProtocolEvent{e})}
}

func TestN14PPPoEOwnerContextSuppressesStaleChildAcrossViews(t *testing.T) {
	for _, link := range []layers.LinkType{layers.LinkTypeLinuxSLL, layers.LinkTypeLinuxSLL2} {
		for _, order := range []string{"alone", "before-stale", "after-stale"} {
			t.Run(fmt.Sprintf("link%d/%s", link, order), func(t *testing.T) {
				raw := n14CookedOwnerPresentationPacket(t, link)
				refused := raw.events[0]
				stale := &pcaputil.ProtocolEvent{ID: 7, Protocol: "http", Status: "decoded", Raw: []byte("GET /unproved HTTP/1.1\r\n\r\n"), FlowID: 19, TCPFlowID: 23, Summary: "unproved application"}
				if order == "before-stale" {
					raw.events = append(raw.events, stale)
				}
				if order == "after-stale" {
					raw.events = []*pcaputil.ProtocolEvent{stale, refused}
				}
				raw.presentation = presentEvents(raw.events)
				row := summarize(raw)
				require.Equal(t, "PPPoE", row.Protocol)
				require.Equal(t, "context-required", row.ProtocolStatus)
				require.Equal(t, "unconfirmed-session-owner", row.Visibility)
				require.Contains(t, row.Info, "PPPoE header observed")
				require.Contains(t, row.Info, "owner unconfirmed")
				require.Empty(t, row.ConnectionProtocol)
				require.Zero(t, row.StreamID)
				require.Zero(t, row.SourcePort)
				require.Zero(t, row.DestinationPort)
				require.Empty(t, row.Source)
				require.Empty(t, row.Destination)
				require.NotContains(t, row.ProtocolStack, "TCP")
				require.NotContains(t, row.ProtocolStack, "IPv6")
				start := 16
				if link == layers.LinkTypeLinuxSLL2 {
					start = 20
				}
				for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
					require.Equal(t, "PPPoE", got.protocol)
					require.NotEmpty(t, got.hex)
					text := detailLines(got.fields)
					require.Contains(t, text, "PPPoE header observed")
					require.Contains(t, text, "session owner unconfirmed")
					require.NotContains(t, text, "unproved application")
					require.NotContains(t, text, "YAML dissector")
					require.Empty(t, got.sources)
					found := 0
					for _, f := range got.fields {
						if f.text == "Observed PPPoE Session ID: 30" {
							found++
							require.True(t, f.hasRange)
							require.Zero(t, f.source)
							require.Equal(t, start+2, f.start)
							require.Equal(t, start+4, f.end)
							require.Equal(t, []byte{0, 30}, raw.data[f.start:f.end])
						}
						if f.text == refused.Summary {
							require.False(t, f.hasRange)
						}
					}
					require.Equal(t, 1, found)
				}
				stream := workerMetadataDetail(t, raw.events, true)
				require.Equal(t, "PPPoE", stream.protocol)
				require.Contains(t, detailLines(stream.fields), "PPPoE Session Owner: unconfirmed")
				for _, f := range stream.fields {
					require.False(t, f.hasRange)
				}
			})
		}
	}
}

func TestN14PPPoEOwnerContextLiteralRangeRequiresMatchingSource(t *testing.T) {
	for _, kind := range []string{"frame", "domain", "literal-SID"} {
		t.Run(kind, func(t *testing.T) {
			raw := n14CookedOwnerPresentationPacket(t, layers.LinkTypeLinuxSLL)
			switch kind {
			case "frame":
				raw.number++
			case "domain":
				raw.reference.Domain.Interface++
			case "literal-SID":
				binary.BigEndian.PutUint16(raw.data[18:20], 31)
			}
			for _, got := range []packetDetail{detail(raw), quickDetail(raw)} {
				require.NotContains(t, detailLines(got.fields), "Observed PPPoE Session ID:")
			}
		})
	}
	raw := n14CookedOwnerPresentationPacket(t, layers.LinkTypeLinuxSLL)
	require.True(t, bytes.Contains(raw.data, []byte{0, 30}))
}

func TestN14PPPoEPrivateHeaderWinsPublicPoisonAcrossViews(t *testing.T) {
	raw := n14CookedOwnerPresentationPacket(t, layers.LinkTypeLinuxSLL)
	e := raw.events[0]
	e.Protocol, e.Profile, e.Status, e.ExpertCode = "http", "forged", "decoded", ""
	e.Summary = "forbidden forged application"
	e.Raw = []byte("forbidden forged PDU")
	e.Fields = map[string]any{"PPPoE Session ID": uint16(31)}
	e.Session = map[string]any{"PPPoE Session ID": uint16(31), "PPPoE Session Owner": "confirmed"}
	e.SourceBytes = pcaputil.ByteSource{Kind: "reassembled", PacketRefs: []pcaputil.PacketReference{{Number: 999}}}
	raw.application, raw.evidence = "HTTP", "forbidden stale connection"
	raw.connectionStack = []string{"TCP", "HTTP"}
	raw.presentation = presentEvents(raw.events)
	row := summarize(raw)
	require.Equal(t, "PPPoE", row.Protocol)
	require.Equal(t, "context-required", row.ProtocolStatus)
	require.Empty(t, row.ConnectionProtocol)
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		require.Equal(t, "PPPoE", got.protocol)
		require.Empty(t, got.sources)
		lines := detailLines(got.fields)
		require.Contains(t, lines, "Observed PPPoE Session ID: 30")
		require.NotContains(t, lines, "forbidden")
		require.NotContains(t, lines, "Session ID: 31")
		for _, f := range got.fields {
			if f.text == "Observed PPPoE Session ID: 30" {
				require.True(t, f.hasRange)
				require.Zero(t, f.source)
				require.Equal(t, 18, f.start)
				require.Equal(t, 20, f.end)
			}
		}
	}
	stream := workerMetadataDetail(t, raw.events, true)
	require.Equal(t, "PPPoE", stream.protocol)
	require.Contains(t, detailLines(stream.fields), "PPPoE Session ID: 30")
	require.NotContains(t, detailLines(stream.fields), "forbidden")
	for _, f := range stream.fields {
		require.False(t, f.hasRange)
	}
}

func TestN14PPPoEDTOCannotSupplyNativeHeaderOrSourceZeroProof(t *testing.T) {
	raw := n14CookedOwnerPresentationPacket(t, layers.LinkTypeLinuxSLL)
	native := raw.events[0]
	projection := pppoeHeaderDisplayEvent(native)
	wire, err := json.Marshal(projection)
	require.NoError(t, err)
	var dto pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &dto))
	_, ok := dto.NativePPPoEHeaderObservation()
	require.False(t, ok)
	require.False(t, dto.NativePPPoEHeaderWireMatches(raw.data[16:22]))
	var d packetDetail
	packet := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.Default)
	appendCapturedPPPoEOwnerHeaderFields(&d, raw, packet, []*pcaputil.ProtocolEvent{&dto})
	require.Empty(t, d.fields, "public fields and source refs cannot mint physical ranges")
	req := packetDecodeRequest(raw)
	req.Events = []*pcaputil.ProtocolEvent{&dto}
	req.PPPoEHeaderProof = &pppoeHeaderWorkerProof{Header: bytes.Clone(raw.data[16:22]), Reference: raw.reference}
	encoded, err := marshalDecodeRequest(req)
	require.NoError(t, err)
	var projected decodeRequest
	require.NoError(t, json.Unmarshal(encoded, &projected))
	require.Nil(t, projected.PPPoEHeaderProof, "parent discards a DTO-supplied private proof")
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(encoded), &output))
	got, err := decodeWorkerOutput(output.Bytes(), raw)
	require.NoError(t, err)
	require.Equal(t, "PPPoE", got.protocol)
	require.Contains(t, detailLines(got.fields), "PPPoE Session Owner: unconfirmed")
	require.Empty(t, got.sources)
	for _, f := range got.fields {
		require.False(t, f.hasRange)
	}
}

func TestN14PPPoEChangedPhysicalHeaderCannotBorrowPrivateRangeInWorker(t *testing.T) {
	raw := n14CookedOwnerPresentationPacket(t, layers.LinkTypeLinuxSLL)
	binary.BigEndian.PutUint16(raw.data[18:20], 31)
	encoded, err := marshalDecodeRequest(packetDecodeRequest(raw))
	require.NoError(t, err)
	var req decodeRequest
	require.NoError(t, json.Unmarshal(encoded, &req))
	require.Nil(t, req.PPPoEHeaderProof)
	got := workerMetadataDetailForPacket(t, raw)
	require.Equal(t, "PPPoE", got.protocol)
	require.Contains(t, detailLines(got.fields), "PPPoE Session ID: 30")
	require.NotContains(t, detailLines(got.fields), "Observed PPPoE Session ID:")
	require.Empty(t, got.sources)
	for _, f := range got.fields {
		require.False(t, f.hasRange)
	}
}
