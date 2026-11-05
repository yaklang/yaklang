package pcaputil

import (
	"context"
	"fmt"
	"net"
	"testing"
	"unsafe"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestTLS13PHAPeerPrefixRequiresContributorACKs(t *testing.T) {
	for _, base := range []uint32{100, 0xfffffff8} {
		for _, mode := range []string{"complete", "wrap-complete", "ACK-ahead", "no-ACK", "hole", "limited", "no-SYN", "TCP-incomplete", "record-incomplete", "handshake-incomplete", "unparsed-peer", "standalone", "zero-length", "ambiguous"} {
			t.Run(fmt.Sprintf("%08x/%s", base, mode), func(t *testing.T) {
				f, s := phaResponseTestFlow(t, 0x1301, 1)
				peer := f.transportFlow.ServerConn
				peer.isn, peer.nextSeq = base-1, base+30
				e := &ProtocolEvent{Offset: 20, Length: 10}
				f.directions[0].contributions = []contribution{{start: 20, end: 24, tcpACK: base + 12, tcpACKKnown: true}, {start: 24, end: 30, tcpACK: base + 20, tcpACKKnown: true}}
				defer func() { f.directions[0].contributions = nil }()
				switch mode {
				case "ACK-ahead":
					f.directions[0].contributions[0].tcpACK = base + 31
				case "no-ACK":
					f.directions[0].contributions[0].tcpACKKnown = false
				case "hole":
					f.directions[0].contributions[1].start++
				case "limited":
					f.directions[0].evidenceLimited = true
				case "no-SYN":
					peer.synSeen = false
				case "TCP-incomplete":
					f.transportFlow.reassemblyIncomplete = true
				case "record-incomplete":
					s.recordGap[1] = true
				case "handshake-incomplete":
					s.handshake[1] = []byte{13}
				case "unparsed-peer":
					f.directions[1].buffer = []byte{23}
					defer func() { f.directions[1].buffer = nil }()
				case "standalone":
					f.transportFlow = nil
				case "zero-length":
					e.Length = 0
				case "ambiguous":
					f.directions[0].contributions[0].tcpACK = base + 1<<31
				}
				require.Equal(t, mode == "complete" || mode == "wrap-complete", f.phaPeerPrefixComplete(0, e))
			})
		}
	}
	// Retained contributor metadata still fits its existing 80-byte charge.
	require.LessOrEqual(t, unsafe.Sizeof(contribution{}), uintptr(80))
}

func TestTLS13PHALateRequestAssociatesWithoutRewritingPriorEvents(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, empty := range []bool{false, true} {
				for _, timing := range []string{"after-certificate", "after-finished", "never-observed"} {
					t.Run(fmt.Sprintf("%04x/server%d/empty%t/%s", suite, server, empty, timing), func(t *testing.T) {
						f, s := phaResponseTestFlow(t, suite, server)
						client, context := 1-server, []byte{0xab}
						peer := f.transportFlow.ServerConn
						if client == 1 {
							peer = f.transportFlow.ClientConn
						}
						peer.nextSeq = 99 // response ACK acknowledges an undelivered peer byte
						certificate, err := phaResponseTestSend(t, f, s, client, phaResponseTestCertificate(context, empty), 22)
						require.NoError(t, err)
						first := phaResponseTestAssociation(t, certificate.Session["Handshake Messages"].([]map[string]any)[0])
						require.Equal(t, "request not observed; association unavailable", first["Context Association"])
						require.NotContains(t, first, "Request Ordinal")
						request := func(complete bool) {
							peer.nextSeq = 100
							e, err := phaResponseTestSend(t, f, s, server, phaContextTestRequest(context), 22)
							require.NoError(t, err)
							admission := e.Session["Handshake Messages"].([]map[string]any)[0]["Certificate Request Admission"].(map[string]any)
							response := admission["Earlier Response"].(map[string]any)
							require.Equal(t, uint64(1), response["Request Ordinal"])
							require.Equal(t, "matched observed CertificateRequest", response["Context Association"])
							require.Equal(t, complete, s.phaRequests[string(context)].finished)
							require.Empty(t, s.phaEarlyResponses)
							e.nativeTLSObservation, e.semanticFields = true, cloneSession(e.Session)
							fields := e.DisplayFieldValues()
							require.Equal(t, []any{uint64(1)}, fields.FieldMap()["tls.pha.request_index"])
							for _, field := range fields.Values {
								if field.Name == "tls.pha.request_index" {
									require.Equal(t, "context", field.Origin)
									require.Nil(t, field.Range)
								}
							}
							clear(response) // public association cannot rewrite private history
						}
						if timing == "after-certificate" {
							request(false)
						}
						if !empty {
							_, err = phaResponseTestSend(t, f, s, client, certificateVerifyTestMessage(0x304, 0x0804, []byte{1}), 22)
							require.NoError(t, err)
						}
						finished, err := phaResponseTestSend(t, f, s, client, finishedTestMessage(phaResponseTestHashSize(suite)), 22)
						require.NoError(t, err)
						last := phaResponseTestAssociation(t, finished.Session["Handshake Messages"].([]map[string]any)[0])
						require.Equal(t, "response block complete; transcript not verified", last["Response Stage"])
						if timing == "after-finished" {
							request(true)
						}
						if timing != "never-observed" {
							require.True(t, s.phaRequests[string(context)].finished)
						} else {
							require.Len(t, s.phaEarlyResponses, 1)
							require.NotContains(t, last, "Request Ordinal")
						}
						require.Equal(t, "request not observed; association unavailable", first["Context Association"])
						require.NotContains(t, first, "Request Ordinal")
						_, err = phaResponseTestSend(t, f, s, client, []byte("GET /after HTTP/1.1\r\n\r\n"), 23)
						require.NoError(t, err)
						_, err = phaResponseTestSend(t, f, s, client, phaResponseTestCertificate(context, empty), 22)
						require.ErrorContains(t, err, "repeated TLS Certificate response context")
					})
				}
			}
		}
	}
}

