package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func phaResponseTestFlow(t *testing.T, suite uint16, server int) (*binFlow, *binTLS) {
	t.Helper()
	f, s := phaContextTestFlow(t, suite, server, "offered")
	// Model both observed initial Finished boundaries using public test
	// constants. These tests verify association, not transcript MACs or PKI.
	for _, dir := range []int{server, 1 - server} {
		key, iv, secret, sequence := s.keys[dir], s.iv[dir], s.secret[dir], s.seq[dir]
		s.app[dir] = false
		_, err := s.handshakeMessage(dir, finishedTestMessage(phaResponseTestHashSize(suite)), true)
		require.NoError(t, err)
		s.keys[dir], s.iv[dir], s.secret[dir], s.seq[dir] = key, iv, secret, sequence
	}
	return f, s
}

func phaResponseTestHashSize(suite uint16) int {
	if suite == 0x1302 {
		return 48
	}
	return 32
}

func phaResponseTestCertificate(context []byte, empty bool) []byte {
	if empty {
		return reliabilityHS(11, certificateExtensionsBody(context))
	}
	return reliabilityHS(11, certificateExtensionsBody(context, nil))
}

func phaResponseTestSend(t *testing.T, f *binFlow, s *binTLS, dir int, encoded []byte, kind byte) (*ProtocolEvent, error) {
	t.Helper()
	e := &ProtocolEvent{Protocol: "tls", Status: "decoded", Raw: boundaryTLSRecord(t, s, dir, s.seq[dir], encoded, kind, 0)}
	_, err := f.consumeTLS(dir, e)
	return e, err
}

func phaResponseTestAssociation(t *testing.T, message map[string]any) map[string]any {
	t.Helper()
	value, ok := message["Post Handshake Response"].(map[string]any)
	require.True(t, ok, "missing typed post-handshake response observation")
	return value
}

func TestTLS13PHAResponseContextAndProgression(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, mode := range []string{"certificate", "empty", "reverse-two-requests", "multiple-requests-one-record"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, mode), func(t *testing.T) {
					f, s := phaResponseTestFlow(t, suite, server)
					client := 1 - server
					contexts := [][]byte{{0xa1}, {0xb2}}
					if mode == "multiple-requests-one-record" {
						e, err := phaResponseTestSend(t, f, s, server, append(phaContextTestRequest(contexts[0]), phaContextTestRequest(contexts[1])...), 22)
						require.NoError(t, err)
						messages := e.Session["Handshake Messages"].([]map[string]any)
						require.Len(t, messages, 2)
						for i, message := range messages {
							require.Equal(t, uint64(i+1), message["Certificate Request Admission"].(map[string]any)["Observed Request Count"])
						}
					} else {
						for _, context := range contexts {
							_, err := phaResponseTestSend(t, f, s, server, phaContextTestRequest(context), 22)
							require.NoError(t, err)
						}
					}
					indices := []int{0, 1}
					if mode == "reverse-two-requests" {
						indices = []int{1, 0}
					}
					for _, index := range indices {
						empty := mode == "empty"
						encoded := phaResponseTestCertificate(contexts[index], empty)
						certificate, err := phaResponseTestSend(t, f, s, client, encoded, 22)
						require.NoError(t, err)
						message := certificate.Session["Handshake Messages"].([]map[string]any)[0]
						association := phaResponseTestAssociation(t, message)
						require.Equal(t, uint64(index+1), association["Request Ordinal"])
						require.Equal(t, "matched observed CertificateRequest", association["Context Association"])
						wantStage, wantSelection := "awaiting CertificateVerify", "certificate list supplied"
						if empty {
							wantStage, wantSelection = "awaiting Finished", "empty certificate list (declined)"
						}
						require.Equal(t, wantStage, association["Response Stage"])
						require.Equal(t, wantSelection, association["Certificate Selection"])
						// Public decoded context and encoded bytes cannot rewrite the
						// private association, even before CV/Finished arrive.
						clear(message["Certificate Body"].(map[string]any)["Context"].([]byte))
						clear(message["Encoded Certificate"].([]byte))
						if !empty {
							verify, err := phaResponseTestSend(t, f, s, client, certificateVerifyTestMessage(0x304, 0x0804, []byte{1, 2}), 22)
							require.NoError(t, err)
							association = phaResponseTestAssociation(t, verify.Session["Handshake Messages"].([]map[string]any)[0])
							require.Equal(t, uint64(index+1), association["Request Ordinal"])
							require.Equal(t, "awaiting Finished", association["Response Stage"])
						}
						key, iv, secret, sequence := s.keys[client], bytes.Clone(s.iv[client]), bytes.Clone(s.secret[client]), s.seq[client]
						finished, err := phaResponseTestSend(t, f, s, client, finishedTestMessage(phaResponseTestHashSize(suite)), 22)
						require.NoError(t, err)
						association = phaResponseTestAssociation(t, finished.Session["Handshake Messages"].([]map[string]any)[0])
						require.Equal(t, uint64(index+1), association["Request Ordinal"])
						require.Equal(t, "response block complete; transcript not verified", association["Response Stage"])
						require.Equal(t, secret, s.secret[client])
						require.Equal(t, iv, s.iv[client])
						require.Same(t, key, s.keys[client])
						require.Equal(t, sequence+1, s.seq[client])
					}
					later, err := phaResponseTestSend(t, f, s, client, []byte("GET /after HTTP/1.1\r\n\r\n"), 23)
					require.NoError(t, err)
					require.NotContains(t, later.Session, "Handshake Messages")
					require.NotContains(t, later.DisplayFields(), "tls.pha.response_stage")
				})
			}
		}
	}
}

