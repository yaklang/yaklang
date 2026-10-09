package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type incrementalMVPCase struct {
	Group, ID, Capture, Answer, SHA256 string
	Packets                            int
}

type incrementalMVPAnswer struct {
	Protocol, Status    string
	ExpertCode          string `json:"expert_code"`
	Domains             []string
	HTTPTransaction     bool             `json:"http_transaction"`
	ExpectedSources     []string         `json:"expected_sources"`
	FlowIsolation       bool             `json:"flow_isolation"`
	DNSID               int              `json:"dns_id"`
	DNSQuestion         string           `json:"dns_question"`
	HTTPRequests        []string         `json:"http_requests"`
	ExpectedMessages    []map[string]any `json:"expected_messages"`
	NativeMessages      []map[string]any `json:"native_expected_messages"`
	NativeRaw           []string         `json:"native_raw_hex"`
	TerminalError       string           `json:"expected_terminal_error"`
	NativeTerminalError *string          `json:"native_terminal_error"`
	Transport           string
	Port                uint16
	Steps               []struct {
		Dir int
		Hex string
	}
	ExpectedOutstanding *int          `json:"expected_outstanding_before_close"`
	ClientDirection     *int          `json:"session_client_direction"`
	Budget              *ParserBudget `json:"parser_budget"`
	MaxMessages         int           `json:"max_messages"`
	MaxMessage          int           `json:"max_message"`
	ErrorKinds          []string      `json:"error_kinds"`
	PureError           string        `json:"pure_error"`
}

// This sealed batch contains independently authored wire controls and answers.
// Every configuration must satisfy the answers, as well as semantic parity;
// parity alone would also pass if every configuration made the same mistake.
func TestIncrementalMVPSealedReplay(t *testing.T) {
	runIncrementalMVPSealedReplay(t, "incremental-mvp", []string{"tunnel", "mavlink", "signature", "memcached", "doip", "nmea", "semtech", "ssh-s7", "dns-pointer", "atg", "media"})
}

func TestSemanticReviewMVPSealedReplay(t *testing.T) {
	runIncrementalMVPSealedReplay(t, "semantic-review-mvp", []string{"dns-stream", "memcached", "nmea", "semtech", "ftp", "ssh"})
}

