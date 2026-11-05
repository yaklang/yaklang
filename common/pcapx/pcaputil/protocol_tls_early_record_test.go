package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Public deterministic secrets; live peer captures/keylogs remain outside Git.
type tlsEarlyRecordTestKeys struct {
	random []byte
	values map[string][]byte
}

func (p *tlsEarlyRecordTestKeys) LookupTLSSecret(label string, random []byte) ([]byte, bool) {
	if !bytes.Equal(p.random, random) {
		return nil, false
	}
	v, ok := p.values[label]
	return bytes.Clone(v), ok
}

func tlsEarlyRecordTestFlow(t *testing.T, suite uint16, client int, deferred bool) (*binFlow, *[]*ProtocolEvent, *tlsEarlyRecordTestKeys, func(int, []byte), func(string, int, uint64, byte, []byte) []byte) {
	t.Helper()
	a, events := remainingParser(t)
	a.config.Deferred = deferred
	ch, _ := n13AuditHelloPair(suite, true, true, 0)
	p := &tlsEarlyRecordTestKeys{random: bytes.Clone(ch[2:34]), values: map[string][]byte{}}
	n := 32
	if suite == 0x1302 {
		n = 48
	}
	for i, label := range []string{"CLIENT_EARLY_TRAFFIC_SECRET", "CLIENT_HANDSHAKE_TRAFFIC_SECRET", "SERVER_HANDSHAKE_TRAFFIC_SECRET", "CLIENT_TRAFFIC_SECRET_0", "SERVER_TRAFFIC_SECRET_0"} {
		p.values[label] = bytes.Repeat([]byte{byte(0x61 + i)}, n)
	}
	a.tlsSecrets = p
	f := &binFlow{a: a, id: 7, protocol: "tls", tls: newBinTLS(), transportFlow: &TrafficFlow{Index: 17}, endpoints: [2]string{"192.0.2.1:40000", "192.0.2.2:40001"}}
	t.Cleanup(func() { f.closeSession(); require.Zero(t, a.stats().BufferedBytes) })
	feed := func(dir int, wire []byte) { t.Helper(); f.feed(dir, wire, time.Unix(1234, 0)) }
	seal := func(epoch string, dir int, seq uint64, kind byte, data []byte) []byte {
		t.Helper()
		s := newBinTLS()
		s.version, s.suite, s.client = 0x304, suite, client
		label := "CLIENT_EARLY_TRAFFIC_SECRET"
		if epoch != "early" {
			prefix := "SERVER_"
			if dir == client {
				prefix = "CLIENT_"
			}
			label = prefix + "HANDSHAKE_TRAFFIC_SECRET"
			if epoch == "application" {
				label = prefix + "TRAFFIC_SECRET_0"
			}
		}
		require.NoError(t, s.install13(dir, p.values[label]))
		return boundaryTLSRecord(t, s, dir, seq, data, kind, 0)
	}
	return f, events, p, feed, seal
}