func TestTLS13PHAResponseRejectsContinuouslyObservedViolation(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, mode := range []string{"unknown-context", "past-response-context", "verify-after-empty", "finished-before-verify", "second-certificate", "application-interleave", "keyupdate-interleave", "orphan-verify", "orphan-finished"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, mode), func(t *testing.T) {
					f, s := phaResponseTestFlow(t, suite, server)
					client, context := 1-server, []byte{0xa1}
					_, err := phaResponseTestSend(t, f, s, server, phaContextTestRequest(context), 22)
					require.NoError(t, err)
					encoded, kind, reason := phaResponseTestCertificate(context, true), byte(22), ""
					if mode == "unknown-context" {
						encoded, reason = phaResponseTestCertificate([]byte{0xff}, false), "unobserved TLS CertificateRequest context"
					} else if mode == "orphan-verify" {
						encoded, reason = certificateVerifyTestMessage(0x304, 0x0804, []byte{1}), "TLS CertificateVerify without an observed Certificate"
					} else if mode == "orphan-finished" {
						encoded, reason = finishedTestMessage(phaResponseTestHashSize(suite)), "TLS post-handshake Finished without an observed Certificate"
					} else {
						empty := mode != "finished-before-verify"
						_, err = phaResponseTestSend(t, f, s, client, phaResponseTestCertificate(context, empty), 22)
						require.NoError(t, err)
						switch mode {
						case "past-response-context":
							_, err = phaResponseTestSend(t, f, s, client, finishedTestMessage(phaResponseTestHashSize(suite)), 22)
							require.NoError(t, err)
							reason = "repeated TLS Certificate response context"
						case "verify-after-empty":
							encoded, reason = certificateVerifyTestMessage(0x304, 0x0804, []byte{1}), "TLS CertificateVerify after an empty Certificate"
						case "finished-before-verify":
							encoded, reason = finishedTestMessage(phaResponseTestHashSize(suite)), "TLS post-handshake Finished before CertificateVerify"
						case "second-certificate":
							reason = "TLS post-handshake response interleaved"
						case "application-interleave":
							encoded, kind, reason = []byte("GET /bad HTTP/1.1\r\n\r\n"), 23, "TLS post-handshake response interleaved"
						case "keyupdate-interleave":
							encoded, reason = []byte{24, 0, 0, 1, 0}, "TLS post-handshake response interleaved"
						}
					}
					key, iv, secret, sequence := s.keys[client], bytes.Clone(s.iv[client]), bytes.Clone(s.secret[client]), s.seq[client]
					event, err := phaResponseTestSend(t, f, s, client, encoded, kind)
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
					require.Contains(t, typed.Error(), reason)
					require.Equal(t, true, event.Session["Authentication Verified"])
					require.NotContains(t, event.Session, "Handshake Messages")
					require.Equal(t, secret, s.secret[client])
					require.Equal(t, iv, s.iv[client])
					require.Same(t, key, s.keys[client])
					require.Equal(t, sequence+1, s.seq[client])
				})
			}
		}
	}
}

