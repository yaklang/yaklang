package sharkcli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func tlsPresentationPackets(t *testing.T, key bool) []*capturedPacket {
	t.Helper()
	base := "../../../../pcapx/pcaputil/testdata/protocol-sessions/first-batch-m1"
	path, err := trafficfixture.Materialize(filepath.Join(base, "tls-h2-bidi.pcap"), t.TempDir())
	require.NoError(t, err)
	cfg := captureConfig{input: path, snaplen: 65535}
	if key {
		data, err := trafficfixture.ReadFile(filepath.Join(base, "tls-h2-bidi.keys"))
		require.NoError(t, err)
		cfg.tlsSecrets, err = pcaputil.ParseTLSKeyLog(string(data))
		require.NoError(t, err)
	}
	src, err := openCapture(cfg)
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, cfg, false)
	var packets []*capturedPacket
	for p := range s.packets {
		packets = append(packets, p)
	}
	require.NoError(t, s.result())
	require.Len(t, packets, 92)
	return packets
}

func TestPresentationDoesNotIdentifyUDPAsTLS(t *testing.T) {
	data := udpPacket(t, 443, []byte{22, 3, 3, 0, 4, 255, 255, 255, 255})
	row := summarize(&capturedPacket{number: 1, data: data, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(data)}})
	require.Equal(t, "UDP", row.Protocol)
	require.Equal(t, "udp", row.Transport)
	require.Contains(t, row.Info, "port hint: QUIC")
}

func TestPresentationShowsNativeTLSAndDecryptedApplications(t *testing.T) {
	for _, keyed := range []bool{false, true} {
		t.Run(map[bool]string{false: "encrypted", true: "authenticated"}[keyed], func(t *testing.T) {
			counts := map[string]int{}
			var text strings.Builder
			for _, p := range tlsPresentationPackets(t, keyed) {
				row := summarize(p)
				counts[row.Protocol]++
				text.WriteString(row.Info + "\n")
				require.NotContains(t, row.Protocol, "TCP/", "connection identity must not replace the current packet protocol")
			}
			require.Positive(t, counts["TLSv1.2"])
			require.Positive(t, counts["TLSv1.3"])
			require.Contains(t, text.String(), "Client Hello")
			require.Contains(t, text.String(), "Server Hello")
			if keyed {
				require.Positive(t, counts["HTTP/2"])
				require.Positive(t, counts["gRPC"])
				require.Contains(t, text.String(), "/m1.Echo/Bidi")
			} else {
				require.Zero(t, counts["HTTP/2"])
				require.Zero(t, counts["gRPC"])
				require.Contains(t, text.String(), "Encrypted record")
			}
		})
	}
}

func TestDecryptedMessageBytesSurviveWorkerAndFieldSelection(t *testing.T) {
	var raw *capturedPacket
	for _, p := range tlsPresentationPackets(t, true) {
		for _, e := range p.protocolEvents() {
			if e.Protocol == "grpc" && len(mapList(eventValues(e)["GRPC Messages"])) > 0 {
				raw = p
				break
			}
		}
		if raw != nil {
			break
		}
	}
	require.NotNil(t, raw)
	req, err := marshalDecodeRequest(decodeRequest{Events: raw.protocolEvents(), Number: raw.number, Data: raw.data, Length: raw.ci.Length, Link: raw.link})
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(req), &output))
	d, err := decodeWorkerOutput(output.Bytes(), raw)
	require.NoError(t, err)
	require.Equal(t, "gRPC", d.protocol)
	u := &tui{ring: newPacketRing(10), width: 180, height: 54, inspect: raw, fields: d, selected: raw.number}
	u.ring.add(raw)
	messageField := -1
	for i, f := range d.fields {
		if strings.HasPrefix(f.text, "gRPC message ") {
			messageField = i
			break
		}
	}
	require.NotEqual(t, -1, messageField)
	u.fieldSelection = messageField
	require.True(t, u.selectFieldBytes())
	require.Positive(t, u.bytesSource)
	require.Equal(t, []byte{10, 3, 'm', '1', '0'}, u.byteData())
	require.Equal(t, 5, u.hexEnd)
	require.Contains(t, u.bytesLabel(), "gRPC message")
	r := u.viewport(2)
	u.selectByteAt(r.x+8, r.y)
	require.Equal(t, messageField, u.fieldSelection, "generated offsets must not select unrelated frame fields")
	u.fieldSelection = 0
	require.True(t, u.selectFieldBytes())
	require.Zero(t, u.bytesSource)
	require.Equal(t, raw.data, u.byteData())
	// The input map takes a JSON round trip across worker boundaries; retain the
	// decoded payload as bytes rather than displaying its base64 transport form.
	var wire decodeRequest
	require.NoError(t, json.Unmarshal(req, &wire))
	fields, sources := protocolEventDetail(wire.Events)
	var text strings.Builder
	for _, f := range fields {
		text.WriteString(f.text + "\n")
	}
	require.Contains(t, text.String(), "HEX 0a 03 6d 31 30")
	require.NotContains(t, text.String(), "CgNtMTA=")
	require.NotEmpty(t, sources)
}

