package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func mmsValuesRead(t *testing.T, file string) []byte {
	t.Helper()
	var b []byte
	var err error
	if file != "manifest.json" && len(file) > 7 && file[:7] == "common/" {
		repo, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(repo, file))
	} else if root := os.Getenv("MMS_VALUES_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, file))
	} else {
		b, err = trafficfixture.ReadFile("mms-services-values/" + file)
	}
	require.NoError(t, err)
	return b
}
func mmsValuesCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(mmsValuesRead(t, "manifest.json"), &v))
	require.Equal(t, 1, v.SchemaVersion)
	require.Len(t, v.Cases, 166)
	if os.Getenv("MMS_VALUES_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		indexed := map[string]trafficfixture.FrozenCase{}
		for _, b := range batches {
			for _, c := range b.Cases {
				indexed[c.ID] = c
			}
		}
		for _, c := range v.Cases {
			if len(c.Capture) > 7 && c.Capture[:7] == "common/" {
				continue
			}
			f, ok := indexed["mms-services-values/"+c.ID]
			require.True(t, ok)
			require.Equal(t, "mms-services-values/"+c.Capture, f.Input.File)
			require.Equal(t, c.SHA256, f.Input.SHA256)
			require.Equal(t, c.Packets, f.Facts.PacketCount)
			require.Len(t, f.Expectations, 1)
			var bind struct {
				Answer string `json:"answer_file"`
				SHA    string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(f.Expectations[0].PayloadConstraints, &bind))
			require.Equal(t, c.Answer, bind.Answer)
			require.Equal(t, c.AnswerSHA, bind.SHA)
		}
	}
	return v.Cases
}
func mmsValuesInput(t *testing.T, c mmsISOCase) ([]byte, mmsISOAnswer) {
	t.Helper()
	raw, ans := mmsValuesRead(t, c.Capture), mmsValuesRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ans)))
	var a mmsISOAnswer
	require.NoError(t, json.Unmarshal(ans, &a))
	require.Equal(t, c.Packets, a.Packets)
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	var steps []struct {
		Dir int
		Hex string
	}
	packets := 0
	for {
		b, _, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		packets++
		p := gopacket.NewPacket(b, r.LinkType(), gopacket.Default)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if len(tcp.Payload) > 0 {
			dir := 0
			if tcp.SrcPort == 102 {
				dir = 1
			}
			steps = append(steps, struct {
				Dir int
				Hex string
			}{dir, hex.EncodeToString(tcp.Payload)})
		}
	}
	require.Equal(t, c.Packets, packets)
	require.Equal(t, a.Steps, steps)
	require.LessOrEqual(t, len(a.Events), len(a.Steps))
	return raw, a
}
func mmsValuesCheck(t *testing.T, events []*ProtocolEvent, a mmsISOAnswer, replay bool) {
	t.Helper()
	require.Len(t, events, len(a.Events))
	for i, e := range events {
		want := a.Events[i]
		require.Equal(t, "mms", e.Protocol)
		require.Equal(t, mmsServicesProfile, e.Profile)
		require.EqualValues(t, i+1, e.ID)
		require.Equal(t, want.Dir, e.Direction)
		require.Equal(t, want.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, want.ResponseTo, e.ResponseTo)
		require.Equal(t, want.TransactionID, e.TransactionID)
		require.Equal(t, want.Completeness, e.Completeness)
		f, err := e.GetFields()
		if want.Error != "" {
			require.Nil(t, f)
			assertOwnedProtocolRefusal(t, e, want.Error, err)
			require.Nil(t, e.Session)
		} else {
			require.NoError(t, err)
			fields := want.Fields
			if _, ok := fields["Contributor Packet Refs"]; ok && replay {
				fields = cloneSession(fields)
				delete(fields, "Contributor Packet Refs")
				actual := cloneSession(f)
				delete(actual, "Contributor Packet Refs")
				rocEqualFields(t, fields, actual)
				ids := want.Fields["Contributor Event IDs"].([]any)
				wantRefs := make([]PacketReference, len(ids))
				for j, v := range ids {
					wantRefs[j] = PacketReference{Number: uint64(v.(float64)) + 3, Domain: e.Domain}
				}
				require.Equal(t, wantRefs, f["Contributor Packet Refs"])
			} else {
				rocEqualFields(t, fields, f)
			}
			rocEqualFields(t, f, e.Session)
			original := cloneSession(f)
			mutateRTPSQuery(f)
			mutateRTPSQuery(e.Session)
			clear(e.Raw)
			testNativeObservationSessionQueries(t, e, original)
		}
		if replay {
			require.Equal(t, "tcp", e.Transport)
			src, dst := "192.0.2.1:41000", "192.0.2.2:102"
			if cOrig := a.Packets == 22 && len(a.Steps) == 12; cOrig {
				src, dst = "172.16.0.101:1345", "172.16.202.5:102"
			}
			require.Equal(t, [2]string{src, dst}[want.Dir], e.Source)
			require.Equal(t, [2]string{dst, src}[want.Dir], e.Destination)
			number := uint64(i + 4)
			if a.Packets == 22 && len(a.Steps) == 12 {
				number = []uint64{4, 6, 7, 8, 10, 12, 13, 14, 15, 16, 17, 18}[i]
			}
			require.Equal(t, []PacketReference{{Number: number, Domain: e.Domain}}, e.SourceBytes.PacketRefs)
			wire, err := hex.DecodeString(want.Raw)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wire)), e.SourceBytes.SHA256)
			require.Empty(t, e.SourceBytes.ParentPDUs)
			require.Zero(t, e.SourceBytes.ParentPDU)
			for _, r := range e.SourceBytes.PacketRefs {
				require.Equal(t, e.Domain, r.Domain)
			}
		}
	}
}
func mmsValuesNew(t *testing.T, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(41000, 102), WithSessionMMSServiceValues())
	require.NoError(t, err)
	return s
}
func TestMMSValuesSealedSession(t *testing.T) {
	for _, c := range mmsValuesCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsValuesInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s := mmsValuesNew(t, DefaultParserBudget())
					var events []*ProtocolEvent
					for i, step := range a.Steps {
						w, err := hex.DecodeString(step.Hex)
						require.NoError(t, err)
						before := s.Stats()
						s.Probe(w)
						s.Probe(w)
						require.Equal(t, before, s.Stats())
						for at := 0; at < len(w); at += chunk {
							fragment := bytes.Clone(w[at:min(at+chunk, len(w))])
							r := s.Feed(step.Dir, time.Unix(1700000000, int64(i)*1000), fragment)
							events = append(events, r.Events...)
							clear(fragment)
						}
					}
					events = append(events, s.Close("end")...)
					mmsValuesCheck(t, events, a, false)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
					r := s.Feed(0, time.Unix(1, 0), []byte("late"))
					require.Empty(t, r.Events)
					rocTypedError(t, "FatalSessionError", r.Err)
				})
			}
		})
	}
}
func TestMMSValuesSealedReplay(t *testing.T) {
	for _, c := range mmsValuesCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := mmsValuesInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", worker, deferred, observer), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							opts := []CaptureOption{WithProtocolMMSServiceValues(102), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							seen := 0
							if observer {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							if observer {
								require.Equal(t, c.Packets, seen)
							}
							mmsValuesCheck(t, events, a, true)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
						})
					}
				}
			}
		})
	}
}