func TestTLS13PHAResponseMissingHistoryDoesNotInventViolation(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, mode := range []string{"server-gap-matched", "server-gap-unobserved", "client-gap-prefix", "client-gap-new-certificate", "initial-finished-unobserved", "keyupdate-before-response"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, mode), func(t *testing.T) {
					f, s := phaResponseTestFlow(t, suite, server)
					client, context := 1-server, []byte{0xa1}
					_, err := phaResponseTestSend(t, f, s, server, phaContextTestRequest(context), 22)
					require.NoError(t, err)
					if mode == "client-gap-prefix" || mode == "client-gap-new-certificate" {
						_, err = phaResponseTestSend(t, f, s, client, phaResponseTestCertificate(context, false), 22)
						require.NoError(t, err)
						// An explicit unavailable-plaintext observation invalidates
						// association, not the earlier authenticated Certificate fact.
						// This does not claim to recover a missing message boundary.
						f.markTLSInnerGap(client)
						if mode == "client-gap-prefix" {
							for _, encoded := range [][]byte{certificateVerifyTestMessage(0x304, 0x0804, []byte{1}), finishedTestMessage(phaResponseTestHashSize(suite))} {
								e, err := phaResponseTestSend(t, f, s, client, encoded, 22)
								require.NoError(t, err)
								association := phaResponseTestAssociation(t, e.Session["Handshake Messages"].([]map[string]any)[0])
								require.Equal(t, "response prefix unavailable", association["Response Stage"])
								require.NotContains(t, association, "Request Ordinal")
							}
							return
						}
						context = []byte{0xb2}
						_, err = phaResponseTestSend(t, f, s, server, phaContextTestRequest(context), 22)
						require.NoError(t, err)
					} else if mode == "server-gap-matched" || mode == "server-gap-unobserved" {
						f.markTLSInnerGap(server)
						if mode == "server-gap-unobserved" {
							context = []byte{0xff}
						}
					} else if mode == "initial-finished-unobserved" {
						s.phaInitialFinished[server] = false
						context = []byte{0xff}
					} else if mode == "keyupdate-before-response" {
						for _, dir := range []int{client, server} {
							_, err = phaResponseTestSend(t, f, s, dir, []byte{24, 0, 0, 1, 0}, 22)
							require.NoError(t, err)
						}
					}
					certificate, err := phaResponseTestSend(t, f, s, client, phaResponseTestCertificate(context, true), 22)
					require.NoError(t, err)
					association := phaResponseTestAssociation(t, certificate.Session["Handshake Messages"].([]map[string]any)[0])
					if mode == "server-gap-unobserved" || mode == "initial-finished-unobserved" {
						require.Equal(t, "request not observed; association unavailable", association["Context Association"])
						require.NotContains(t, association, "Request Ordinal")
					} else {
						ordinal := uint64(1)
						if mode == "client-gap-new-certificate" {
							ordinal = 2
						}
						require.Equal(t, ordinal, association["Request Ordinal"])
					}
					finished, err := phaResponseTestSend(t, f, s, client, finishedTestMessage(phaResponseTestHashSize(suite)), 22)
					require.NoError(t, err)
					association = phaResponseTestAssociation(t, finished.Session["Handshake Messages"].([]map[string]any)[0])
					require.Equal(t, "response block complete; transcript not verified", association["Response Stage"])
					if mode == "client-gap-new-certificate" {
						_, err = phaResponseTestSend(t, f, s, client, phaResponseTestCertificate([]byte{0xa1}, true), 22)
						require.ErrorContains(t, err, "repeated TLS Certificate response context", "missing later bytes cannot erase a seen Certificate")
					}
				})
			}
		}
	}
}