func runIncrementalMVPSealedReplay(t *testing.T, prefix string, groups []string) {
	t.Helper()
	raw, err := trafficfixture.ReadFile(prefix + "/manifest.json")
	require.NoError(t, err)
	var inventory struct {
		Schema string
		Groups []string
		Cases  []incrementalMVPCase
	}
	require.NoError(t, json.Unmarshal(raw, &inventory))
	require.Equal(t, "pr5013-incremental-mvp/v1", inventory.Schema)
	require.NotEmpty(t, inventory.Cases)
	for _, group := range groups {
		require.Contains(t, inventory.Groups, group)
	}
	// Every sealed capture must also be represented in the shared inventory.
	// The generic rule runner has a narrower scope; this test executes each
	// native answer and binds it to the same capture and answer hashes.
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	frozen := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			if strings.HasPrefix(c.ID, prefix+"/") {
				frozen[c.ID] = c
			}
		}
	}
	require.Len(t, frozen, len(inventory.Cases))
	seen := map[string]bool{}
	for _, c := range inventory.Cases {
		require.False(t, seen[c.Group+"/"+c.ID], "duplicate control")
		seen[c.Group+"/"+c.ID] = true
		t.Run(c.Group+"/"+c.ID, func(t *testing.T) {
			wire, err := trafficfixture.ReadFile(prefix + "/" + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(wire)))
			answer, err := trafficfixture.ReadFile(prefix + "/" + c.Answer)
			require.NoError(t, err)
			fact, present := frozen[prefix+"/"+c.Group+"/"+c.ID]
			require.True(t, present)
			require.Equal(t, c.Capture, fact.Input.File)
			require.Equal(t, c.SHA256, fact.Input.SHA256)
			require.EqualValues(t, len(wire), fact.Input.Bytes)
			require.Equal(t, c.Packets, fact.Facts.PacketCount)
			require.Len(t, fact.Expectations, 1)
			require.Equal(t, "source_manifest_constraints", fact.Expectations[0].Kind)
			var binding struct {
				AnswerFile string `json:"answer_file"`
				AnswerSHA  string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(fact.Expectations[0].PayloadConstraints, &binding))
			require.Equal(t, c.Answer, binding.AnswerFile)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(answer)), binding.AnswerSHA)
			if prefix == "incremental-mvp" {
				wire = correctedIncrementalCarrier(t, c, wire, answer)
			}
			var a incrementalMVPAnswer
			require.NoError(t, json.Unmarshal(answer, &a))
			incrementalMVPBudget(t, c, a, wire)
			if prefix == "semantic-review-mvp" {
				semanticReviewMVPSessions(t, c, a, answer)
			}
			var reference []string
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("workers%d/deferred%v/observer%v", workers, deferred, observe), func(t *testing.T) {
							var mu sync.Mutex
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var observed atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { observed.Add(1) }))
							}
							input := bytes.Clone(wire)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							require.EqualValues(t, c.Packets, assembly.CapturedPackets)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Zero(t, assembly.CallbackPanics)
							if observe {
								require.EqualValues(t, c.Packets, observed.Load())
							}
							// Retained fields and raw PDUs must own their bytes after replay returns.
							for i := range input {
								input[i] = 0
							}
							for _, e := range events {
								if len(e.Raw) > 0 {
									require.NotEmpty(t, e.SourceBytes.PacketRefs, "%s %s source=%s", e.Protocol, e.Status, e.SourceBytes.Kind)
								}
								for _, ref := range e.SourceBytes.PacketRefs {
									require.Equal(t, e.Domain, ref.Domain)
									require.Greater(t, ref.Number, uint64(0))
									require.LessOrEqual(t, ref.Number, uint64(c.Packets))
								}
							}
							canonical := smallCanonicalEvents(t, events)
							normalized := incrementalMVPNormalized(t, events)
							switch c.Group {
							case "dns-stream":
								assertDNSStreamMVPEvents(t, answer, normalized)
							case "ftp", "ssh":
								assertFTPSSHReviewMVPEvents(t, c.Group, answer, normalized)
							case "memcached":
								assertMemcachedMVPEvents(t, answer, normalized)
							case "media":
								assertMediaMVPEvents(t, answer, normalized)
							case "dns-pointer":
								assertDNSPointerMVPEvents(t, answer, normalized)
							case "signature":
								assertSignatureMVPEvents(t, answer, normalized)
								incrementalMVPRawOracle(t, answer, normalized)
							case "tunnel":
								incrementalMVPTunnel(t, a, normalized)
							case "mavlink", "doip", "nmea", "semtech", "ssh-s7", "atg":
								incrementalMVPMessages(t, c, a, normalized)
								if prefix == "semantic-review-mvp" {
									assertSemanticReviewDatagrams(t, c, a, normalized)
								}
							default:
								t.Fatalf("control group %q has no semantic verifier", c.Group)
							}

							if reference == nil {
								reference = canonical
							} else {
								require.Equal(t, reference, canonical, "worker/deferred/observer semantic parity")
							}
						})
					}
				}
			}
		})
	}
}

func assertSemanticReviewDatagrams(t testing.TB, c incrementalMVPCase, a incrementalMVPAnswer, events []*ProtocolEvent) {
	t.Helper()
	if c.Group != "nmea" && c.Group != "semtech" {
		return
	}
	if a.PureError != "" {
		require.Len(t, events, 1)
		require.Equal(t, "unrecognized", events[0].Status)
		require.Empty(t, events[0].Protocol, "invalid datagram cannot be admitted as another native profile")
		return
	}
	require.Len(t, events, len(a.ExpectedMessages))
	for i, e := range events {
		fields, err := e.GetFields()
		require.NoError(t, err)
		got, err := json.Marshal(fields)
		require.NoError(t, err)
		want, err := json.Marshal(a.ExpectedMessages[i])
		require.NoError(t, err)
		require.JSONEq(t, string(want), string(got), "complete independent observed datagram fields")
	}
}

