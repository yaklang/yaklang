package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPUnknownMethodRetainsUnframedObservation(t *testing.T) {
	for _, initial := range []bool{false, true} {
		for _, kind := range []string{"reassembled", "decrypted"} {
			t.Run(fmt.Sprintf("initial=%t/%s", initial, kind), func(t *testing.T) {
				var s ProtocolSession
				if initial {
					var err error
					s, err = NewProtocolSession(DefaultParserBudget())
					require.NoError(t, err)
				} else {
					s = httpWaitSession(t, DefaultParserBudget())
				}
				f := s.(*captureSession).f
				f.byteSource = kind
				f.domain = CaptureDomain{Section: 3, Interface: 4, Encapsulation: "/vlan:80"}
				header := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nX-Repeat: one\r\nX-Repeat: two\r\n\r\n"
				// This could be GET's body or HEAD/CONNECT's following bytes.
				// It is not safe to consume 2 bytes or scan for another HTTP line.
				tail := []byte("OKHTTP/1.1 204 No Content\r\n\r\n\x00\xff")
				wire := append([]byte(header), tail...)
				d := &f.directions[1]
				base := d.offset
				refs := []PacketReference{{Number: 17, Domain: f.domain}, {Number: 19, Domain: f.domain}}
				require.True(t, f.a.reserveEvidence(160))
				d.contributions = append(d.contributions,
					contribution{start: base, end: base + uint64(len(header)), ref: refs[0], parent: 117},
					contribution{start: base + uint64(len(header)), end: base + uint64(len(wire)), ref: refs[1], parent: 119})
				f.parentID = 999 // An arrival/context marker cannot become a byte contributor.
				r := s.Feed(1, time.Unix(200, 0), wire)
				events := append(r.Events, s.Close("eof")...)
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, header, string(e.Raw), "existing header-only API remains exact")
				require.Equal(t, "headers", e.Completeness)
				require.Equal(t, string(ErrContextRequired), e.ExpertCode)
				require.Equal(t, "http-response-header-observation", e.Profile)
				require.Equal(t, "complete-header-without-request-method", e.Admission)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				require.Nil(t, e.SourceBytes.CompletedIn)
				require.Equal(t, []PacketReference{refs[0]}, e.SourceBytes.PacketRefs)
				require.Equal(t, false, e.Session["Request Context Available"])
				require.Equal(t, false, e.Session["Body Boundary Known"])
				require.NotContains(t, e.Session, "Method")
				require.Equal(t, len(wire), e.Session["Retained Stream Bytes"])
				require.Equal(t, len(tail), e.Session["Unframed Stream Bytes"])
				o := e.Session["Unframed Stream Observation"].(map[string]any)
				require.Equal(t, tail, o["Unframed Stream Prefix"])
				require.Equal(t, len(tail), o["Prefix Bytes"])
				require.Equal(t, false, o["Preview Limited"])
				require.Equal(t, base+uint64(len(header)), o["Offset"])
				require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(tail)), o["Prefix SHA256"])
				require.Equal(t, o["Prefix SHA256"], o["Observed SHA256"])
				require.Equal(t, kind, o["Source Kind"])
				require.Equal(t, []PacketReference{refs[1]}, o["Packet References"])
				require.Equal(t, []uint64{119}, o["Parent PDUs"])
				require.NotContains(t, o, "Completed In")
				fields, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, []string{"one", "two"}, fields["Headers"].(map[string]any)["X-Repeat"])
				require.NotContains(t, fields, "Body")
				// Retained observation and query snapshots do not alias input or
				// public maps. Header and opaque tail remain separate sources.
				wire[len(header)] ^= 0xff
				require.Equal(t, tail, o["Unframed Stream Prefix"])
				e.Session["Body Boundary Known"] = true
				o["Unframed Stream Prefix"].([]byte)[0] ^= 0xff
				e.Fields = map[string]any{"Body": "forged", "Status Code": 204}
				queried, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, 200, queried["Status Code"])
				require.NotContains(t, queried, "Body")
				require.Equal(t, []any{uint64(200)}, e.DisplayFields()["http.response.code"])
				require.NotContains(t, e.DisplayFields(), "http.request.method")
				decoded, err := e.Decode()
				require.NoError(t, err)
				context := decoded["session"].(map[string]any)
				require.Equal(t, false, context["Body Boundary Known"])
				require.Equal(t, tail, context["Unframed Stream Observation"].(map[string]any)["Unframed Stream Prefix"])
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestHTTPUnknownMethodObservationPreviewAndBudget(t *testing.T) {
	for _, deny := range []bool{false, true} {
		t.Run(fmt.Sprintf("deny-extra-evidence=%t", deny), func(t *testing.T) {
			s := httpWaitSession(t, DefaultParserBudget())
			f := s.(*captureSession).f
			header := "HTTP/1.1 200 OK\r\nContent-Length: 99999\r\n\r\n"
			tail := bytes.Repeat([]byte{0, 0xff, 'H'}, (httpUnframedPreviewLimit/3)+100)
			r := s.Feed(1, time.Unix(200, 0), append([]byte(header), tail...))
			require.Empty(t, r.Events)
			var charge int64
			if deny {
				charge = int64(f.a.config.MaxBufferedBytes) - f.a.buffered.Load()
				require.True(t, f.a.reserveEvidence(charge))
			}
			es := s.Close("eof")
			if deny {
				f.a.buffered.Add(-charge)
			}
			require.Len(t, es, 1)
			e := es[0]
			require.Equal(t, header, string(e.Raw))
			require.Equal(t, len(tail), e.Session["Unframed Stream Bytes"])
			require.Equal(t, "context-required", e.Status)
			require.NotEqual(t, "HTTPPayloadOmitted", e.ExpertCode)
			if deny {
				require.NotContains(t, e.Session, "Unframed Stream Observation")
				require.Equal(t, "capture memory budget exhausted", e.Session["Unframed Preview Unavailable"])
			} else {
				o := e.Session["Unframed Stream Observation"].(map[string]any)
				require.Equal(t, tail[:httpUnframedPreviewLimit], o["Unframed Stream Prefix"])
				require.Equal(t, true, o["Preview Limited"])
				require.Equal(t, httpUnframedPreviewLimit, o["Prefix Bytes"])
				require.Equal(t, len(tail), o["Observed Bytes"])
				require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(tail)), o["Observed SHA256"])
				require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(tail[:httpUnframedPreviewLimit])), o["Prefix SHA256"])
			}
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestHTTPKnownMethodDoesNotBecomeUnframedObservation(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "CONNECT"} {
		t.Run(method, func(t *testing.T) {
			s := httpWaitSession(t, DefaultParserBudget())
			uri := "/known"
			if method == "CONNECT" {
				uri = "example.test:443"
			}
			request := method + " " + uri + " HTTP/1.1\r\nHost: example.test\r\n\r\n"
			r := s.Feed(0, time.Unix(200, 0), []byte(request))
			require.Nil(t, r.Err)
			response := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n"
			if method == "GET" {
				response += "OK"
			}
			r = s.Feed(1, time.Unix(201, 0), []byte(response))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, response, string(r.Events[0].Raw))
			require.NotEqual(t, "http-response-header-observation", r.Events[0].Profile)
			require.NotContains(t, r.Events[0].Session, "Unframed Stream Observation")
			require.False(t, strings.Contains(r.Events[0].Summary, "unframed"))
			s.Close("eof")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestHTTPUnknownMethodLaterArrivalsRemainOwnedUnframedBytes(t *testing.T) {
	for _, kind := range []string{"reassembled", "decrypted"} {
		t.Run(kind, func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			f := s.(*captureSession).f
			f.byteSource = kind
			f.domain = CaptureDomain{Section: 2, Interface: 3, Encapsulation: "/vlan:81"}
			header := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n"
			r := s.Feed(1, time.Unix(100, 0), []byte(header))
			require.Len(t, r.Events, 1)
			headerID := r.Events[0].ID
			base := uint64(len(header))
			for i, tail := range [][]byte{[]byte("OK"), []byte("HTTP/1.1 204 No Content\r\n\r\n\xff\x00"), []byte("GET /embedded HTTP/1.1\r\nHost: example.test\r\n\r\n")} {
				ref := PacketReference{Number: uint64(21 + i), Domain: f.domain}
				require.True(t, f.a.reserveEvidence(80))
				f.directions[1].contributions = append(f.directions[1].contributions, contribution{start: base, end: base + uint64(len(tail)), ref: ref, parent: uint64(121 + i)})
				r = s.Feed(1, time.Unix(101+int64(i), 0), tail)
				require.Len(t, r.Events, 1, "a stopped framer must not silently discard later ordered bytes")
				e := r.Events[0]
				require.Equal(t, "http-unframed-stream-observation", e.Profile)
				require.Equal(t, "context-required", e.Status)
				require.Equal(t, string(ErrContextRequired), e.ExpertCode)
				require.Equal(t, tail, e.Raw)
				require.Equal(t, len(tail), e.Length)
				require.Equal(t, base, e.Offset)
				require.Equal(t, []PacketReference{ref}, e.SourceBytes.PacketRefs)
				require.Equal(t, []uint64{uint64(121 + i)}, e.SourceBytes.ParentPDUs)
				require.Nil(t, e.SourceBytes.CompletedIn)
				require.Equal(t, headerID, e.Session["Observed Header PDU"])
				require.Equal(t, "HTTP/1.1", e.Session["Observed Header Protocol"])
				require.Equal(t, false, e.Session["Body Boundary Known"])
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				require.NotContains(t, e.DisplayFields(), "http.response.code")
				require.NotContains(t, e.DisplayFields(), "http.request.method")
				_, err = e.GetFields()
				require.Error(t, err, "an unframed arrival has no complete HTTP field grammar")
				e.Fields, e.Structured, e.Protocol, e.Status = map[string]any{"Body": "forged"}, map[string]any{"fields": map[string]any{"Body": "forged"}}, "http", "decoded"
				_, err = e.GetFields()
				require.Error(t, err, "public cache edits cannot turn an observation into a message")
				require.NotContains(t, e.DisplayFields(), "http.response.code")
				require.NotContains(t, e.DisplayFields(), "http.request.method")
				base += uint64(len(tail))
				require.Zero(t, s.Stats().BufferedBytes)
			}
			require.Empty(t, s.Close("eof"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestHTTPUnframedLineageSurvivesPublicCallbackEdits(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	f := s.(*captureSession).f
	callback := f.a.config.OnEvent
	var nativeID uint64
	f.a.config.OnEvent = func(e *ProtocolEvent) {
		if e.Profile == "http-response-header-observation" {
			nativeID = e.ID
			e.ID, e.Raw[0] = 99999, 'X'
		}
		callback(e)
	}
	header := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n")
	r := s.Feed(1, time.Unix(100, 0), header)
	require.Len(t, r.Events, 1)
	require.NotZero(t, nativeID)
	r = s.Feed(1, time.Unix(101, 0), []byte("OK"))
	require.Len(t, r.Events, 1)
	require.Equal(t, nativeID, r.Events[0].Session["Observed Header PDU"])
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(header)), r.Events[0].Session["Observed Header SHA256"])
	// A later request cannot retroactively turn this initial orphan into a
	// known HEAD/CONNECT body. The caller must start a new physical generation.
	s.Feed(0, time.Unix(102, 0), []byte("HEAD /late HTTP/1.1\r\nHost: example.test\r\n\r\n"))
	r = s.Feed(1, time.Unix(103, 0), []byte("HTTP/1.1 204 No Content\r\n\r\n"))
	require.Len(t, r.Events, 1)
	require.Equal(t, "http-unframed-stream-observation", r.Events[0].Profile)
	require.Zero(t, r.Events[0].ResponseTo)
	s.Close("eof")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestHTTPUnframedArrivalResourceRefusalRetiresContinuation(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	f := s.(*captureSession).f
	r := s.Feed(1, time.Unix(100, 0), []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n"))
	require.Len(t, r.Events, 1)
	charge := int64(f.a.config.MaxBufferedBytes) - f.a.buffered.Load()
	require.True(t, f.a.reserveEvidence(charge))
	r = s.Feed(1, time.Unix(101, 0), []byte("OK"))
	f.a.buffered.Add(-charge)
	require.Len(t, r.Events, 1)
	require.Equal(t, "limited", r.Events[0].Status)
	require.Equal(t, string(ErrResourceExceeded), r.Events[0].ExpertCode)
	require.Empty(t, r.Events[0].Raw)
	require.Equal(t, 2, r.Events[0].Length)
	require.Equal(t, "capture memory budget exhausted", r.Events[0].Session["Preview Unavailable"])
	require.Zero(t, f.directions[1].httpUnframedHeaderID)
	require.Empty(t, s.Feed(1, time.Unix(102, 0), []byte("later")).Events)
	s.Close("eof")
	require.Zero(t, s.Stats().BufferedBytes)
}

func FuzzHTTPUnframedResponseObservation(f *testing.F) {
	f.Add([]byte("OK"), uint8(1), true)
	f.Add([]byte("HTTP/1.1 204 No Content\r\n\r\n"), uint8(3), true)
	f.Add([]byte{0, 0xff, 0xc3, 0x28, 0}, uint8(2), false)
	f.Fuzz(func(t *testing.T, tail []byte, step uint8, initial bool) {
		if len(tail) > 2*httpUnframedPreviewLimit {
			return
		}
		var s ProtocolSession
		if initial {
			var err error
			s, err = NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
		} else {
			s = httpWaitSession(t, DefaultParserBudget())
		}
		header := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n")
		got := s.Feed(1, time.Unix(100, 0), header)
		all := append([]*ProtocolEvent(nil), got.Events...)
		chunk := 1 + int(step)
		for at := 0; at < len(tail); at += chunk {
			r := s.Feed(1, time.Unix(101, 0), tail[at:min(len(tail), at+chunk)])
			all = append(all, r.Events...)
		}
		all = append(all, s.Close("eof")...)
		var observed []byte
		for _, e := range all {
			require.Zero(t, e.ResponseTo)
			require.Nil(t, e.SourceBytes.CompletedIn)
			require.Equal(t, "context-required", e.Status)
			require.Equal(t, false, e.Session["Body Boundary Known"])
			switch e.Profile {
			case "http-response-header-observation":
				require.Equal(t, header, e.Raw)
				if m, ok := e.Session["Unframed Stream Observation"].(map[string]any); ok {
					prefix := m["Unframed Stream Prefix"].([]byte)
					require.Equal(t, tail[:len(prefix)], prefix)
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(tail)), m["Observed SHA256"])
					observed = append(observed, prefix...)
				}
			case "http-unframed-stream-observation":
				require.Equal(t, len(e.Raw), e.Length)
				require.NotContains(t, e.DisplayFields(), "http.response.code")
				require.NotContains(t, e.DisplayFields(), "http.request.method")
				observed = append(observed, e.Raw...)
			default:
				t.Fatalf("unframed bytes acquired a protocol message: %s", e.Profile)
			}
		}
		want := tail
		if !initial && len(want) > httpUnframedPreviewLimit {
			want = want[:httpUnframedPreviewLimit]
		}
		require.True(t, bytes.Equal(want, observed))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func TestHTTPUnframedCaptureReorderingAndReusedGeneration(t *testing.T) {
	header := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n"
	tail := "OKHTTP/1.1 204 No Content\r\n\r\n"
	request := "HEAD /new-generation HTTP/1.1\r\nHost: example.test\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Length: 99999\r\n\r\n"
	for _, order := range []string{"whole", "split", "out-of-order-retransmit"} {
		steps := []tcpStep{{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500}}
		steps = append(steps, tcpStep{seq: 500, ack: 100, reverse: true, data: header})
		at := uint32(500 + len(header))
		switch order {
		case "whole":
			steps = append(steps, tcpStep{seq: at, ack: 100, reverse: true, data: tail})
		case "split":
			steps = append(steps, tcpStep{seq: at, ack: 100, reverse: true, data: tail[:2]}, tcpStep{seq: at + 2, ack: 100, reverse: true, data: tail[2:]})
		case "out-of-order-retransmit":
			steps = append(steps, tcpStep{seq: at + 2, ack: 100, reverse: true, data: tail[2:]}, tcpStep{seq: at, ack: 100, reverse: true, data: tail[:2]}, tcpStep{seq: at + 2, ack: 100, reverse: true, data: tail[2:]})
		}
		steps = append(steps,
			tcpStep{seq: 100, ack: at + uint32(len(tail)), fin: true}, tcpStep{seq: at + uint32(len(tail)), ack: 101, reverse: true, fin: true},
			tcpStep{seq: 9000, syn: true}, tcpStep{seq: 12000, ack: 9001, reverse: true, syn: true, synack: true}, tcpStep{seq: 9001, ack: 12001},
			tcpStep{seq: 9001, ack: 12001, data: request}, tcpStep{seq: 12001, ack: 9001 + uint32(len(request)), reverse: true, data: response},
			tcpStep{seq: 9001 + uint32(len(request)), ack: 12001 + uint32(len(response)), fin: true}, tcpStep{seq: 12001 + uint32(len(response)), ack: 9002 + uint32(len(request)), reverse: true, fin: true})
		for _, workers := range []int{1, 2} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/w%d/deferred%t", order, workers, deferred), func(t *testing.T) {
					es, stats, err := binReplay(t, binTestPcap(t, steps, 8080, false, true), workers, WithProtocolDeferred(deferred))
					require.NoError(t, err)
					var observed []byte
					var first *ProtocolEvent
					var complete []*ProtocolEvent
					for _, e := range es {
						switch e.Profile {
						case "http-response-header-observation":
							require.Nil(t, first)
							first = e
							require.Equal(t, header, string(e.Raw))
						case "http-unframed-stream-observation":
							require.NotNil(t, first)
							require.Equal(t, first.FlowID, e.FlowID)
							require.Equal(t, first.TCPFlowID, e.TCPFlowID)
							require.Equal(t, first.ID, e.Session["Observed Header PDU"])
							require.Nil(t, e.SourceBytes.CompletedIn)
							observed = append(observed, e.Raw...)
						default:
							if e.Protocol == "http" && (e.Status == "decoded" || e.Status == "deferred") {
								complete = append(complete, e)
							}
						}
						if e.Profile == "http-unframed-stream-observation" {
							require.Zero(t, e.ResponseTo)
							_, err := e.GetFields()
							require.Error(t, err)
						}
					}
					require.NotNil(t, first)
					require.Equal(t, tail, string(observed), "retransmitted or reordered bytes are observed exactly once")
					require.Len(t, complete, 2)
					require.Equal(t, request, string(complete[0].Raw))
					require.Equal(t, response, string(complete[1].Raw), "HEAD has no body despite the large representation length")
					require.Equal(t, complete[0].ID, complete[1].ResponseTo)
					require.NotEqual(t, first.FlowID, complete[0].FlowID)
					require.NotEqual(t, first.TCPFlowID, complete[0].TCPFlowID)
					require.NotContains(t, complete[1].Session, "Observed Header PDU")
					require.Zero(t, stats.BufferedBytes)
				})
			}
		}
	}
}
