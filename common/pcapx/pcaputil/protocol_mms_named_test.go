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

func mmsNamedReadFile(t *testing.T, file string) []byte {
	t.Helper()
	var b []byte
	var err error
	if file != "manifest.json" && len(file) > 7 && file[:7] == "common/" {
		repo, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(repo, file))
	} else if root := os.Getenv("MMS_NAMED_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, file))
	} else {
		b, err = trafficfixture.ReadFile("mms-named-lists/" + file)
	}
	require.NoError(t, err)
	return b
}
func mmsNamedCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(mmsNamedReadFile(t, "manifest.json"), &v))
	require.Equal(t, 1, v.SchemaVersion)
	require.Len(t, v.Cases, 133)
	if os.Getenv("MMS_NAMED_TEST_ROOT") == "" {
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
			f, ok := indexed["mms-named-lists/"+c.ID]
			require.True(t, ok)
			require.Equal(t, "mms-named-lists/"+c.Capture, f.Input.File)
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
func mmsNamedInput(t *testing.T, c mmsISOCase) ([]byte, mmsISOAnswer) {
	t.Helper()
	raw, ans := mmsNamedReadFile(t, c.Capture), mmsNamedReadFile(t, c.Answer)
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
func TestMMSNamedSealedSession(t *testing.T) {
	for _, c := range mmsNamedCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsNamedInput(t, c)
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
func TestMMSNamedSealedReplay(t *testing.T) {
	for _, c := range mmsNamedCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := mmsNamedInput(t, c)
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

func mmsNamedAnswer(t *testing.T, id string) mmsISOAnswer {
	t.Helper()
	for _, c := range mmsNamedCases(t) {
		if c.ID == id {
			_, a := mmsNamedInput(t, c)
			return a
		}
	}
	t.Fatalf("missing named-list case %s", id)
	return mmsISOAnswer{}
}
func TestMMSNamedUnknownPartialRejectsLateBinding(t *testing.T) {
	for _, id := range []string{"late-delete-unknown-partial-7", "late-delete-unknown-partial-3"} {
		t.Run(id, func(t *testing.T) {
			a := mmsNamedAnswer(t, id)
			s := mmsValuesNew(t, DefaultParserBudget())
			defer func() { s.Close("end"); require.Zero(t, s.Stats().BufferedBytes) }()
			events := mmsISOFeed(t, s, a, len(a.Steps))
			require.Len(t, events, len(a.Events))
			last := events[len(events)-1]
			_, err := last.GetFields()
			assertOwnedProtocolRefusal(t, last, "ContextRequired", err)
			require.Zero(t, last.ResponseTo)
			require.Zero(t, last.TransactionID)
		})
	}
	a := mmsNamedAnswer(t, "delete-zero-restores-None")
	s := mmsValuesNew(t, DefaultParserBudget())
	events := mmsISOFeed(t, s, a, len(a.Steps))
	events = append(events, s.Close("end")...)
	mmsValuesCheck(t, events, a, false)
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestMMSNamedPrivateContextOwnership(t *testing.T) {
	a := mmsNamedAnswer(t, "cycle-domain-3")
	s := mmsValuesNew(t, DefaultParserBudget())
	var all []*ProtocolEvent
	for i, step := range a.Steps {
		raw, err := hex.DecodeString(step.Hex)
		require.NoError(t, err)
		out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
		clear(raw)
		for _, event := range out.Events {
			savedRaw := bytes.Clone(event.Raw)
			fields, err := event.GetFields()
			require.NoError(t, err)
			snapshot := cloneSession(fields)
			// Replace the public views after mutation; restoring a view does not repair
			// any mistakenly retained pointer to its old, corrupted map/buffer.
			if app, ok := fields["MMS"].(map[string]any); ok {
				if members, ok := app["Variables"].([]map[string]any); ok && len(members) > 0 {
					members[0]["Item ID"] = "CALLER"
				}
			}
			mutateRTPSQuery(event.Session)
			clear(event.Raw)
			event.Raw = savedRaw
			event.Session = cloneSession(snapshot)
			owned, err := event.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, snapshot, owned)
			all = append(all, event)
		}
	}
	all = append(all, s.Close("end")...)
	mmsValuesCheck(t, all, a, false)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestMMSNamedDerivedProjectionBudget(t *testing.T) {
	a := mmsNamedAnswer(t, "derived-members-budget")
	s := mmsValuesNew(t, DefaultParserBudget()).(*captureSession)
	seed := mmsISOFeed(t, s, a, 6)
	require.Len(t, seed, 6)
	for _, event := range seed {
		_, err := event.GetFields()
		require.NoError(t, err)
	}
	raw, err := hex.DecodeString(a.Steps[6].Hex)
	require.NoError(t, err)
	foreign := int64(s.f.a.config.MaxBufferedBytes) - s.Stats().BufferedBytes - 32768 - s.f.mms.projectionFactor(s.f.a.budget)*int64(len(raw)) - 65536
	require.Greater(t, foreign, int64(0))
	require.True(t, s.f.a.reserveEvidence(foreign))
	before := s.Stats()
	require.Equal(t, ProbeAccept, s.Probe(raw).Verdict)
	require.Equal(t, before, s.Stats())
	result := s.Feed(0, time.Unix(7, 0), raw)
	require.Len(t, result.Events, 1)
	bad := result.Events[0]
	_, err = bad.GetFields()
	assertOwnedProtocolRefusal(t, bad, "ResourceExceeded", err)
	require.Zero(t, bad.ResponseTo)
	require.Zero(t, bad.TransactionID)
	require.Nil(t, bad.Session)
	require.Equal(t, foreign, s.Stats().BufferedBytes)
	later, err := hex.DecodeString(a.Steps[7].Hex)
	require.NoError(t, err)
	reply := s.Feed(1, time.Unix(8, 0), later)
	require.Empty(t, reply.Events)
	rocTypedError(t, "FatalSessionError", reply.Err)
	s.f.a.buffered.Add(-foreign)
	require.Empty(t, s.Close("end"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestMMSNamedDerivedCollectionBudget(t *testing.T) {
	a := mmsNamedAnswer(t, "derived-collection-budget")
	b := DefaultParserBudget()
	b.MaxCollectionElements = 67
	s := mmsValuesNew(t, b)
	events := mmsISOFeed(t, s, a, 6)
	require.Len(t, events, 6)
	for _, event := range events {
		_, err := event.GetFields()
		require.NoError(t, err)
	}
	raw, err := hex.DecodeString(a.Steps[6].Hex)
	require.NoError(t, err)
	result := s.Feed(0, time.Unix(7, 0), raw)
	require.Len(t, result.Events, 1)
	_, err = result.Events[0].GetFields()
	assertOwnedProtocolRefusal(t, result.Events[0], "ResourceExceeded", err)
	require.Zero(t, result.Events[0].TransactionID)
	require.Zero(t, result.Events[0].ResponseTo)
	require.Empty(t, s.Close("end"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestMMSNamedSessionIsolationAndPendingClose(t *testing.T) {
	a := mmsNamedAnswer(t, "cycle-domain-1")
	first := mmsValuesNew(t, DefaultParserBudget())
	second := mmsValuesNew(t, DefaultParserBudget())
	require.Len(t, mmsISOFeed(t, first, a, 6), 6)
	require.Len(t, mmsISOFeed(t, second, a, 4), 4)
	w, err := hex.DecodeString(a.Steps[8].Hex)
	require.NoError(t, err)
	result := second.Feed(0, time.Unix(9, 0), w)
	require.Len(t, result.Events, 1)
	_, err = result.Events[0].GetFields()
	assertOwnedProtocolRefusal(t, result.Events[0], "ContextRequired", err)
	require.Empty(t, second.Close("end"))
	require.Zero(t, second.Stats().BufferedBytes)
	require.Empty(t, first.Close("end"))
	require.Zero(t, first.Stats().BufferedBytes)
	// A pending definition holds both candidate and prior observations privately,
	// produces one close diagnostic, and releases registry/member history.
	a = mmsNamedAnswer(t, "successful-redefinition")
	s := mmsValuesNew(t, DefaultParserBudget())
	require.Len(t, mmsISOFeed(t, s, a, 7), 7)
	closed := s.Close("end")
	require.Len(t, closed, 1)
	fields, err := closed[0].GetFields()
	rocTypedError(t, "NeedMore", err)
	require.Nil(t, fields)
	require.Zero(t, closed[0].ResponseTo)
	require.Zero(t, closed[0].TransactionID)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