func mmsValuesAnswer(t *testing.T, id string) mmsISOAnswer {
	for _, c := range mmsValuesCases(t) {
		if c.ID == id {
			_, a := mmsValuesInput(t, c)
			return a
		}
	}
	t.Fatalf("missing %s", id)
	return mmsISOAnswer{}
}
func TestMMSValuesBudgetsLifecycleAndOwnership(t *testing.T) {
	a := mmsValuesAnswer(t, "returned-spec-domain")
	for _, b := range []ParserBudget{{MaxRecursionDepth: 10}, {MaxCollectionElements: 16}} {
		s := mmsValuesNew(t, b)
		events := mmsISOFeed(t, s, a, 5)
		require.NotEmpty(t, events)
		bad := events[len(events)-1]
		_, err := bad.GetFields()
		assertOwnedProtocolRefusal(t, bad, "ResourceExceeded", err)
		require.Zero(t, bad.ResponseTo)
		require.Zero(t, bad.TransactionID)
		require.Zero(t, s.Stats().BufferedBytes)
		require.Empty(t, s.Close("end"))
	}
	s := mmsValuesNew(t, DefaultParserBudget())
	events := mmsISOFeed(t, s, a, 5)
	require.Len(t, events, 5)
	snapshot := cloneSession(events[4].Session)
	fields, err := events[4].GetFields()
	require.NoError(t, err)
	mutateRTPSQuery(fields)
	mutateRTPSQuery(events[4].Session)
	clear(events[4].Raw)
	w, _ := hex.DecodeString(a.Steps[5].Hex)
	r := s.Feed(1, time.Unix(6, 0), w)
	require.Len(t, r.Events, 1)
	f, err := r.Events[0].GetFields()
	require.NoError(t, err)
	rocEqualFields(t, a.Events[5].Fields, f)
	require.EqualValues(t, 5, r.Events[0].ResponseTo)
	require.EqualValues(t, 5, r.Events[0].TransactionID)
	require.True(t, snapshot["Observed Context"].(map[string]any)["Association Active"].(bool))
	require.Empty(t, s.Close("end"))
	require.Zero(t, s.Stats().BufferedBytes)
	// Pending returned-specification snapshots diagnose and release exactly once.
	s = mmsValuesNew(t, DefaultParserBudget())
	events = mmsISOFeed(t, s, a, 5)
	want := map[string]any{"Observed Context": cloneSession(events[4].Session["Observed Context"].(map[string]any))}
	closed := s.Close("end")
	require.Len(t, closed, 1)
	rocEqualFields(t, want, closed[0].Session)
	require.Zero(t, closed[0].ResponseTo)
	require.Zero(t, closed[0].TransactionID)
	_, err = closed[0].GetFields()
	rocTypedError(t, "NeedMore", err)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestMMSValuesSharedProjectionAndOIDBudget(t *testing.T) {
	a := mmsValuesAnswer(t, "oid-2a030405")
	w, _ := hex.DecodeString(a.Steps[0].Hex)
	b := ParserBudget{MaxMessageBytes: 4096, MaxFrameBytes: 4096, MaxRecursionDepth: 64, MaxBufferedBytes: 256 + 32768 + 1536*len(w) + 1000}
	s := mmsValuesNew(t, b).(*captureSession)
	require.True(t, s.f.a.reserveEvidence(1001))
	before := s.Stats()
	require.Equal(t, ProbeReject, s.Probe(w).Verdict)
	require.Equal(t, before, s.Stats())
	r := s.Feed(0, time.Unix(1, 0), w)
	require.Len(t, r.Events, 1)
	_, err := r.Events[0].GetFields()
	assertOwnedProtocolRefusal(t, r.Events[0], "ResourceExceeded", err)
	require.EqualValues(t, 1001, s.Stats().BufferedBytes)
	s.f.a.buffered.Add(-1001)
	s.Close("end")
	require.Zero(t, s.Stats().BufferedBytes)
	// Encoded OID subidentifiers share the remaining carrier/BER budget;
	// two OIDs in one Identify reply cannot each reset their own allowance.
	c := &mmsValueContext{left: 3}
	oid, err := c.oid([]byte{42, 3, 4})
	require.NoError(t, err)
	require.Equal(t, "1.2.3.4", oid)
	require.Zero(t, c.left)
	_, err = c.oid([]byte{42})
	rocTypedError(t, "ResourceExceeded", err)
}
func TestMMSValuesCarrierSelectionAndClose(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{{WithSessionTransport("udp"), WithSessionMMSServiceValues()}, {WithSessionMMSServiceValues(), WithSessionTransport("udp")}} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
	s := mmsValuesNew(t, DefaultParserBudget())
	r := s.Feed(2, time.Unix(1, 0), []byte{3})
	require.Empty(t, r.Events)
	require.Zero(t, r.Consumed)
	rocTypedError(t, "MalformedMessage", r.Err)
	require.Empty(t, s.Close("end"))
	require.Zero(t, s.Stats().BufferedBytes)
	r = s.Feed(0, time.Unix(1, 0), []byte("late"))
	require.Empty(t, r.Events)
	rocTypedError(t, "FatalSessionError", r.Err)
	cfg := NewDefaultConfig()
	require.Error(t, WithProtocolMMSServiceValues(0)(cfg))
}