func TestTLS13PHAResponseBudgetAndBodyRefusalAreTransactional(t *testing.T) {
	for _, mode := range []string{"body", "retained-bytes"} {
		t.Run(mode, func(t *testing.T) {
			f, s := phaResponseTestFlow(t, 0x1301, 1)
			context := []byte{0xa1}
			_, err := phaResponseTestSend(t, f, s, 1, phaContextTestRequest(context), 22)
			require.NoError(t, err)
			request, storage := s.phaRequests[string(context)], s.requestContextStorage()
			encoded := phaResponseTestCertificate(context, true)
			if mode == "body" {
				encoded = reliabilityHS(11, []byte{1, 0xa1, 0, 0, 1})
			} else {
				f.a.config.MaxBufferedBytes = int(f.a.buffered.Load()) + len(encoded) + 80
			}
			sequence := s.seq[0]
			event, err := phaResponseTestSend(t, f, s, 0, encoded, 22)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			wantKind := ErrMalformedMessage
			if mode == "retained-bytes" {
				wantKind = ErrResourceExceeded
			}
			require.Equal(t, wantKind, typed.Kind)
			require.Equal(t, true, event.Session["Authentication Verified"])
			require.NotContains(t, event.Session, "Handshake Messages")
			require.Equal(t, request, s.phaRequests[string(context)])
			require.Equal(t, tlsPHAResponseNone, s.phaResponse.stage)
			require.Equal(t, storage, s.requestContextStorage())
			require.Equal(t, sequence+1, s.seq[0])
			// A production decoder stops at refusal. Only this direct state
			// test discards refused bytes to exercise a subsequent retry.
			s.handshake[0] = nil
			f.a.config.MaxBufferedBytes = 16 << 20
			event, err = phaResponseTestSend(t, f, s, 0, phaResponseTestCertificate(context, true), 22)
			require.NoError(t, err)
			association := phaResponseTestAssociation(t, event.Session["Handshake Messages"].([]map[string]any)[0])
			require.Equal(t, uint64(1), association["Request Ordinal"])
		})
	}
}

func TestTLS13PHAResponseTypedFieldsOwnTheirObservedContext(t *testing.T) {
	f, s := phaResponseTestFlow(t, 0x1301, 1)
	_, err := phaResponseTestSend(t, f, s, 1, phaContextTestRequest([]byte{0xa1}), 22)
	require.NoError(t, err)
	event, err := phaResponseTestSend(t, f, s, 0, phaResponseTestCertificate([]byte{0xa1}, true), 22)
	require.NoError(t, err)
	event.nativeTLSObservation, event.semanticFields = true, cloneSession(event.Session)
	before := event.DisplayFieldValues()
	want := map[string]any{
		"tls.pha.request_index": uint64(1), "tls.pha.response_context": "matched observed CertificateRequest",
		"tls.pha.response_stage": "awaiting Finished", "tls.pha.certificate_selection": "empty certificate list (declined)",
	}
	for name, value := range want {
		require.Equal(t, []any{value}, before.FieldMap()[name])
		count := 0
		for _, field := range before.Values {
			if field.Name == name {
				count++
				require.Equal(t, "context", field.Origin)
				require.Nil(t, field.Range)
			}
		}
		require.Equal(t, 1, count)
	}
	clear(event.Session)
	clear(event.Raw)
	event.Protocol = "http"
	after := event.DisplayFieldValues()
	for name, value := range want {
		require.Equal(t, []any{value}, after.FieldMap()[name])
	}
	for _, field := range after.Values {
		if _, ok := want[field.Name]; ok {
			require.Equal(t, "context", field.Origin)
			require.Nil(t, field.Range)
		}
	}
}