func TestTCPQueuedContributorPreservesItsOriginalACK(t *testing.T) {
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			pool := newTrafficPool(context.Background(), TCPReassemblyOptions{Workers: workers, Stream: true})
			defer pool.Close()
			var payloads []string
			var acknowledgements []uint32
			var known []bool
			pool.onFlowFrameDataFrameArrived = append(pool.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, frame *TrafficFrame) {
				payloads = append(payloads, string(frame.Payload))
				acknowledgements = append(acknowledgements, frame.evidence.tcpACK)
				known = append(known, frame.evidence.tcpACKKnown)
			})
			pool.startWorkers()
			ip := &layers.IPv4{SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
			feed := func(seq, ack uint32, syn bool, payload string) {
				tcp := &layers.TCP{SrcPort: 12345, DstPort: 443, Seq: seq, Ack: ack, SYN: syn, ACK: !syn}
				tcp.Payload = []byte(payload)
				pool.Feed(nil, ip, tcp)
			}
			feed(99, 0, true, "")
			feed(103, 1400, false, "def")
			feed(100, 1300, false, "abc")
			pool.Close()
			require.NoError(t, pool.Err())
			require.Equal(t, []string{"abc", "def"}, payloads)
			require.Equal(t, []uint32{1300, 1400}, acknowledgements)
			require.Equal(t, []bool{true, true}, known)
		})
	}
}

func TestTLS13PHAEarlyResponseLimitsCommitAtomically(t *testing.T) {
	for _, mode := range []string{"collection", "retained-bytes"} {
		t.Run(mode, func(t *testing.T) {
			f, s := phaResponseTestFlow(t, 0x1301, 1)
			f.transportFlow.ServerConn.nextSeq = 99
			_, err := phaResponseTestSend(t, f, s, 0, phaResponseTestCertificate([]byte{0xa1}, true), 22)
			require.NoError(t, err)
			_, err = phaResponseTestSend(t, f, s, 0, finishedTestMessage(32), 22)
			require.NoError(t, err)
			before, charge := s.phaEarlyResponses, s.requestContextStorage()
			encoded := phaResponseTestCertificate([]byte{0xb2}, true)
			if mode == "collection" {
				f.a.budget.MaxCollectionElements = 1
			} else {
				f.a.config.MaxBufferedBytes = int(f.a.buffered.Load()) + len(encoded) + 80
			}
			_, err = phaResponseTestSend(t, f, s, 0, encoded, 22)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrResourceExceeded, typed.Kind)
			require.Equal(t, before, s.phaEarlyResponses)
			require.Equal(t, charge, s.requestContextStorage())
			require.Equal(t, tlsPHAResponseNone, s.phaResponse.stage)
		})
	}
}