func semanticReviewMVPSessions(t *testing.T, c incrementalMVPCase, a incrementalMVPAnswer, answer json.RawMessage) {
	t.Helper()
	if c.Group == "nmea" || c.Group == "semtech" {
		for _, step := range a.Steps {
			wire, err := hex.DecodeString(step.Hex)
			require.NoError(t, err)
			var fields map[string]any
			if c.Group == "nmea" {
				fields, err = decodeNMEADatagram(wire, 4096)
			} else {
				fields, err = decodeSemtechDatagram(wire, 4096)
			}
			if a.PureError == "" {
				require.NoError(t, err)
				require.NotEmpty(t, fields)
			} else {
				require.Error(t, err)
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, a.PureError, string(typed.Kind))
			}
		}
	}
	chunks := []int{0}
	if a.Transport == "tcp" {
		chunks = []int{0, 1, 7, 64}
	}
	for _, deferred := range []bool{false, true} {
		for _, chunk := range chunks {
			opts := []ProtocolSessionOption{WithSessionTransport(a.Transport), WithSessionPorts(41707, a.Port)}
			if a.ClientDirection != nil {
				opts = append(opts, WithSessionClientDirection(*a.ClientDirection))
			}
			s, err := NewProtocolSessionWithOptions(ParserBudget{}, opts...)
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			var events []*ProtocolEvent
			for _, step := range a.Steps {
				wire, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				for len(wire) > 0 {
					n := len(wire)
					if chunk > 0 {
						n = min(n, chunk)
					}
					input := bytes.Clone(wire[:n])
					r := s.Feed(step.Dir, time.Unix(1700000000, 0), input)
					require.Equal(t, n, r.Consumed)
					events = append(events, r.Events...)
					clear(input)
					wire = wire[n:]
				}
			}
			events = append(events, s.Close("fixture-end")...)
			require.Zero(t, s.Stats().BufferedBytes)
			require.Zero(t, s.Stats().Flows)
			normalized := incrementalMVPNormalized(t, events)
			switch c.Group {
			case "dns-stream":
				assertDNSStreamMVPEvents(t, answer, normalized)
			case "ftp", "ssh":
				assertFTPSSHReviewMVPEvents(t, c.Group, answer, normalized)
			case "memcached":
				assertMemcachedMVPEvents(t, answer, normalized)
			case "nmea", "semtech":
				incrementalMVPMessages(t, c, a, normalized)
				assertSemanticReviewDatagrams(t, c, a, normalized)
			}
		}
	}
}

func assertDNSStreamMVPEvents(t testing.TB, answer json.RawMessage, events []*ProtocolEvent) {
	t.Helper()
	var a struct {
		Forbidden []string          `json:"forbidden_protocols"`
		Status    string            `json:"status"`
		Error     string            `json:"error_contains"`
		Messages  []json.RawMessage `json:"dns_messages"`
		Raw       []string          `json:"native_raw_hex"`
	}
	require.NoError(t, json.Unmarshal(answer, &a))
	if a.Status != "decoded" {
		require.Len(t, events, 1, "a failed admission or exact DNS frame emits one terminal diagnostic")
		require.Equal(t, a.Status, events[0].Status)
		if a.Error != "" {
			require.Contains(t, events[0].Error, a.Error)
		}
	}
	var decoded []*ProtocolEvent
	for _, e := range events {
		for _, protocol := range a.Forbidden {
			require.NotEqual(t, protocol, e.Protocol)
		}
		if e.Status == "decoded" {
			require.Equal(t, "dns", e.Protocol)
			require.Empty(t, e.Error)
			decoded = append(decoded, e)
		}
	}
	require.Len(t, decoded, len(a.Messages), "whole independently specified DNS message set")
	require.Len(t, a.Raw, len(decoded))
	for i, e := range decoded {
		got, err := json.Marshal(e.Session["DNS"])
		require.NoError(t, err)
		require.JSONEq(t, string(a.Messages[i]), string(got), "RFC1035 header/question/RR oracle")
		require.Equal(t, a.Raw[i], hex.EncodeToString(e.Raw))
		if i%2 == 1 {
			require.Equal(t, decoded[i-1].ID, e.ResponseTo)
		}
	}
}