func TestTLSEarlyRecordEpochsAndObservedAcceptance(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("suite%04x/client%d/deferred%t", suite, client, deferred), func(t *testing.T) {
					f, events, _, feed, seal := tlsEarlyRecordTestFlow(t, suite, client, deferred)
					ch, sh := n13AuditHelloPair(suite, true, true, 0)
					request := []byte("GET /early HTTP/1.1\r\nHost: early.invalid\r\n\r\n")
					feed(client, carrierTestRecord(22, reliabilityHS(1, ch)))
					feed(client, seal("early", client, 0, 23, request))
					original := (*events)[len(*events)-1]
					originalID := original.ID
					require.False(t, original.Session["Authentication Verified"] == true)
					complete, _ := closeNotifyTestHTTP(*events)
					require.Empty(t, complete, "an early key alone does not prove server acceptance")
					feed(1-client, carrierTestRecord(22, reliabilityHS(2, sh)))
					feed(1-client, seal("handshake", 1-client, 0, 22, reliabilityHS(8, eeScopeBody(tlsResumptionExt{42, nil}))))
					complete, _ = closeNotifyTestHTTP(*events)
					require.Len(t, complete, 1, "accepted early bytes must recover using their own key epoch")
					require.Equal(t, request, complete[0].Raw)
					require.Equal(t, "decrypted", complete[0].SourceBytes.Kind)
					require.NotEqual(t, originalID, complete[0].SourceBytes.ParentPDU)
					var recovered *ProtocolEvent
					for _, e := range *events {
						if e.Protocol == "tls" && e.Session["Reprocessed From PDU"] == originalID {
							recovered = e
						}
					}
					require.NotNil(t, recovered)
					require.Equal(t, uint64(17), recovered.TCPFlowID, "supplemental TLS preserves the physical capture generation")
					require.Equal(t, []any{uint64(17)}, recovered.DisplayFieldValues().FieldMap()["tcp.flow.id"])
					require.Equal(t, recovered.TCPFlowID, complete[0].TCPFlowID)
					require.Equal(t, true, recovered.Session["Authentication Verified"])
					require.Equal(t, "early", recovered.Session["TLS Key Epoch"])
					require.Equal(t, suite, recovered.Session["Early Cipher Suite"])
					require.Equal(t, recovered.ID, complete[0].SourceBytes.ParentPDU)
					require.False(t, original.Session["Authentication Verified"] == true, "published observations remain immutable")
					require.Zero(t, f.tls.seq[client], "early sequence must not consume handshake record zero")
					feed(1-client, seal("handshake", 1-client, 1, 22, reliabilityHS(20, bytes.Repeat([]byte{0x31}, len(f.tls.secret[1-client])))))
					feed(client, seal("early", client, 1, 22, reliabilityHS(5, nil)))
					eoed := (*events)[len(*events)-1]
					require.Empty(t, eoed.Error)
					require.Equal(t, "early", eoed.Session["TLS Key Epoch"])
					require.Equal(t, byte(5), eoed.Session["Handshake Messages"].([]map[string]any)[0]["Type"])
					require.Zero(t, f.tls.seq[client])
					n := 32
					if suite == 0x1302 {
						n = 48
					}
					feed(client, seal("handshake", client, 0, 22, reliabilityHS(20, bytes.Repeat([]byte{0x32}, n))))
					finished := (*events)[len(*events)-1]
					require.Empty(t, finished.Error)
					require.Equal(t, true, finished.Session["Authentication Verified"])
					feed(1-client, seal("application", 1-client, 0, 23, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")))
					complete, _ = closeNotifyTestHTTP(*events)
					require.Len(t, complete, 2)
					require.Equal(t, complete[0].ID, complete[1].ResponseTo)
					for _, e := range *events {
						require.Empty(t, e.Error)
					}
				})
			}
		}
	}
}