func TestTLS13PHAResponseCoalescedMessagesKeepTheirOrdinals(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, empty := range []bool{false, true} {
			t.Run(fmt.Sprintf("suite%04x/empty%t", suite, empty), func(t *testing.T) {
				f, s := phaResponseTestFlow(t, suite, 1)
				_, err := phaResponseTestSend(t, f, s, 1, append(phaContextTestRequest([]byte{0xa1}), phaContextTestRequest([]byte{0xb2})...), 22)
				require.NoError(t, err)
				// Reverse the two responses. A single TLS record carries each
				// complete block, with Finished at its required record boundary.
				for _, index := range []int{1, 0} {
					block := phaResponseTestCertificate([]byte{[]byte{0xa1, 0xb2}[index]}, empty)
					stages := []any{"awaiting CertificateVerify", "awaiting Finished", "response block complete; transcript not verified"}
					if !empty {
						block = append(block, certificateVerifyTestMessage(0x304, 0x0804, []byte{1})...)
					} else {
						stages = stages[1:]
					}
					block = append(block, finishedTestMessage(phaResponseTestHashSize(suite))...)
					event, err := phaResponseTestSend(t, f, s, 0, block, 22)
					require.NoError(t, err)
					messages := event.Session["Handshake Messages"].([]map[string]any)
					require.Len(t, messages, len(stages))
					indices := make([]any, len(stages))
					for i, message := range messages {
						association := phaResponseTestAssociation(t, message)
						require.Equal(t, uint64(index+1), association["Request Ordinal"])
						require.Equal(t, stages[i], association["Response Stage"])
						indices[i] = uint64(index + 1)
					}
					event.nativeTLSObservation, event.semanticFields = true, cloneSession(event.Session)
					fields := event.DisplayFieldValues().FieldMap()
					require.Equal(t, indices, fields["tls.pha.request_index"])
					require.Equal(t, stages, fields["tls.pha.response_stage"])
				}
			})
		}
	}
}

func TestTLS13PHAResponseUninterpretedPrefixStaysUnavailable(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, kind := range []byte{25, 255} {
			t.Run(fmt.Sprintf("suite%04x/type%d", suite, kind), func(t *testing.T) {
				f, s := phaResponseTestFlow(t, suite, 1)
				_, err := phaResponseTestSend(t, f, s, 1, phaContextTestRequest([]byte{0xa1}), 22)
				require.NoError(t, err)
				// Opaque bytes do not claim a valid compressed Certificate.
				// They exercise the unsupported-message uncertainty fence.
				event, err := phaResponseTestSend(t, f, s, 0, reliabilityHS(kind, []byte{1, 2, 3}), 22)
				require.NoError(t, err)
				require.Equal(t, "unavailable after uninterpreted post-handshake message", event.Session["Handshake Messages"].([]map[string]any)[0]["PHA Response Context"])
				for _, encoded := range [][]byte{certificateVerifyTestMessage(0x304, 0x0804, []byte{1}), finishedTestMessage(phaResponseTestHashSize(suite))} {
					event, err = phaResponseTestSend(t, f, s, 0, encoded, 22)
					require.NoError(t, err)
					association := phaResponseTestAssociation(t, event.Session["Handshake Messages"].([]map[string]any)[0])
					require.Equal(t, "response prefix unavailable", association["Response Stage"])
					require.NotContains(t, association, "Request Ordinal")
				}
			})
		}
	}
}