func incrementalMVPNormalized(t *testing.T, es []*ProtocolEvent) []*ProtocolEvent {
	t.Helper()
	out := make([]*ProtocolEvent, len(es))
	for i, e := range es {
		copyEvent := *e
		if e.Status == "decoded" || e.Status == "deferred" {
			_, err := e.GetFields()
			require.NoError(t, err, "%s %s", e.Protocol, e.Error)
			copyEvent.Status = "decoded"
		}
		out[i] = &copyEvent
	}
	return out
}

func incrementalMVPMessages(t *testing.T, c incrementalMVPCase, a incrementalMVPAnswer, es []*ProtocolEvent) {
	t.Helper()
	protocol := a.Protocol
	switch c.Group {
	case "mavlink", "doip", "nmea":
		protocol = c.Group
	case "semtech":
		protocol = "semtech-udp"
	}
	want := a.ExpectedMessages
	if a.NativeMessages != nil {
		want = a.NativeMessages
	}
	var decoded []*ProtocolEvent
	for _, e := range es {
		if e.Protocol == protocol && e.Status == "decoded" {
			decoded = append(decoded, e)
		}
	}
	require.Len(t, decoded, len(want), "complete decoded count for %s", protocol)
	for i, e := range decoded {
		fields, err := e.GetFields()
		require.NoError(t, err)
		// Session includes negotiated context/association, not only the wire tree.
		merged := make(map[string]any, len(fields)+len(e.Session))
		for k, v := range fields {
			merged[k] = v
		}
		for k, v := range e.Session {
			merged[k] = v
		}
		assertMVPJSONFields(t, want[i], merged)
		if a.NativeRaw != nil {
			require.Len(t, a.NativeRaw, len(decoded))
			require.Equal(t, a.NativeRaw[i], hex.EncodeToString(e.Raw), "complete PDU source order")
		}
		if c.Group == "mavlink" {
			b, err := json.Marshal(fields)
			require.NoError(t, err)
			expected, err := json.Marshal(want[i])
			require.NoError(t, err)
			require.JSONEq(t, string(expected), string(b), "complete MAVLink header/body/opaque-signature oracle")
			require.Equal(t, a.NativeRaw[i], hex.EncodeToString(e.Raw))
		}
		if c.Group == "nmea" || c.Group == "semtech" {
			require.Zero(t, e.ResponseTo, "observed datagrams have no inferred transaction")
			require.NotContains(t, e.Session, "Association")
		}
	}
	terminal := a.TerminalError
	if a.NativeTerminalError != nil {
		terminal = *a.NativeTerminalError
	}
	if c.Group == "mavlink" && len(want) == 0 && len(a.ErrorKinds) > 0 {
		terminal = a.ErrorKinds[0]
	}
	if a.NativeMessages != nil && a.Budget != nil {
		terminal = ""
	}
	switch terminal {
	case "MalformedMessage", "UnsupportedFeature", "UnsupportedVersion", "ResourceExceeded", "ContextRequired":
		found := false
		for _, e := range es {
			if e.Protocol == protocol && (e.ExpertCode == terminal || strings.Contains(e.Error, terminal)) {
				found = true
			}
		}
		require.True(t, found, "expected typed terminal %s for %s", terminal, protocol)
	case "IncompleteMessage":
		found := false
		for _, e := range es {
			if e.Protocol == protocol && e.Status == "incomplete" {
				found = true
			}
		}
		require.True(t, found, "truncated message must produce incomplete diagnostic")
	case "":
		for _, e := range es {
			if e.Protocol == protocol {
				require.NotEqual(t, "malformed", e.Status, e.Error)
				require.NotEqual(t, "limited", e.Status, e.Error)
			}
		}
	}
}

