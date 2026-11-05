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

func wcfDisplayTestSession(t *testing.T, mode byte) (pcaputil.ProtocolSession, []*pcaputil.ProtocolEvent) {
	t.Helper()
	s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	via := []byte("net.tcp://example.test/owned")
	preamble := append([]byte{0, 1, 0, 1, mode, 2, byte(len(via))}, via...)
	preamble = append(preamble, 3, 7, 12)
	r := s.Feed(0, time.Unix(1, 0), preamble)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 5)
	ack := s.Feed(1, time.Unix(2, 0), []byte{11})
	require.Nil(t, ack.Err)
	return s, append(r.Events, ack.Events...)
}

func TestWCFDisplaySeparatesRecordBytesAndObservedSession(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		t.Run(map[bool]string{false: "sized", true: "chunked"}[chunked], func(t *testing.T) {
			mode := byte(2)
			body := []byte{0, 0xff, 7, 0x80, 11, 3}
			wire := append([]byte{6, byte(len(body))}, body...)
			if chunked {
				mode = 1
				wire = []byte{5, 3, 0, 0xff, 7, 3, 0x80, 11, 3, 0}
			}
			s, _ := wcfDisplayTestSession(t, mode)
			r := s.Feed(0, time.Unix(3, 0), wire)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			before, err := json.Marshal(e)
			require.NoError(t, err)
			localFields, localSources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, got := range []packetDetail{{fields: localFields, sources: localSources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				inContext, contextSeen, payloadSeen := false, false, false
				chunkBodies, chunkLengths, terminators := 0, 0, 0
				for _, f := range got.fields {
					if strings.HasPrefix(f.text, "Observed WCF Session ·") {
						inContext, contextSeen = true, true
					} else if inContext && f.depth == 0 {
						inContext = false
					}
					if inContext {
						require.False(t, f.hasRange, f.text)
						require.Zero(t, f.source, f.text)
					}
					if strings.HasPrefix(f.text, "Envelope Payload (opaque):") {
						payloadSeen = true
						require.True(t, f.hasRange)
						require.Positive(t, f.source)
						source := got.sources[f.source-1]
						require.Equal(t, body, source.Data[f.start:f.end])
						if chunked {
							require.Equal(t, body, source.Data)
							require.Contains(t, source.Name, "frame offsets unavailable")
						} else {
							require.Equal(t, wire, source.Data)
							require.Equal(t, 2, f.start)
						}
					}
					if strings.HasPrefix(f.text, "Payload Length:") {
						require.False(t, f.hasRange)
					}
					if chunked && strings.HasPrefix(f.text, "Declared Length:") {
						chunkLengths++
						require.True(t, f.hasRange)
						require.Equal(t, []byte{3}, got.sources[f.source-1].Data[f.start:f.end])
					}
					if chunked && strings.HasPrefix(f.text, "Chunk Body (opaque):") {
						require.True(t, f.hasRange)
						require.Equal(t, body[3*chunkBodies:3*(chunkBodies+1)], got.sources[f.source-1].Data[f.start:f.end])
						chunkBodies++
					}
					if chunked && strings.HasPrefix(f.text, "Terminator:") {
						terminators++
						require.True(t, f.hasRange)
						require.Equal(t, []byte{0}, got.sources[f.source-1].Data[f.start:f.end])
					}
				}
				require.True(t, contextSeen)
				require.True(t, payloadSeen)
				if chunked {
					require.Equal(t, 2, chunkBodies)
					require.Equal(t, 2, chunkLengths)
					require.Equal(t, 1, terminators)
				}
				require.Contains(t, detailLines(got.fields), "Observed Session Index: 1")
				require.Contains(t, detailLines(got.fields), "envelope body remains opaque")
			}
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestWCFNativeDiagnosticIdentitySurvivesWorkerWithoutPublicPromotion(t *testing.T) {
	for _, incomplete := range []bool{false, true} {
		t.Run(map[bool]string{false: "malformed", true: "incomplete"}[incomplete], func(t *testing.T) {
			s, _ := wcfDisplayTestSession(t, 2)
			wire := []byte{0x7f}
			if incomplete {
				wire = []byte{6, 4, 1, 2}
			}
			r := s.Feed(0, time.Unix(3, 0), wire)
			events := r.Events
			if incomplete {
				events = s.Close("FIN")
			}
			require.Len(t, events, 1)
			e := events[0]
			observation, ok := e.NativeWCFObservation()
			require.True(t, ok)
			e.Protocol, e.Status, e.Summary, e.Error = "edited", "decoded", "edited", ""
			e.Session = map[string]any{"Mode": 99, "Envelope Payload (opaque)": []byte("forged"), "Wire Field Ranges": map[string][]int{"Mode": {0, 1}}}
			row := presentEvents(events)
			require.Equal(t, "WCF/MC-NMF", row.protocol)
			require.Equal(t, observation.Status, row.status)
			require.Equal(t, observation.Summary, row.summary)
			raw := &capturedPacket{number: 1, events: events, presentation: row}
			request, err := marshalDecodeRequest(packetDecodeRequest(raw))
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
			detail, err := decodeWorkerOutput(output.Bytes(), raw)
			require.NoError(t, err)
			require.Equal(t, row.protocol, detail.protocol)
			require.Contains(t, detailLines(detail.fields), "WCF/MC-NMF · observation #")
			require.Contains(t, detailLines(detail.fields), "no complete record fields")
			require.NotContains(t, detailLines(detail.fields), "WCF/MC-NMF · PDU #")
			require.NotContains(t, detailLines(detail.fields), "forged")
			require.NotContains(t, detailLines(detail.fields), "Mode: 99")
			for _, source := range detail.sources {
				require.NotEqual(t, []byte("forged"), source.Data)
				require.Contains(t, source.Name, "stream bytes")
				require.NotContains(t, source.Name, "PDU")
			}
		})
	}
	fake := &pcaputil.ProtocolEvent{Protocol: "wcf", Status: "malformed", Transport: "tcp", Profile: "wcf-mc-nmf", Session: map[string]any{"Record Type": 6, "Observed Session Index": 1}}
	row := presentEvents([]*pcaputil.ProtocolEvent{fake}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
	require.Equal(t, "TCP", row.Protocol)
}

func TestWCFOpaqueBodyProjectionRequiresOwnedRecordBytes(t *testing.T) {
	s, _ := wcfDisplayTestSession(t, 2)
	r := s.Feed(0, time.Unix(3, 0), []byte{6, 3, 1, 2, 3})
	require.Nil(t, r.Err)
	e := r.Events[0]
	e.Raw[2] ^= 1
	projected := protocolDisplayEvent(e)
	require.NotContains(t, projected.Session, "Envelope Payload (opaque)")
	require.NotContains(t, projected.Session, "Wire Field Ranges")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.Empty(t, sources)
	for _, f := range fields {
		require.False(t, f.hasRange, f.text)
	}
	require.Contains(t, detailLines(fields), "owned Raw changed")
}