func TestTLSDetailAndStreamShowEvidenceAndActualApplication(t *testing.T) {
	packets := tlsPresentationPackets(t, true)
	var events []*pcaputil.ProtocolEvent
	seen := map[uint64]bool{}
	var app *capturedPacket
	for _, p := range packets {
		row := summarize(p)
		if row.Protocol == "gRPC" && strings.Contains(strings.Join(row.ProtocolStack, " "), "TLSv1.3") {
			app = p
			break
		}
	}
	require.NotNil(t, app)
	for _, p := range packets {
		if p.streamID != app.streamID {
			continue
		}
		for _, e := range p.protocolEvents() {
			if !seen[e.ID] {
				seen[e.ID] = true
				events = append(events, e)
			}
		}
	}
	fields, sources := protocolEventDetail(events)
	var content strings.Builder
	for _, f := range fields {
		content.WriteString(f.text + "\n")
		if strings.HasPrefix(f.text, "Record Version:") && f.hasRange {
			require.Positive(t, f.source)
			data := sources[f.source-1].Data[f.start:f.end]
			require.Len(t, data, 2)
			require.Equal(t, byte(3), data[0])
			require.Contains(t, []byte{1, 3}, data[1], "record legacy version is independent of negotiated TLS version")
		}
	}
	require.Contains(t, content.String(), "TLSv1.3 (0x0304)")
	require.Contains(t, content.String(), "TLS_AES_128_GCM_SHA256")
	require.Contains(t, content.String(), "Record AEAD verified: true")
	require.Contains(t, content.String(), ":path: /m1.Echo/Bidi")
	v := &streamSnapshot{id: app.streamID, events: events, protocol: "TLS", endpoints: [2]string{events[0].Source, events[0].Destination}}
	u := &tui{ring: newPacketRing(100), width: 180, height: 54, pane: 3, inspect: app, stream: v, streamPacket: app.number, streamRefresh: time.Now(), streamPinned: true, streamApplication: true, tlsKeysConfigured: true}
	u.ring.add(app)
	var rendered strings.Builder
	for _, l := range u.streamLines() {
		rendered.WriteString(l.text + "\n")
	}
	require.Contains(t, rendered.String(), "gRPC")
	require.Contains(t, rendered.String(), "/m1.Echo/Bidi")
	require.Contains(t, rendered.String(), "record AEAD verified")
	require.NotContains(t, rendered.String(), "keys not configured")
	require.Contains(t, rendered.String(), "0a 03 6d 31 30")
	u.key("a", nil)
	require.False(t, u.streamApplication)
	u.key("a", nil)
	require.True(t, u.streamApplication)
	u.key("h", nil)
	require.False(t, u.streamApplication)
	row := summarize(app)
	for _, query := range []string{"proto:grpc", "proto:http2", "proto:tls"} {
		term, err := compileSearchTerm(query)
		require.NoError(t, err)
		require.True(t, term.matches(nil, row, nil), query)
	}
}

func TestTLSAuthenticationFailureRemainsOpaqueAndVisible(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "tls", Status: "malformed", ExpertCode: string(pcaputil.ErrAuthenticationFailed), Error: "AEAD tag mismatch", Session: map[string]any{"Negotiated Version": uint16(0x304)}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TLS", Transport: "tcp", Info: "old"})
	require.Equal(t, "TLSv1.3", row.Protocol)
	require.Equal(t, "encrypted", row.Visibility)
	require.Contains(t, row.Info, "authentication failed")
	require.Contains(t, row.Info, "AEAD tag mismatch")
	require.NotContains(t, row.ProtocolStack, "gRPC")
	v := &streamSnapshot{protocol: "TLS", events: []*pcaputil.ProtocolEvent{
		{Protocol: "http2", Status: "decoded", Summary: "previous authenticated app message"}, e,
	}}
	var messages strings.Builder
	for _, line := range applicationMessageRows(v, 0, 0, 100) {
		messages.WriteString(line.text + "\n")
	}
	require.Contains(t, messages.String(), "AEAD tag mismatch", "later TLS failures remain visible when application messages exist")
}