func incrementalMVPTunnel(t *testing.T, a incrementalMVPAnswer, es []*ProtocolEvent) {
	t.Helper()
	require.Len(t, es, len(a.Domains))
	var domains, requests []string
	flows := map[uint64]bool{}
	for i, e := range es {
		require.Equal(t, a.Protocol, e.Protocol)
		require.Equal(t, a.Status, e.Status)
		require.Equal(t, a.ExpertCode, e.ExpertCode)
		domains = append(domains, e.Domain.Encapsulation)
		flows[e.FlowID] = true
		if e.Protocol == "dns" {
			dns := e.Session["DNS"].(map[string]any)
			require.Equal(t, fmt.Sprint(a.DNSID), fmt.Sprint(dns["ID"]))
			qs := dns["Questions"].([]map[string]any)
			require.Len(t, qs, 1)
			require.Equal(t, a.DNSQuestion, qs[0]["Name"])
			require.Equal(t, "1", fmt.Sprint(qs[0]["Type"]))
			require.Equal(t, "1", fmt.Sprint(qs[0]["Class"]))
		}
		if e.Protocol == "http" {
			requests = append(requests, strings.SplitN(string(e.Raw), "\r\n", 2)[0])
			if a.HTTPTransaction {
				require.Equal(t, a.ExpectedSources[i], e.Source)
				require.Equal(t, a.ExpectedSources[1-i], e.Destination)
				require.Equal(t, a.NativeRaw[i], hex.EncodeToString(e.Raw))
				require.Equal(t, i, e.Direction)
			} else {
				require.Equal(t, "192.0.2.1:40000", e.Source)
				require.Equal(t, "192.0.2.2:80", e.Destination)
			}
		}
	}
	sort.Strings(domains)
	want := append([]string(nil), a.Domains...)
	sort.Strings(want)
	require.Equal(t, want, domains)
	sort.Strings(requests)
	http := append([]string(nil), a.HTTPRequests...)
	sort.Strings(http)
	require.Equal(t, http, requests)
	if a.HTTPTransaction {
		require.Equal(t, es[0].FlowID, es[1].FlowID)
		require.Equal(t, es[0].ID, es[1].ResponseTo)
		require.Equal(t, es[0].ID, es[1].TransactionID)
		fields, err := es[1].GetFields()
		require.NoError(t, err)
		assertMVPJSONFields(t, map[string]any{"Message": map[string]any{"HTTP Response": map[string]any{"FirstLine": map[string]any{"Version": "HTTP/1.1", "Status": "200", "Message": "OK"}, "Headers": []string{"Content-Length: 5", ""}, "Body": map[string]any{"Octets": "hello"}}}}, fields)
	}
	if a.FlowIsolation {
		require.Len(t, flows, len(a.Domains))
	}
}