func TestTLSEarlyRecordAcceptanceRecoveryAndRefusal(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, client := range []int{0, 1} {
			for _, mode := range []string{"accepted", "rejected", "rejected-key-unavailable", "early-key-unavailable", "early-key-late", "client-handshake-key-unavailable", "client-flight-before-EE", "bad-early-tag", "missing-EndOfEarlyData", "EndOfEarlyData-handshake-key", "EndOfEarlyData-body", "EndOfEarlyData-trailing"} {
				t.Run(fmt.Sprintf("suite%04x/client%d/%s", suite, client, mode), func(t *testing.T) {
					flow, events, p, feed, seal := tlsEarlyRecordTestFlow(t, suite, client, false)
					ch, sh := n13AuditHelloPair(suite, true, true, 0)
					n := 32
					if suite == 0x1302 {
						n = 48
					}
					request := []byte("GET /early-boundary HTTP/1.1\r\nHost: early.invalid\r\n\r\n")
					requestRecord := seal("early", client, 0, 23, request)
					eoed := seal("early", client, 1, 22, reliabilityHS(5, nil))
					clientFinished := seal("handshake", client, 0, 22, reliabilityHS(20, bytes.Repeat([]byte{0x35}, n)))
					serverFinished := seal("handshake", 1-client, 1, 22, reliabilityHS(20, bytes.Repeat([]byte{0x36}, n)))
					earlyKey := bytes.Clone(p.values["CLIENT_EARLY_TRAFFIC_SECRET"])
					if mode == "early-key-late" || mode == "early-key-unavailable" || mode == "rejected-key-unavailable" {
						delete(p.values, "CLIENT_EARLY_TRAFFIC_SECRET")
					}
					if mode == "client-handshake-key-unavailable" {
						delete(p.values, "CLIENT_HANDSHAKE_TRAFFIC_SECRET")
					}
					if mode == "bad-early-tag" {
						requestRecord[len(requestRecord)-1] ^= 1
					}
					feed(client, carrierTestRecord(22, reliabilityHS(1, ch)))
					feed(client, requestRecord)
					feed(1-client, carrierTestRecord(22, reliabilityHS(2, sh)))
					accepted := mode != "rejected" && mode != "rejected-key-unavailable"
					if mode == "client-flight-before-EE" {
						feed(client, eoed)
						feed(client, clientFinished)
					}
					ee := eeScopeBody()
					if accepted {
						ee = eeScopeBody(tlsResumptionExt{42, nil})
					}
					feed(1-client, seal("handshake", 1-client, 0, 22, reliabilityHS(8, ee)))
					complete, _ := closeNotifyTestHTTP(*events)
					wantEarly := accepted && mode != "bad-early-tag" && mode != "early-key-late" && mode != "early-key-unavailable"
					if wantEarly {
						require.Len(t, complete, 1)
					} else {
						require.Empty(t, complete)
					}
					if mode == "rejected" {
						require.Nil(t, flow.tls.child, "rejected plaintext must not enter the accepted application owner")
						var observed []*ProtocolEvent
						for _, e := range *events {
							if e.Protocol == "tls" && e.Session["TLS Key Epoch"] == "early" && e.Session["Authentication Verified"] == true {
								observed = append(observed, e)
							}
						}
						require.Len(t, observed, 1)
						require.Equal(t, request, observed[0].Session["Authenticated Plaintext"])
					}
					if mode == "early-key-late" {
						p.values["CLIENT_EARLY_TRAFFIC_SECRET"] = earlyKey
					}
					feed(1-client, serverFinished)
					if mode == "early-key-late" {
						complete, _ = closeNotifyTestHTTP(*events)
						require.Len(t, complete, 1)
					}
					if mode == "client-handshake-key-unavailable" {
						return
					}
					if mode == "EndOfEarlyData-handshake-key" {
						eoed = seal("handshake", client, 0, 22, reliabilityHS(5, nil))
					}
					if mode == "EndOfEarlyData-body" {
						eoed = seal("early", client, 1, 22, reliabilityHS(5, []byte{0}))
					}
					if mode == "EndOfEarlyData-trailing" {
						eoed = seal("early", client, 1, 22, append(reliabilityHS(5, nil), 0))
					}
					if accepted && mode != "client-flight-before-EE" && mode != "missing-EndOfEarlyData" {
						feed(client, eoed)
					}
					invalidEOED := mode == "EndOfEarlyData-handshake-key" || mode == "EndOfEarlyData-body" || mode == "EndOfEarlyData-trailing"
					if invalidEOED {
						last := (*events)[len(*events)-1]
						require.NotEmpty(t, last.Error)
						require.Equal(t, true, last.Session["Authentication Verified"])
						return
					}
					if mode != "client-flight-before-EE" {
						feed(client, clientFinished)
					}
					authenticatedFinished := false
					for _, e := range *events {
						if e.Protocol == "tls" && bytes.Equal(e.Raw, clientFinished) {
							require.Empty(t, e.Error, "a valid handshake-key Finished must retain sequence zero independently of early failures")
							authenticatedFinished = authenticatedFinished || e.Session["Authentication Verified"] == true
						}
					}
					require.True(t, authenticatedFinished, "the immutable pending observation must have an authenticated supplemental observation")
					if !accepted {
						feed(client, seal("application", client, 0, 23, request))
					}
					feed(1-client, seal("application", 1-client, 0, 23, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")))
					complete, _ = closeNotifyTestHTTP(*events)
					if mode == "early-key-unavailable" {
						require.Empty(t, complete)
						var partial []*ProtocolEvent
						for _, e := range *events {
							require.NotContains(t, e.Error, "AuthenticationFailed")
							if e.Protocol == "http" {
								partial = append(partial, e)
							}
						}
						require.Len(t, partial, 1)
						require.Equal(t, "http-response-header-observation", partial[0].Profile)
						require.Zero(t, partial[0].ResponseTo)
					} else if mode != "bad-early-tag" {
						require.Len(t, complete, 2)
						require.Equal(t, complete[0].ID, complete[1].ResponseTo)
					}
				})
			}
		}
	}
}

func TestTLSEarlyRecordPendingOwnershipAndCallbackPoison(t *testing.T) {
	f, events, _, feed, seal := tlsEarlyRecordTestFlow(t, 0x1301, 0, false)
	f.domain = CaptureDomain{Interface: 7, Encapsulation: "vlan:17"}
	callback := f.a.config.OnEvent
	var original, decision uint64
	f.a.config.OnEvent = func(e *ProtocolEvent) {
		if e.Protocol == "tls" {
			copy := *e
			cloneEvidence(&copy)
			copy.Raw = bytes.Clone(e.Raw)
			copy.Session = cloneSession(e.Session)
			callback(&copy)
			if e.Session["Secret Lookup Status"] == "early-context-pending" {
				original = e.ID
			}
			for _, h := range func() []map[string]any { v, _ := e.Session["Handshake Messages"].([]map[string]any); return v }() {
				if h["Type"] == byte(8) {
					decision = e.ID
				}
			}
			e.ID = 99999
			e.TCPFlowID = 99999
			e.Timestamp = time.Unix(0, 0)
			e.Domain = CaptureDomain{}
			e.SourceBytes = ByteSource{}
			e.Session = nil
			clear(e.Raw)
			return
		}
		callback(e)
	}
	ch, sh := n13AuditHelloPair(0x1301, true, true, 0)
	feed(0, carrierTestRecord(22, reliabilityHS(1, ch)))
	request := []byte("GET /owned HTTP/1.1\r\nHost: early.invalid\r\n\r\n")
	wire := seal("early", 0, 0, 23, request)
	f.contribute(0, len(wire), PacketReference{Number: 42, Domain: f.domain})
	feed(0, wire)
	feed(1, carrierTestRecord(22, reliabilityHS(2, sh)))
	ee := seal("handshake", 1, 0, 22, reliabilityHS(8, eeScopeBody(tlsResumptionExt{42, nil})))
	f.contribute(1, len(ee), PacketReference{Number: 44, Domain: f.domain})
	feed(1, ee)
	complete, _ := closeNotifyTestHTTP(*events)
	require.Len(t, complete, 1)
	require.Equal(t, request, complete[0].Raw)
	require.Equal(t, f.domain, complete[0].Domain)
	require.Equal(t, []PacketReference{{Number: 42, Domain: f.domain}}, complete[0].SourceBytes.PacketRefs)
	var recovered *ProtocolEvent
	for _, e := range *events {
		if e.Protocol == "tls" && e.Session["Reprocessed From PDU"] == original {
			recovered = e
		}
	}
	require.NotNil(t, recovered)
	require.Equal(t, decision, recovered.Session["Early Data Decision PDU"])
	require.Equal(t, recovered.ID, complete[0].SourceBytes.ParentPDU)
	require.Equal(t, uint64(42), recovered.SourceBytes.PacketRefs[0].Number)
	require.NotEqual(t, uint64(99999), decision)
}

func TestTLSEarlyRecordPendingBudgetsAndClose(t *testing.T) {
	for _, mode := range []string{"close", "count", "bytes", "global"} {
		t.Run(mode, func(t *testing.T) {
			f, events, _, feed, seal := tlsEarlyRecordTestFlow(t, 0x1301, 0, false)
			ch, _ := n13AuditHelloPair(0x1301, true, true, 0)
			feed(0, carrierTestRecord(22, reliabilityHS(1, ch)))
			if mode == "count" {
				f.a.budget.MaxCollectionElements = 1
			}
			if mode == "global" {
				f.a.config.MaxBufferedBytes = int(f.a.buffered.Load()) + 10
			}
			n := 2
			if mode == "bytes" {
				n = 5
			}
			if mode == "close" || mode == "global" {
				n = 1
			}
			for i := 0; i < n; i++ {
				plain := []byte("prefix")
				if mode == "bytes" {
					plain = bytes.Repeat([]byte{0x61}, 16384)
				}
				feed(0, seal("early", 0, uint64(i), 23, plain))
			}
			complete, _ := closeNotifyTestHTTP(*events)
			require.Empty(t, complete)
			if mode != "close" {
				last := (*events)[len(*events)-1]
				wantStatus := "limited"
				if mode == "global" {
					wantStatus = "context-required"
				}
				require.Equal(t, wantStatus, last.Status)
				require.NotNil(t, last.sessionError)
				require.Equal(t, ErrResourceExceeded, last.sessionError.Kind)
				require.NotEmpty(t, last.Error)
			} else {
				require.NotEmpty(t, f.tls.early.pending)
			}
			f.closeSession()
			require.Zero(t, f.a.stats().BufferedBytes)
		})
	}
}

func TestTLSEarlyRecordHelloRetryRejectsOnlyFirstFlight(t *testing.T) {
	f, events, _, feed, seal := tlsEarlyRecordTestFlow(t, 0x1301, 0, false)
	share := append([]byte{0, 29, 0, 32}, bytes.Repeat([]byte{7}, 32)...)
	base := []tlsResumptionExt{{43, []byte{2, 3, 4}}, {10, []byte{0, 4, 0, 29, 0, 23}}, {51, append([]byte{0, 36}, share...)}, {45, []byte{1, 1}}}
	psk := tlsResumptionExt{41, tlsResumptionPSKs([][]byte{[]byte("audit identity A")}, []uint32{1}, []int{32})}
	ch := tlsResumptionHello(true, nil, 0x1301, append(append([]tlsResumptionExt{}, base...), tlsResumptionExt{42, nil}, psk)...)
	feed(0, carrierTestRecord(22, reliabilityHS(1, ch)))
	feed(0, seal("early", 0, 0, 23, []byte("GET /rejected-first-flight HTTP/1.1\r\nHost: early.invalid\r\n\r\n")))
	require.Len(t, f.tls.early.pending, 1)
	hrr := tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, []byte{0, 23}})
	copy(hrr[2:34], tlsRetryRandom[:])
	feed(1, carrierTestRecord(22, reliabilityHS(2, hrr)))
	require.Empty(t, f.tls.early.pending)
	point := append([]byte{4}, bytes.Repeat([]byte{8}, 64)...)
	share2 := append([]byte{0, 23, 0, 65}, point...)
	base[2] = tlsResumptionExt{51, append([]byte{0, 69}, share2...)}
	ch2 := tlsResumptionHello(true, nil, 0x1301, append(base, psk)...)
	feed(0, carrierTestRecord(22, reliabilityHS(1, ch2)))
	sh2 := tlsResumptionHello(false, nil, 0x1301, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, share2}, tlsResumptionExt{41, []byte{0, 0}})
	feed(1, carrierTestRecord(22, reliabilityHS(2, sh2)))
	feed(1, seal("handshake", 1, 0, 22, reliabilityHS(8, eeScopeBody())))
	feed(1, seal("handshake", 1, 1, 22, reliabilityHS(20, bytes.Repeat([]byte{0x38}, 32))))
	feed(0, seal("handshake", 0, 0, 22, reliabilityHS(20, bytes.Repeat([]byte{0x39}, 32))))
	request := []byte("GET /after-retry HTTP/1.1\r\nHost: early.invalid\r\n\r\n")
	feed(0, seal("application", 0, 0, 23, request))
	feed(1, seal("application", 1, 0, 23, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")))
	complete, _ := closeNotifyTestHTTP(*events)
	require.Len(t, complete, 2)
	require.Equal(t, request, complete[0].Raw)
	require.Equal(t, complete[0].ID, complete[1].ResponseTo)
	for _, e := range *events {
		require.Empty(t, e.Error)
	}
}
