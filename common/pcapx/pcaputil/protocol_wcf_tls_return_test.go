package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func wcfTLSReturnTestPreamble() []byte {
	p := wcfTestPreamble(2, 3)
	return append(bytes.Clone(p[:len(p)-1]), wcfUpgradeRequestTestWire()...)
}

func wcfTLSReturnTestHello(client bool, epoch byte) []byte {
	h := reliabilityHello(client, false, 29, nil)
	h[2] = epoch
	typ := byte(2)
	if client {
		typ = 1
	}
	return carrierTestRecord(22, reliabilityHS(typ, h))
}

func wcfTLSReturnFeed(t *testing.T, s ProtocolSession, dir, width int, wire []byte) (events []*ProtocolEvent) {
	t.Helper()
	for len(wire) > 0 {
		n := min(width, len(wire))
		r := s.Feed(dir, time.Unix(42, 0), wire[:n])
		if r.Err != nil {
			require.Equal(t, ErrNeedMore, r.Err.Kind, "%+v", r.Events)
		}
		events = append(events, r.Events...)
		wire = wire[n:]
	}
	return
}

func wcfTLSReturnTestStart(t *testing.T, client int) *captureSession {
	t.Helper()
	s := newWCFTestSession(t, DefaultParserBudget()).(*captureSession)
	t.Cleanup(func() { s.Close("FIN"); require.Zero(t, s.f.a.buffered.Load()) })
	wcfTLSReturnFeed(t, s, client, 4096, wcfTLSReturnTestPreamble())
	wcfTLSReturnFeed(t, s, 1-client, 4096, []byte{wcfRecordUpgradeResponse})
	wcfTLSReturnFeed(t, s, client, 4096, wcfTLSReturnTestHello(true, 1))
	wcfTLSReturnFeed(t, s, 1-client, 4096, wcfTLSReturnTestHello(false, 1))
	for _, dir := range []int{client, 1 - client} {
		// The observed TLS 1.3 suite requires room for its inner type and tag;
		// opaque test records still need a possible protected envelope.
		wcfTLSReturnFeed(t, s, dir, 4096, carrierTestRecord(23, bytes.Repeat([]byte{1, 2, 3}, 6)))
	}
	return s
}

func TestWCFTLSRepeatedCarrierEpochsKeepDirectionalBoundaries(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, width := range []int{1, 5, 64, 4096} {
			for _, earlyResponse := range []bool{false, true} {
				t.Run(fmt.Sprintf("client%d/chunk%d/early-response%v", client, width, earlyResponse), func(t *testing.T) {
					s := wcfTLSReturnTestStart(t, client)
					server := 1 - client
					var previous []*ProtocolEvent
					for epoch := uint64(2); epoch <= 3; epoch++ {
						old := s.f.tls
						old.seq[client] = 991
						old.alpn = "old-application"
						old.requestContexts = map[string]struct{}{"old": {}}
						before, err := json.Marshal(previous)
						require.NoError(t, err)
						prefix := append(wcfTLSReturnTestPreamble(), wcfTLSReturnTestHello(true, byte(epoch))...)
						var got []*ProtocolEvent
						if earlyResponse {
							require.Empty(t, wcfTLSReturnFeed(t, s, server, width, []byte{wcfRecordUpgradeResponse}))
							require.Same(t, old, s.f.tls)
							got = append(got, wcfTLSReturnFeed(t, s, client, width, prefix)...)
						} else {
							require.Empty(t, wcfTLSReturnFeed(t, s, client, width, prefix))
							require.Same(t, old, s.f.tls)
							// Client capture is already at its next clear epoch; the
							// receiver still owns and parses the preceding TLS epoch.
							oldRecords := wcfTLSReturnFeed(t, s, server, width, carrierTestRecord(23, bytes.Repeat([]byte{4, 5, 6}, 6)))
							require.Len(t, oldRecords, 1)
							require.Equal(t, epoch-1, oldRecords[0].Session["Observed WCF Carrier"].(map[string]any)["Observed Session Index"])
							got = append(got, wcfTLSReturnFeed(t, s, server, width, []byte{wcfRecordUpgradeResponse})...)
						}
						got = append(got, wcfTLSReturnFeed(t, s, server, width, wcfTLSReturnTestHello(false, byte(epoch)))...)
						require.Len(t, got, 8, "six clear MC-NMF records and two new TLS hellos")
						for i, e := range got {
							require.Equal(t, "decoded", e.Status, "%d: %s", i, e.Error)
							if e.Protocol == "wcf" {
								require.True(t, e.NativeWCFWireMatches())
								require.Equal(t, epoch, e.Session["Observed Session Index"])
								require.Contains(t, e.Session["Previous Carrier Completion"], "not verified")
							} else {
								require.Equal(t, "tls", e.Protocol)
								require.Equal(t, epoch, e.Session["Observed WCF Carrier"].(map[string]any)["Observed Session Index"])
								require.False(t, e.Session["Authentication Verified"].(bool))
							}
						}
						require.NotSame(t, old, s.f.tls)
						require.Zero(t, s.f.tls.seq[client])
						require.Empty(t, s.f.tls.alpn)
						require.Empty(t, s.f.tls.requestContexts)
						require.Equal(t, byte(epoch), s.f.tls.random[0])
						after, err := json.Marshal(previous)
						require.NoError(t, err)
						require.Equal(t, before, after, "published older record/source metadata is immutable")
						previous = got
						for _, dir := range []int{client, server} {
							wcfTLSReturnFeed(t, s, dir, width, carrierTestRecord(23, bytes.Repeat([]byte{7, 8, 9}, 6)))
						}
					}
				})
			}
		}
	}
}