func incrementalMVPBudget(t *testing.T, c incrementalMVPCase, a incrementalMVPAnswer, wire []byte) {
	t.Helper()
	if c.Group == "mavlink" {
		r, err := pcapgo.NewReader(bytes.NewReader(wire))
		require.NoError(t, err)
		errorIndex := 0
		for {
			b, _, err := r.ReadPacketData()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			p := gopacket.NewPacket(b, r.LinkType(), gopacket.Default)
			udp := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
			messages, err := decodeMAVLinkDatagram(udp.Payload, a.MaxMessage, a.MaxMessages)
			if len(a.ErrorKinds) > 0 {
				require.Error(t, err)
				require.Nil(t, messages)
				require.Less(t, errorIndex, len(a.ErrorKinds))
				require.Equal(t, a.ErrorKinds[errorIndex], string(err.(*ProtocolError).Kind))
				errorIndex++
			} else {
				require.NoError(t, err)
			}
		}
		require.Equal(t, len(a.ErrorKinds), errorIndex)
	}
	if a.Budget == nil && a.ExpectedOutstanding == nil {
		return
	}
	for _, deferred := range []bool{false, true} {
		options := []ProtocolSessionOption{WithSessionTransport(a.Transport), WithSessionPorts(41707, a.Port)}
		if a.ClientDirection != nil {
			options = append(options, WithSessionClientDirection(*a.ClientDirection))
		}
		budget := DefaultParserBudget()
		if a.Budget != nil {
			budget = *a.Budget
		}
		s, err := NewProtocolSessionWithOptions(budget, options...)
		require.NoError(t, err)
		s.(*captureSession).f.a.config.Deferred = deferred
		var es []*ProtocolEvent
		for i, step := range a.Steps {
			b, err := hex.DecodeString(step.Hex)
			require.NoError(t, err)
			result := s.Feed(step.Dir, time.Unix(1700000000, int64(i)), b)
			es = append(es, result.Events...)
			for j := range b {
				b[j] = 0
			}
		}
		if a.ExpectedOutstanding != nil {
			require.NotNil(t, s.(*captureSession).f.atg)
			require.Equal(t, *a.ExpectedOutstanding, s.(*captureSession).f.atg.outstanding(), "framing a rejected direction must preserve its request before Close")
		}
		es = append(es, s.Close("fixture-end")...)
		require.Zero(t, s.Stats().BufferedBytes)
		normalized := incrementalMVPNormalized(t, es)
		budgetAnswer := a
		budgetAnswer.NativeMessages = nil
		if a.Budget != nil {
			budgetAnswer.NativeTerminalError = nil
		}
		if budgetAnswer.NativeRaw != nil {
			budgetAnswer.NativeRaw = budgetAnswer.NativeRaw[:len(budgetAnswer.ExpectedMessages)]
		}
		budgetAnswer.Budget = nil
		if c.Group == "nmea" || c.Group == "semtech" {
			budgetAnswer.TerminalError = string(ErrResourceExceeded)
		}
		incrementalMVPMessages(t, c, budgetAnswer, normalized)
		if c.Group == "nmea" || c.Group == "semtech" {
			found := false
			for _, e := range normalized {
				found = found || e.ExpertCode == string(ErrResourceExceeded)
			}
			require.True(t, found, "typed datagram collection limit")
		}
	}
}

// Raw order is specified alongside the independent field answers. It is not
// inferred by comparing one parser configuration with another.
func incrementalMVPRawOracle(t *testing.T, raw json.RawMessage, es []*ProtocolEvent) {
	t.Helper()
	var a struct {
		Protocol string   `json:"expected_protocol"`
		Count    int      `json:"message_count"`
		Raw      []string `json:"exact_raw_hex"`
	}
	require.NoError(t, json.Unmarshal(raw, &a))
	if a.Protocol == "" {
		return
	}
	require.NotEmpty(t, a.Raw, "every positive signature has an independent source-order oracle")
	var decoded []*ProtocolEvent
	for _, e := range es {
		if e.Protocol == a.Protocol && e.Status == "decoded" {
			decoded = append(decoded, e)
		}
	}
	require.Len(t, decoded, a.Count)
	require.Len(t, a.Raw, a.Count)
	for i, e := range decoded {
		require.Equal(t, a.Raw[i], hex.EncodeToString(e.Raw))
	}
}

