package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

// Once a UDP conversation expires, identical wire IDs cannot identify whether
// the next reply was emitted for the expired request or its later reuse.
// Expiration must not manufacture a fresh trustworthy exchange identity.
func TestDLMSUDPIdleReuseExistingAPI(t *testing.T) {
	t.Run("completed-fresh", testDLMSUDPIdleFreshIdentityExistingAPI)
	native := dlmsWires(t, "get-u8")
	var wrapper [][]byte
	for _, c := range wrapperStructuredControls(t) {
		if c.Name == "structured-exchange-udp" {
			wrapper = [][]byte{wrapperWire(t, c.Steps[0].Hex), wrapperWire(t, c.Steps[1].Hex)}
		}
	}
	require.Len(t, wrapper, 2)
	for name, wires := range map[string][][]byte{"native": native[:2], "wrapper": wrapper} {
		t.Run(name, func(t *testing.T) {
			for _, deferred := range []bool{false, true} {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				defer s.Close("cleanup")
				a := s.Feed(0, time.Unix(100, 0), wires[0])
				require.Len(t, a.Events, 1)
				_, err = a.Events[0].GetFields()
				require.NoError(t, err)
				b := s.Feed(0, time.Unix(131, 0), wires[0])
				require.Len(t, b.Events, 1)
				late := s.Feed(1, time.Unix(131, 0), wires[1])
				require.Len(t, late.Events, 1)
				require.Zero(t, late.Events[0].ResponseTo, "late reply must not acquire request B ID %d after A expired", b.Events[0].ID)
				require.Zero(t, late.Events[0].TransactionID)
				require.Empty(t, s.Close("expired"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				// An adjacent fully observed exchange remains usable before the boundary.
				s, err = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				a = s.Feed(0, time.Unix(100, 0), wires[0])
				late = s.Feed(1, time.Unix(129, 0), wires[1])
				require.Len(t, a.Events, 1)
				require.Len(t, late.Events, 1)
				_, err = late.Events[0].GetFields()
				require.NoError(t, err)
				require.Equal(t, a.Events[0].ID, late.Events[0].ResponseTo)
				require.Empty(t, s.Close("normal"))
				require.Zero(t, s.Stats().BufferedBytes)
			}
		})
	}
}

func TestDLMSUDPOtherProtocolExpirationExistingAPI(t *testing.T) {
	wires := dlmsWires(t, "get-u8")
	for name, trigger := range map[string]func(*binParser) bool{
		"STUN": func(a *binParser) bool {
			return a.decodeSTUNDatagram(&ProtocolEvent{Source: "x:3478", Destination: "y:40000", Timestamp: time.Unix(700, 0), Transport: "udp"}, wrapperWire(t, "000100002112a4420102030405060708090a0b0c"))
		},
		"SNMP": func(a *binParser) bool {
			return a.decodeSNMPDatagram(&ProtocolEvent{Source: "x:161", Destination: "y:40000", Timestamp: time.Unix(700, 0), Transport: "udp"}, wrapperWire(t, "302602010104067075626c6963a019020101020100020100300e300c06082b060102010101000500"), "snmp")
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			a := s.Feed(0, time.Unix(100, 0), wires[0])
			require.Len(t, a.Events, 1)
			require.True(t, trigger(s.(*captureSession).f.a), "trigger must be actually admitted")
			b := s.Feed(0, time.Unix(700, 0), wires[0])
			require.Len(t, b.Events, 1)
			late := s.Feed(1, time.Unix(700, 0), wires[1])
			require.Len(t, late.Events, 1)
			require.Zero(t, late.Events[0].ResponseTo, "other protocol expiry must not erase DLMS identity quarantine")
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

type dlmsIdleControl struct {
	ID, Protocol, Profile, Capture, Answer, SHA256 string
	AnswerSHA                                      string `json:"answer_sha256"`
	InputAlias                                     string `json:"input_alias"`
	Packets                                        int
	Events                                         []struct {
		Raw, Error      string
		Direction       int
		Timestamp       int64 `json:"timestamp_sec"`
		Fields, Session map[string]any
		Reply           *int `json:"response_to_index"`
		Transaction     *int `json:"transaction_index"`
	}
}

func dlmsIdleControls(t *testing.T) []dlmsIdleControl {
	return dlmsIdleControlsFrom(t, "dlms-udp-idle", "owned-dlms-udp-idle/v1", 12)
}
func dlmsIdleControlsFrom(t *testing.T, folder, schema string, count int) []dlmsIdleControl {
	t.Helper()
	b, err := trafficfixture.ReadFile(folder + "/controls.json")
	require.NoError(t, err)
	var d struct {
		Schema string
		Cases  []dlmsIdleControl
	}
	require.NoError(t, json.Unmarshal(b, &d))
	require.Equal(t, schema, d.Schema)
	require.Len(t, d.Cases, count)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range d.Cases {
		ab, err := trafficfixture.ReadFile(folder + "/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
		var a dlmsIdleControl
		require.NoError(t, json.Unmarshal(ab, &a))
		require.Equal(t, c.Events, a.Events)
		bound := 0
		for _, batch := range all {
			for _, row := range batch.Cases {
				if row.ID != folder+"/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, row.Input.SHA256)
				require.Equal(t, c.InputAlias, row.Input.OriginalPath)
				require.Equal(t, c.Packets, row.Facts.PacketCount)
				bound++
			}
		}
		require.Equal(t, 1, bound)
	}
	return d.Cases
}
func TestDLMSUDPIdleSealedMatrix(t *testing.T) {
	runDLMSUDPIdleMatrix(t, "dlms-udp-idle", dlmsIdleControls(t))
	runDLMSUDPIdleMatrix(t, "dlms-udp-adjacent", dlmsIdleControlsFrom(t, "dlms-udp-adjacent", "owned-dlms-udp-adjacent/v1", 18))
}
func runDLMSUDPIdleMatrix(t *testing.T, folder string, controls []dlmsIdleControl) {
	for _, c := range controls {
		t.Run(c.ID, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile(folder + "/" + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolDecodeAs("udp", 4059, c.Protocol))
				require.Len(t, events, len(c.Events))
				ids := map[uint64]bool{}
				for i, e := range events {
					w := c.Events[i]
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.Equal(t, c.Protocol, e.Protocol)
					require.Equal(t, c.Profile, e.Profile)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, w.Timestamp, e.Timestamp.Unix())
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, events[0].FlowID, e.FlowID)
					require.Equal(t, events[0].Domain, e.Domain)
					src, dst := "192.0.2.1:40000", "192.0.2.2:4059"
					if w.Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					if w.Reply == nil {
						require.Zero(t, e.ResponseTo)
					} else {
						require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
					}
					if w.Transaction == nil {
						require.Zero(t, e.TransactionID)
					} else {
						require.Equal(t, events[*w.Transaction].ID, e.TransactionID)
					}
					f, err := e.GetFields()
					rocTypedError(t, w.Error, err)
					if w.Error != "" {
						require.Nil(t, f)
						require.Nil(t, e.Session)
						require.Nil(t, e.Structured)
						require.NotEmpty(t, e.Error)
						continue
					}
					require.Empty(t, e.Error)
					rocEqualFields(t, w.Fields, f)
					rocEqualFields(t, w.Session, e.Session)
					poisonWrapperStructured(f)
					e.Session["Association"] = "caller-mutated"
					again, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, again)
				}
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}
func TestDLMSUDPIdleQuarantineBudgetAndDomains(t *testing.T) {
	t.Run("completed-history", testDLMSUDPIdleCompletedHistoryBudget)
	native := dlmsWires(t, "get-u8")
	q, r := native[0], native[1]
	b := DefaultParserBudget()
	b.MaxCollectionElements = 1
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"))
	require.NoError(t, err)
	defer s.Close("cleanup")
	a := s.(*captureSession).f.a
	feed := func(src, dst string, domain CaptureDomain, at int64, w []byte) *ProtocolEvent {
		e := &ProtocolEvent{Source: src, Destination: dst, Domain: domain, Timestamp: time.Unix(at, 0), Transport: "udp"}
		require.True(t, a.decodeDLMSDatagram(e, w, true))
		return e
	}
	d := CaptureDomain{Section: 1, Interface: 0}
	first := feed("a:40000", "b:4059", d, 100, q)
	require.Nil(t, first.sessionError)
	expired := feed("b:4059", "a:40000", d, 130, r)
	rocTypedError(t, "ContextRequired", expired.sessionError)
	require.Equal(t, first.FlowID, expired.FlowID)
	require.Zero(t, expired.ResponseTo)
	require.EqualValues(t, 512, s.Stats().BufferedBytes)
	rejected := feed("a:40000", "b:4059", CaptureDomain{Section: 1, Interface: 1}, 130, q)
	rocTypedError(t, "ResourceExceeded", rejected.sessionError)
	require.Zero(t, rejected.ResponseTo)
	require.Len(t, a.udpSessions.entries, 1)
	require.EqualValues(t, 512, s.Stats().BufferedBytes)
	require.Empty(t, s.Close("bounded"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	s, err = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"))
	require.NoError(t, err)
	defer s.Close("cleanup")
	a = s.(*captureSession).f.a
	first = feed("a:40000", "b:4059", d, 100, q)
	expired = feed("b:4059", "a:40000", d, 130, r)
	rocTypedError(t, "ContextRequired", expired.sessionError)
	for _, domain := range []CaptureDomain{{Section: 1, Interface: 1}, {Section: 2, Interface: 0}} {
		request := feed("a:40000", "b:4059", domain, 130, q)
		require.Nil(t, request.sessionError)
		response := feed("b:4059", "a:40000", domain, 130, r)
		require.Nil(t, response.sessionError)
		require.Equal(t, request.ID, response.ResponseTo)
		require.NotEqual(t, first.FlowID, response.FlowID)
		require.Equal(t, domain, response.Domain)
	}
	retry := feed("a:40000", "b:4059", d, 100, q)
	rocTypedError(t, "ContextRequired", retry.sessionError)
	late := feed("b:4059", "a:40000", d, 100, r)
	rocTypedError(t, "ContextRequired", late.sessionError)
	require.Zero(t, late.ResponseTo)
	require.Empty(t, s.Close("domains"))
	require.Zero(t, s.Stats().BufferedBytes)
}