func TestWCFTLSInitialUpgradeWaitsForActualPeerAcceptance(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, earlyResponse := range []bool{false, true} {
			t.Run(fmt.Sprintf("client%d/early-response%v", client, earlyResponse), func(t *testing.T) {
				s := newWCFTestSession(t, DefaultParserBudget()).(*captureSession)
				defer s.Close("FIN")
				prefix := wcfTLSReturnTestPreamble()
				var got []*ProtocolEvent
				if earlyResponse {
					got = append(got, wcfTLSReturnFeed(t, s, client, 1, prefix[:8])...)
					require.Empty(t, wcfTLSReturnFeed(t, s, 1-client, 1, []byte{wcfRecordUpgradeResponse}))
					got = append(got, wcfTLSReturnFeed(t, s, client, 1, append(bytes.Clone(prefix[8:]), wcfTLSReturnTestHello(true, 1)...))...)
				} else {
					got = append(got, wcfTLSReturnFeed(t, s, client, 4096, append(prefix, wcfTLSReturnTestHello(true, 1)...))...)
					require.Nil(t, s.f.tls)
					got = append(got, wcfTLSReturnFeed(t, s, 1-client, 4096, []byte{wcfRecordUpgradeResponse})...)
				}
				require.Len(t, got, 7)
				require.Equal(t, "tls", got[6].Protocol)
				require.Equal(t, client, s.f.tls.client)
			})
		}
	}
}

func TestWCFTLSReturnDoesNotScanCiphertextOrAdmitUnpairedPrefix(t *testing.T) {
	for _, missing := range []string{"receiver", "initiator-upgrade", "wrong-role", "generic-tls", "record-gap", "handshake-gap", "no-hello", "counter-exhausted"} {
		t.Run(missing, func(t *testing.T) {
			s := wcfTLSReturnTestStart(t, 0)
			f := s.f
			inside := wcfTLSReturnFeed(t, s, 0, 4096, carrierTestRecord(23, wcfTLSReturnTestPreamble()))
			require.Len(t, inside, 1)
			require.Equal(t, "tls", inside[0].Protocol, "no scanning inside protected record bodies")
			if missing == "generic-tls" {
				f.wcfTLS = nil
			}
			if missing == "record-gap" {
				f.tls.recordGap[0] = true
			}
			if missing == "handshake-gap" {
				f.tls.handshakeGap[1] = true
			}
			if missing == "no-hello" {
				f.tls.random = nil
			}
			if missing == "counter-exhausted" {
				f.wcfTLS.observedIndex = ^uint64(0)
			}
			prefix := wcfTLSReturnTestPreamble()
			if missing == "initiator-upgrade" {
				prefix = prefix[:len(prefix)-len(wcfUpgradeRequestTestWire())]
			}
			dir := 0
			if missing == "wrong-role" {
				dir = 1
			}
			events := s.Feed(dir, time.Unix(43, 0), prefix).Events
			if missing != "receiver" {
				events = append(events, s.Feed(1, time.Unix(44, 0), []byte{wcfRecordUpgradeResponse}).Events...)
			}
			events = append(events, s.Close("FIN")...)
			for _, e := range events {
				if e.Protocol == "wcf" {
					require.NotEqual(t, "decoded", e.Status, "incomplete/unsupported evidence cannot admit a new framing epoch")
				}
			}
			require.Nil(t, f.wcfTLS)
			require.Nil(t, f.tls)
			require.Zero(t, f.a.buffered.Load())
			require.Equal(t, ErrFatalSessionError, s.Feed(0, time.Unix(45, 0), prefix).Err.Kind)
		})
	}
}

