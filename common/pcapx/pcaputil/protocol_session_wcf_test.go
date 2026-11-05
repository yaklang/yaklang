package pcaputil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func wcfTestSize(n int) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if n == 0 {
			return out
		}
	}
}

func wcfTestPreamble(mode, encoding byte) []byte {
	via := []byte("net.tcp://example.test/service")
	wire := []byte{wcfRecordVersion, 0x01, 0x00, wcfRecordMode, mode, wcfRecordVia}
	wire = append(wire, wcfTestSize(len(via))...)
	wire = append(wire, via...)
	wire = append(wire, wcfRecordKnownEncoding, encoding)
	if mode != 0x04 {
		wire = append(wire, wcfRecordPreambleEnd)
	}
	return wire
}

func wcfTestSized(payload []byte) []byte {
	wire := []byte{wcfRecordSizedEnvelope}
	wire = append(wire, wcfTestSize(len(payload))...)
	return append(wire, payload...)
}

func wcfTestUnsized(chunks ...[]byte) []byte {
	wire := []byte{wcfRecordUnsizedEnvelope}
	for _, chunk := range chunks {
		wire = append(wire, wcfTestSize(len(chunk))...)
		wire = append(wire, chunk...)
	}
	return append(wire, 0)
}

func wcfTextEnvelope() []byte {
	return []byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><Ping>ok</Ping></s:Body></s:Envelope>`)
}

func wcfBinaryEnvelope() []byte {
	return append([]byte{0x70, 0x08}, []byte("Envelope")...)
}

func newWCFTestSession(t *testing.T, budget ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("tcp"))
	require.NoError(t, err)
	return s
}

func TestWCFProbePreambleAndCoalescedFraming(t *testing.T) {
	envelope := wcfTextEnvelope()
	preamble := wcfTestPreamble(0x02, 0x03)
	envelopeRecord := wcfTestSized(envelope)
	wire := append(bytes.Clone(preamble), envelopeRecord...)
	wire = append(wire, wcfRecordEnd)

	require.Equal(t, ProbeAccept, probeWCF(wire, 4096).Verdict)
	s := newWCFTestSession(t, DefaultParserBudget())
	t.Cleanup(func() { s.Close("FIN") })
	probe := s.Probe(wire)
	require.Equal(t, ProbeAccept, probe.Verdict)
	require.Equal(t, "wcf", probe.Protocol)

	original := bytes.Clone(wire)
	result := s.Feed(0, time.Unix(1, 0), wire)
	require.Nil(t, result.Err)
	require.False(t, result.NeedMore)
	require.Equal(t, "wcf", result.State)
	require.Len(t, result.Events, 7)

	wantNames := []string{"Version", "Mode", "Via", "Known Encoding", "Preamble End", "Sized Envelope", "End"}
	for i, event := range result.Events {
		require.Equal(t, "wcf", event.Protocol)
		require.Equal(t, "decoded", event.Status, event.Error)
		require.Equal(t, uint64(1), event.FlowID)
		require.Equal(t, 0, event.Direction)
		require.Equal(t, wantNames[i], event.Session["Record Name"])
		require.Equal(t, "wcf-mc-nmf", event.Profile)
		require.Contains(t, event.Summary, "WCF MC-NMF")
	}
	require.Equal(t, "1.0", result.Events[0].Session["Version"])
	require.Equal(t, "Duplex", result.Events[1].Session["Mode Name"])
	require.Equal(t, "net.tcp://example.test/service", result.Events[2].Session["Via"])
	require.Equal(t, "SOAP 1.2 UTF-8", result.Events[3].Session["Encoding Name"])
	require.Equal(t, "sized", result.Events[5].Session["Envelope Kind"])
	require.Equal(t, uint64(len(envelope)), result.Events[5].Session["Payload Length"])
	require.Equal(t, true, result.Events[5].Session["Envelope Body Opaque"])
	require.Equal(t, "initiator", result.Events[6].Session["Role"])

	display := result.Events[5].DisplayFields()
	require.Equal(t, []any{uint64(wcfRecordSizedEnvelope)}, display["wcf.record_type"])
	require.Equal(t, []any{uint64(len(envelope))}, display["wcf.payload_length"])
	require.Equal(t, []any{true}, display["wcf.envelope_opaque"])

	for i := range original {
		original[i] ^= 0xff
	}
	require.Equal(t, envelopeRecord, result.Events[5].Raw)
	require.NotEmpty(t, result.Events[5].Session["Envelope Body SHA256"])

	snapshot := result.Events[5].DisplayFieldValues()
	ranges := map[string]DisplayFieldRange{}
	for _, value := range snapshot.Values {
		if value.Range != nil {
			ranges[value.Name] = *value.Range
		}
	}
	require.Equal(t, DisplayFieldRange{Offset: 0, Length: 1}, ranges["wcf.record_type"])
	require.Equal(t, DisplayFieldRange{Offset: 1, Length: 1}, ranges["wcf.declared_length"])
	_, payloadLengthHasWireSpan := ranges["wcf.payload_length"]
	require.False(t, payloadLengthHasWireSpan, "a derived byte count is not the body it counts")
	require.Equal(t, []any{uint64(2), uint64(2 + len(envelope))}, result.Events[5].Session["Wire Field Ranges"].(map[string]any)["Envelope Body"])
}

func TestWCFPreambleAndRecordSplitAtEveryByte(t *testing.T) {
	preamble := wcfTestPreamble(0x02, 0x03)
	envelope := wcfTestSized(wcfTextEnvelope())
	for split := 0; split <= len(envelope); split++ {
		t.Run(fmt.Sprintf("split-%d", split), func(t *testing.T) {
			s := newWCFTestSession(t, DefaultParserBudget())
			defer s.Close("FIN")
			var events []*ProtocolEvent
			for _, chunk := range [][]byte{preamble, envelope[:split], envelope[split:]} {
				if len(chunk) == 0 {
					continue
				}
				result := s.Feed(0, time.Unix(2, 0), chunk)
				if result.Err != nil {
					require.Equal(t, ErrNeedMore, result.Err.Kind)
				}
				events = append(events, result.Events...)
			}
			require.Len(t, events, 6, "split=%d", split)
			require.Equal(t, envelope, events[5].Raw)
		})
	}
}

func TestWCFCaptureReplayChunkAndCoalescedBoundaries(t *testing.T) {
	wire := append(wcfTestPreamble(0x02, 0x03), wcfTestSized(wcfTextEnvelope())...)
	wire = append(wire, wcfRecordEnd)
	for _, chunk := range []int{0, 1, 2, 3, 7, 64} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("chunk=%d/deferred=%t", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "wcf", []sessionStep{{dir: 0, wire: wire}}, chunk, deferred)
				require.Len(t, events, 7)
				for _, event := range events {
					require.Equal(t, "wcf", event.Protocol)
					require.Equal(t, 0, event.Direction)
				}
				require.Equal(t, wcfTestSized(wcfTextEnvelope()), events[5].Raw)
				fields, err := events[5].GetFields()
				require.NoError(t, err)
				require.Equal(t, uint64(wcfRecordSizedEnvelope), fields["Record Type"])
			})
		}
	}
}

func TestWCFRedlineCaptureFraming(t *testing.T) {
	root := os.Getenv("YAKLANG_ENTERPRISE_FIXTURE_ROOT")
	if root == "" {
		t.Skip("set YAKLANG_ENTERPRISE_FIXTURE_ROOT to replay the external enterprise capture")
	}
	path := filepath.Join(root, "redline-infostealer-2023-07", "capture.pcap")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("optional external enterprise fixture unavailable: %v", err)
	}
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcapFile(path,
		WithTCPReassemblyWorkers(1),
		WithOnProtocolMessage(func(event *ProtocolEvent) {
			if event.Protocol == "wcf" {
				events = append(events, event)
			}
		}),
	))
	require.Len(t, events, 12)
	for i, event := range events {
		require.Equal(t, "decoded", event.Status, "event %d: %s", i, event.Error)
		require.Equal(t, "wcf", event.Protocol)
		require.NotZero(t, event.FlowID)
		require.NotEmpty(t, event.Raw)
	}
	wantNames := []string{
		"Version", "Mode", "Via", "Known Encoding", "Preamble End", "Preamble Ack",
		"Sized Envelope", "Sized Envelope", "Sized Envelope", "Sized Envelope", "Sized Envelope", "Sized Envelope",
	}
	for i, name := range wantNames {
		require.Equal(t, name, events[i].Session["Record Name"], "event %d", i)
	}
	wantLengths := []int{3, 2, 33, 2, 1, 1, 218, 159, 166, 6899, 541960, 120}
	for i, length := range wantLengths {
		require.Len(t, events[i].Raw, length, "event %d", i)
	}
	wantDirections := []int{0, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1}
	for i, direction := range wantDirections {
		require.Equal(t, direction, events[i].Direction, "event %d", i)
	}
	for _, index := range []int{6, 7, 8, 9, 10, 11} {
		require.Equal(t, true, events[index].Session["Envelope Body Opaque"])
		require.Equal(t, "Binary with in-band dictionary", events[index].Session["Encoding Name"])
	}
}

func TestWCFMidstreamBidirectionalSizedEnvelopes(t *testing.T) {
	contractName := []byte("http://tempuri.org/Contract/MSValue1")
	contractPayload := append([]byte{0x62}, wcfTestSize(len(contractName))...)
	contractPayload = append(contractPayload, contractName...)
	responsePrefix := []byte("http://tempuri.org/Contract/MSValue1Response")
	responseName := []byte("MSValue1Response")
	responsePayload := []byte{0x61}
	responsePayload = append(responsePayload, wcfTestSize(len(responsePrefix))...)
	responsePayload = append(responsePayload, responsePrefix...)
	responsePayload = append(responsePayload, wcfTestSize(len(responseName))...)
	responsePayload = append(responsePayload, responseName...)

	for name, payload := range map[string][]byte{
		"text":                    wcfTextEnvelope(),
		"binary-envelope":         wcfBinaryEnvelope(),
		"binary-wcf-request-root": contractPayload,
		"binary-wcf-reply-root":   responsePayload,
	} {
		t.Run(name, func(t *testing.T) {
			first, second := wcfTestSized(payload), wcfTestSized(append(bytes.Clone(payload), []byte("response")...))
			require.Equal(t, ProbeAccept, probeWCF(first, 4096).Verdict)
			s := newWCFTestSession(t, DefaultParserBudget())
			defer s.Close("FIN")

			result := s.Feed(1, time.Unix(3, 0), first)
			require.Nil(t, result.Err)
			require.Len(t, result.Events, 1)
			require.Equal(t, 1, result.Events[0].Direction)
			require.Equal(t, "midstream", result.Events[0].Session["Context Level"])
			require.Equal(t, "unobserved", result.Events[0].Session["Role"])

			result = s.Feed(0, time.Unix(4, 0), second)
			require.Nil(t, result.Err)
			require.Len(t, result.Events, 1)
			require.Equal(t, 0, result.Events[0].Direction)
			require.Equal(t, uint64(2), result.Events[0].Session["Envelope Sequence"])
		})
	}
}

func TestWCFMidstreamProbeHonorsCompleteDeclaredEnvelope(t *testing.T) {
	payload := wcfBinaryEnvelope()
	wire := wcfTestSized(payload)
	require.Equal(t, ProbeAccept, probeWCF(wire, len(wire)).Verdict)

	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	probe := s.Probe(wire)
	require.Equal(t, ProbeAccept, probe.Verdict)
	require.Equal(t, "wcf", probe.Protocol)
}

func TestWCFMidstreamReplaysBufferedPreambleAck(t *testing.T) {
	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")

	result := s.Feed(1, time.Unix(4, 0), []byte{wcfRecordPreambleAck})
	require.True(t, result.NeedMore)
	require.Empty(t, result.Events)

	result = s.Feed(0, time.Unix(5, 0), wcfTestSized(wcfBinaryEnvelope()))
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 2)
	byName := map[string]*ProtocolEvent{}
	for _, event := range result.Events {
		require.Equal(t, "wcf", event.Protocol)
		byName[event.Session["Record Name"].(string)] = event
	}
	require.Equal(t, 1, byName["Preamble Ack"].Direction)
	require.Equal(t, 0, byName["Sized Envelope"].Direction)
}

func TestWCFMidstreamFaultEvidence(t *testing.T) {
	fault := []byte(wcfFaultNamespace + "EndpointNotFound")
	wire := []byte{wcfRecordFault}
	wire = append(wire, wcfTestSize(len(fault))...)
	wire = append(wire, fault...)
	require.Equal(t, ProbeAccept, probeWCF(wire, 4096).Verdict)

	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	result := s.Feed(1, time.Unix(5, 0), wire)
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 1)
	require.Equal(t, "Fault", result.Events[0].Session["Record Name"])
	require.Equal(t, string(fault), result.Events[0].Session["Fault"])
	require.Equal(t, true, result.Events[0].Session["Terminal"])
}

func TestWCFUpgradeRequestResponseAndCarrierTransition(t *testing.T) {
	protocol := []byte("application/ssl-tls")
	preamble := wcfTestPreamble(0x02, 0x03)
	wire := append(bytes.Clone(preamble[:len(preamble)-1]), wcfRecordUpgradeRequest)
	wire = append(wire, wcfTestSize(len(protocol))...)
	wire = append(wire, protocol...)

	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	result := s.Feed(0, time.Unix(6, 0), wire)
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 5)
	require.Equal(t, "Upgrade Request", result.Events[4].Session["Record Name"])
	require.Equal(t, "application/ssl-tls", result.Events[4].Session["Upgrade Protocol"])
	// MC-NMF puts the response in the opposite direction, before the
	// upgraded stream carries its remaining preamble/application records.
	response := append([]byte{wcfRecordUpgradeResponse}, []byte("not-a-tls-record")...)
	result = s.Feed(1, time.Unix(7, 0), response)
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 1)
	require.Equal(t, true, result.Events[0].Session["Upgrade Accepted"])
	require.Equal(t, "wcf->carried", result.Events[0].Session["Protocol Transition"])
}

func TestWCFModeRecordSequenceViolation(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	defer s.Close("FIN")
	result := s.Feed(0, time.Unix(6, 0), append(wcfTestPreamble(0x02, 0x03), []byte{wcfRecordMode, 0x02}...))
	require.NotNil(t, result.Err)
	require.Equal(t, ErrMalformedMessage, result.Err.Kind)
	last := result.Events[len(result.Events)-1]
	require.Equal(t, "malformed", last.Status)
	require.Equal(t, "wcf", last.Protocol)
}

func TestWCFUnsizedEnvelopeChunks(t *testing.T) {
	first, second := []byte(`<s:Envelope xmlns:s="x"><s:Body>`), []byte(`<Ping/></s:Body></s:Envelope>`)
	wire := append(wcfTestPreamble(0x01, 0x03), wcfTestUnsized(first, second)...)
	wire = append(wire, wcfRecordEnd)

	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	result := s.Feed(0, time.Unix(5, 0), wire)
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 7)
	envelope := result.Events[5]
	require.Equal(t, "unsized", envelope.Session["Envelope Kind"])
	require.Equal(t, uint64(len(first)+len(second)), envelope.Session["Payload Length"])
	require.Equal(t, uint64(2), envelope.Session["Chunk Count"])
	require.Equal(t, wcfEnvelopeDigest(append(first, second...)), envelope.Session["Envelope Body SHA256"])
}

func TestWCFTruncatedEnvelopePreservesFailureState(t *testing.T) {
	preamble := wcfTestPreamble(0x02, 0x03)
	envelope := wcfTestSized(wcfTextEnvelope())

	s := newWCFTestSession(t, DefaultParserBudget())
	result := s.Feed(0, time.Unix(6, 0), append(bytes.Clone(preamble), envelope[:len(envelope)-3]...))
	require.True(t, result.NeedMore)
	require.Len(t, result.Events, 5)

	closed := s.Close("FIN")
	require.Len(t, closed, 1)
	require.Equal(t, "incomplete", closed[0].Status)
	require.Equal(t, "wcf", closed[0].Protocol)
	require.Equal(t, "wcf-mc-nmf", closed[0].Profile)
	require.Equal(t, uint64(wcfRecordSizedEnvelope), closed[0].Session["Observed Record Type"])
	require.Equal(t, 0, closed[0].Direction)
}

func TestWCFMalformedLengthsAndTransitions(t *testing.T) {
	tests := []struct {
		name   string
		before []byte
		bad    []byte
		kind   ProtocolErrorKind
		status string
	}{
		{"zero-size", wcfTestPreamble(0x02, 0x03), []byte{wcfRecordSizedEnvelope, 0}, ErrMalformedMessage, "malformed"},
		{"unterminated-size", wcfTestPreamble(0x02, 0x03), []byte{wcfRecordSizedEnvelope, 0x80, 0x80, 0x80, 0x80, 0x80}, ErrMalformedMessage, "malformed"},
		{"oversized", wcfTestPreamble(0x02, 0x03), []byte{wcfRecordSizedEnvelope, 0x81, 0x01}, ErrResourceExceeded, "limited"},
		{"reserved-type", wcfTestPreamble(0x02, 0x03), []byte{0x7f}, ErrMalformedMessage, "malformed"},
		{"envelope-before-end", []byte{wcfRecordVersion, 1, 0, wcfRecordMode, 2, wcfRecordVia}, nil, ErrMalformedMessage, "malformed"},
	}
	via := []byte("net.tcp://example.test/service")
	tests[len(tests)-1].before = append(tests[len(tests)-1].before, wcfTestSize(len(via))...)
	tests[len(tests)-1].before = append(tests[len(tests)-1].before, via...)
	tests[len(tests)-1].before = append(tests[len(tests)-1].before, wcfRecordKnownEncoding, 3)
	tests[len(tests)-1].bad = wcfTestSized(wcfTextEnvelope())

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWCFTestSession(t, ParserBudget{MaxFrameBytes: 128})
			defer s.Close("FIN")
			if len(test.before) > 0 {
				result := s.Feed(0, time.Unix(7, 0), test.before)
				require.Nil(t, result.Err)
			}
			result := s.Feed(0, time.Unix(8, 0), test.bad)
			require.NotNil(t, result.Err)
			require.Equal(t, test.kind, result.Err.Kind)
			require.NotEmpty(t, result.Events)
			last := result.Events[len(result.Events)-1]
			require.Equal(t, test.status, last.Status)
			require.Equal(t, "wcf", last.Protocol)
			require.Equal(t, "wcf-mc-nmf", last.Profile)
			require.Equal(t, 0, last.Direction)
		})
	}
}

func TestWCFSingletonSizedBodyRequiresContext(t *testing.T) {
	preamble := wcfTestPreamble(0x04, 0x03)
	s := newWCFTestSession(t, DefaultParserBudget())
	defer s.Close("FIN")
	result := s.Feed(0, time.Unix(9, 0), preamble)
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 4)

	result = s.Feed(0, time.Unix(10, 0), []byte("<s:Envelope/>"))
	require.NotNil(t, result.Err)
	require.Equal(t, ErrUnsupportedFeature, result.Err.Kind)
	require.NotEmpty(t, result.Events)
	require.Equal(t, "context-required", result.Events[len(result.Events)-1].Status)
}

func TestWCFRejectsUnrelatedLengthPrefixedTraffic(t *testing.T) {
	for name, wire := range map[string][]byte{
		"http":          []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"),
		"mqtt-pubrel":   {0x06, 0x02, 0x00, 0x01},
		"unknown":       {0x7f, 0x01, 0x00},
		"fake-size":     {0x06, 0x20, 'n', 'o', 't', '-', 's', 'o', 'a', 'p'},
		"wrong-scheme":  {0x00, 0x01, 0x00, 0x01, 0x02, 0x0a, 'h', 't', 't', 'p', ':', '/', '/', 'x', '/', 'y'},
		"short-version": {0x00, 0x01},
	} {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, ProbeAccept, probeWCF(wire, 4096).Verdict)
		})
	}
}

func FuzzWCFRecord(f *testing.F) {
	f.Add(wcfTestPreamble(0x02, 0x03))
	f.Add(wcfTestSized(wcfTextEnvelope()))
	f.Add(wcfTestUnsized([]byte("<s:Envelope/>")))
	f.Add([]byte{0x06, 0x80, 0x80, 0x80, 0x80, 0x80})
	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) > 4096 {
			wire = wire[:4096]
		}
		_, _, _ = parseWCFRecord(wire, 4096)
		_ = probeWCF(wire, 4096)
	})
}