// Bind complete independently specified fields, source order and diagnostics.
func assertFTPSSHReviewMVPEvents(t testing.TB, protocol string, raw json.RawMessage, events []*ProtocolEvent) {
	t.Helper()
	var a struct {
		Session     []json.RawMessage `json:"exact_session_fields"`
		Fields      []json.RawMessage `json:"exact_fields"`
		Raw         []string          `json:"native_raw_hex"`
		Terminal    string            `json:"expected_terminal_error"`
		Diagnostics *int              `json:"expected_diagnostic_count"`
	}
	require.NoError(t, json.Unmarshal(raw, &a))
	var decoded []*ProtocolEvent
	terminal := false
	diagnostics := 0
	for _, e := range events {
		require.Equal(t, protocol, e.Protocol, "controls must not escape to a different profile")
		if e.Status == "decoded" || e.Status == "deferred" {
			decoded = append(decoded, e)
			require.Empty(t, e.Error)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
		} else {
			diagnostics++
			if a.Terminal == "IncompleteMessage" {
				require.Equal(t, "incomplete", e.Status)
				terminal = true
			} else if a.Terminal != "" {
				require.Equal(t, a.Terminal, e.ExpertCode, "complete diagnostic taxonomy")
				terminal = true
			}
			if a.Terminal == "ContextRequired" {
				require.NotEqual(t, "malformed", e.Status, "unobserved role is not invalid wire")
			}
		}
	}
	require.Len(t, decoded, len(a.Session))
	require.Len(t, a.Fields, len(decoded))
	require.Len(t, a.Raw, len(decoded))
	for i, e := range decoded {
		require.Equal(t, a.Raw[i], hex.EncodeToString(e.Raw))
		wire, err := json.Marshal(e.Session)
		require.NoError(t, err)
		require.JSONEq(t, string(a.Session[i]), string(wire), "complete Session oracle")
		fields, err := e.GetFields()
		require.NoError(t, err)
		wire, err = json.Marshal(fields)
		require.NoError(t, err)
		require.JSONEq(t, string(a.Fields[i]), string(wire), "complete GetFields oracle")
	}
	require.NotNil(t, a.Diagnostics, "complete diagnostic count oracle")
	require.Equal(t, *a.Diagnostics, diagnostics, "complete diagnostic collection")
	if a.Terminal != "" {
		require.True(t, terminal, "expected terminal %s", a.Terminal)
	} else {
		require.Len(t, events, len(decoded), "control must have no extra error/incomplete diagnostics")
	}
}

// Historical synthetic captures are immutable, including their invalid ACK=0
// carriers. The new carrier binds the same application bytes/answers and keeps
// those application assertions intact while testing a legal opening handshake.
type correctedCarrier struct {
	Group                 string `json:"group"`
	OriginalID            string `json:"original_id"`
	Capture               string `json:"capture"`
	SHA256                string `json:"sha256"`
	Answer                string `json:"answer"`
	AnswerSHA256          string `json:"answer_sha256"`
	OriginalCaptureSHA256 string `json:"original_capture_sha256"`
	OriginalAnswerSHA256  string `json:"original_answer_sha256"`
}

func correctedIncrementalCarrier(t *testing.T, c incrementalMVPCase, original, answer []byte) []byte {
	t.Helper()
	manifest, err := trafficfixture.ReadFile("industrial-link-core/carriers/manifest.json")
	require.NoError(t, err)
	var inventory struct{ Cases []correctedCarrier }
	require.NoError(t, json.Unmarshal(manifest, &inventory))
	require.Len(t, inventory.Cases, 38)
	for _, v := range inventory.Cases {
		if v.Group != c.Group || v.OriginalID != c.ID {
			continue
		}
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(original)), v.OriginalCaptureSHA256)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(answer)), v.OriginalAnswerSHA256)
		wire, err := trafficfixture.ReadFile("industrial-link-core/" + v.Capture)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wire)), v.SHA256)
		oracle, err := trafficfixture.ReadFile("industrial-link-core/" + v.Answer)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(oracle)), v.AnswerSHA256)
		var a struct {
			ApplicationAnswer json.RawMessage `json:"application_answer"`
		}
		require.NoError(t, json.Unmarshal(oracle, &a))
		require.JSONEq(t, string(answer), string(a.ApplicationAnswer))
		return wire
	}
	return original
}