func TestWCFTLSReturnProbeRequiresAllFieldsWithinBudget(t *testing.T) {
	p := wcfTLSReturnTestPreamble()
	for n := 0; n < len(p); n++ {
		ok, err := wcfTLSReturnPreamble(p[:n], 4096)
		require.NoError(t, err)
		require.False(t, ok, "cut %d of %d", n, len(p))
	}
	ok, err := wcfTLSReturnPreamble(p, 4096)
	require.NoError(t, err)
	require.True(t, ok)
	for _, limit := range []int{1, 2, 3, len(p) - 1} {
		ok, err := wcfTLSReturnPreamble(p, limit)
		require.Error(t, err)
		require.False(t, ok)
	}
	for _, at := range []int{0, 1, 3, 4, len(p) - 1} {
		bad := bytes.Clone(p)
		bad[at] ^= 0x80
		ok, _ := wcfTLSReturnPreamble(bad, 4096)
		require.False(t, ok, "corrupted required field at %d", at)
	}
}

func TestWCFTLSCarrierObservationOwnsMetadataAndCannotComeFromDTO(t *testing.T) {
	s := wcfTLSReturnTestStart(t, 0)
	r := s.Feed(0, time.Unix(43, 0), carrierTestRecord(23, bytes.Repeat([]byte{4, 5, 6}, 6)))
	require.Nil(t, r.Err)
	e := r.Events[0]
	witness, ok := e.NativeWCFTLSCarrier()
	require.True(t, ok)
	require.Equal(t, uint64(1), witness.ObservedSessionIndex)
	require.Equal(t, []string{"WCF/MC-NMF"}, e.CarrierLayers)
	fields, ok := e.NativeWCFTLSFields()
	require.True(t, ok)
	fields["Observed WCF Carrier"].(map[string]any)["Observed Session Index"] = uint64(999)
	e.Session = fields
	e.Protocol = "http3"
	e.Fields = map[string]any{"forged": true}
	e.Raw[0] ^= 0xff
	witness, ok = e.NativeWCFTLSCarrier()
	require.True(t, ok)
	require.Equal(t, uint64(1), witness.ObservedSessionIndex)
	require.False(t, e.NativeWCFTLSWireMatches())
	owned, _ := e.NativeWCFTLSFields()
	require.Equal(t, uint64(1), owned["Observed WCF Carrier"].(map[string]any)["Observed Session Index"])
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	var dto ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &dto))
	_, ok = dto.NativeWCFTLSCarrier()
	require.False(t, ok)
	// Editing an earlier published DTO cannot manufacture or consume the
	// private witness used to authorize the subsequent framing boundary.
	require.Empty(t, wcfTLSReturnFeed(t, s, 0, 4096, wcfTLSReturnTestPreamble()))
	next := wcfTLSReturnFeed(t, s, 1, 4096, []byte{10})
	require.Len(t, next, 6)
	for _, event := range next {
		require.Equal(t, uint64(2), event.Session["Observed Session Index"])
	}
}

func TestWCFTLSCarrierRefusalReleasesStateAndCannotResurrect(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, why := range []string{"shared-budget", "fatal-header", "new-owner"} {
			t.Run(fmt.Sprintf("client%d/%s", client, why), func(t *testing.T) {
				s := wcfTLSReturnTestStart(t, client)
				f := s.f
				prefix := wcfTLSReturnTestPreamble()
				var events []*ProtocolEvent
				if why == "new-owner" {
					fresh := newWCFTestSession(t, DefaultParserBudget()).(*captureSession)
					defer fresh.Close("FIN")
					wcfTLSReturnFeed(t, fresh, client, 4096, wcfTLSReturnTestHello(true, 2))
					require.Nil(t, fresh.f.wcfTLS, "same endpoints/port alone cannot transfer the old owner's witness")
					events = append(events, fresh.Feed(client, time.Unix(44, 0), prefix).Events...)
					for _, e := range events {
						require.NotEqual(t, "wcf", e.Protocol)
					}
					return
				}
				if why == "shared-budget" {
					require.Empty(t, wcfTLSReturnFeed(t, s, client, 4096, prefix))
					f.a.config.MaxBufferedBytes = int(f.a.buffered.Load()) + 16
					r := s.Feed(1-client, time.Unix(44, 0), []byte{10})
					require.NotNil(t, r.Err)
					// FeedResult reports the later peer invalidation; the first
					// record retains the actual typed resource refusal separately.
					require.Equal(t, ErrContextRequired, r.Err.Kind)
					require.Len(t, r.Events, 2)
					require.NotNil(t, r.Events[0].sessionError)
					require.Equal(t, ErrResourceExceeded, r.Events[0].sessionError.Kind)
					require.Contains(t, r.Events[0].sessionError.Message, "memory budget")
					events = r.Events
				} else {
					r := s.Feed(client, time.Unix(44, 0), []byte{0xff, 3, 3, 0, 0})
					require.NotNil(t, r.Err)
					events = r.Events
				}
				require.Nil(t, f.wcfTLS)
				require.Nil(t, f.tls)
				events = append(events, s.Feed(client, time.Unix(45, 0), prefix).Events...)
				for _, e := range events {
					if e.Protocol == "wcf" {
						require.NotEqual(t, "decoded", e.Status)
					}
				}
				s.Close("FIN")
				require.Zero(t, f.a.buffered.Load())
			})
		}
	}
}

// The model is the explicit trace's evidence: only complete paired upgrades
// under this owner may start epoch two. Mutations of public history are not
// protocol evidence. No actual secrets/authenticated plaintext are supplied.
func FuzzWCFTLSCarrierReturnEvidence(f *testing.F) {
	for mode := byte(0); mode < 8; mode++ {
		f.Add([]byte{mode, 1, mode & 1})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 || len(data) > 128 {
			return
		}
		mode, width, client := data[0]%8, 1+int(data[1]%96), int(data[2]&1)
		s := wcfTLSReturnTestStart(t, client)
		prefix := wcfTLSReturnTestPreamble()
		if mode == 2 {
			prefix = prefix[:len(prefix)-len(wcfUpgradeRequestTestWire())]
		}
		if mode == 4 {
			client = 1 - client
		}
		if mode == 5 {
			s.f.tls.recordGap[0] = true
		}
		if mode == 6 {
			s.f.wcfTLS = nil
		}
		if mode == 7 {
			prefix[1] ^= 0x80
		}
		var events []*ProtocolEvent
		feed := func(dir int, wire []byte) {
			for len(wire) > 0 {
				n := min(width, len(wire))
				events = append(events, s.Feed(dir, time.Unix(44, 0), wire[:n]).Events...)
				wire = wire[n:]
			}
		}
		if mode == 1 {
			feed(1-client, []byte{10})
		}
		feed(client, prefix)
		if mode != 1 && mode != 3 {
			feed(1-client, []byte{10})
		}
		admitted := 0
		for _, e := range events {
			if observation, ok := e.NativeWCFObservation(); ok && observation.Status == "decoded" {
				fields, _ := e.NativeWCFFields()
				require.Equal(t, uint64(2), fields["Observed Session Index"])
				admitted++
				// Public history edits must neither forge continuation evidence
				// nor rewrite the current record's owned native projection.
				e.Protocol = "tls"
				e.Session = map[string]any{"Upgrade Accepted": true, "Observed Session Index": 999}
				again, _ := e.NativeWCFFields()
				require.Equal(t, uint64(2), again["Observed Session Index"])
			}
		}
		if mode <= 1 {
			require.Equal(t, 6, admitted)
		} else {
			require.Zero(t, admitted)
		}
		for _, e := range append(events, s.Close("FIN")...) {
			if o, ok := e.NativeWCFTLSCarrier(); ok && o.Status != "decoded" && o.Status != "deferred" {
				_, err := e.GetFields()
				require.Error(t, err, "retained carrier context is not record admission")
			}
		}
		require.Zero(t, s.f.a.buffered.Load())
	})
}

func TestWCFTLSIncompleteCarrierContextCannotAuthorizeRecordQuery(t *testing.T) {
	s := wcfTLSReturnTestStart(t, 0)
	r := s.Feed(0, time.Unix(43, 0), []byte{23, 3, 3, 0, 32, 1, 2, 3})
	require.True(t, r.NeedMore)
	events := s.Close("FIN")
	require.Len(t, events, 1)
	e := events[0]
	o, ok := e.NativeWCFTLSCarrier()
	require.True(t, ok)
	require.Equal(t, "incomplete", o.Status)
	context, ok := e.NativeWCFTLSFields()
	require.True(t, ok)
	require.NotEmpty(t, context["Observed WCF Carrier"])
	e.Protocol, e.Status = "http3", "decoded"
	e.Fields = map[string]any{"forged": true}
	e.Session = map[string]any{"Authentication Verified": true}
	_, err := e.GetFields()
	require.Error(t, err, "incomplete carrier context cannot authorize a complete TLS record query")
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrNeedMore, typed.Kind)
}
