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

func mmsAccessReadFile(t *testing.T, file string) []byte {
	t.Helper()
	var b []byte
	var err error
	if file != "manifest.json" && len(file) > 7 && file[:7] == "common/" {
		repo, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(repo, file))
	} else if root := os.Getenv("MMS_ACCESS_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, file))
	} else {
		b, err = trafficfixture.ReadFile("mms-access-selectors/" + file)
	}
	require.NoError(t, err)
	return b
}
func mmsAccessCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(mmsAccessReadFile(t, "manifest.json"), &v))
	require.Equal(t, 1, v.SchemaVersion)
	require.Len(t, v.Cases, 106)
	if os.Getenv("MMS_ACCESS_TEST_ROOT") == "" {
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
			f, ok := indexed["mms-access-selectors/"+c.ID]
			require.True(t, ok)
			require.Equal(t, "mms-access-selectors/"+c.Capture, f.Input.File)
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
func mmsAccessInput(t *testing.T, c mmsISOCase) ([]byte, mmsISOAnswer) {
	t.Helper()
	raw, ans := mmsAccessReadFile(t, c.Capture), mmsAccessReadFile(t, c.Answer)
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
func TestMMSAccessSealedSession(t *testing.T) {
	for _, c := range mmsAccessCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsAccessInput(t, c)
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
func TestMMSAccessSealedReplay(t *testing.T) {
	for _, c := range mmsAccessCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := mmsAccessInput(t, c)
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

func TestMMSAccessExistingAPIBaseline(t *testing.T) {
	for _, c := range mmsAccessCases(t) {
		if c.ID != "entry-read-numeric" && c.ID != "entry-read-descriptor-bool" && c.ID != "entry-read-invalidated" && c.ID != "adjacent-read" {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsAccessInput(t, c)
			s := mmsValuesNew(t, DefaultParserBudget())
			defer func() { s.Close("end"); require.Zero(t, s.Stats().BufferedBytes) }()
			var events []*ProtocolEvent
			for i, step := range a.Steps {
				w, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), w)
				require.Len(t, out.Events, 1)
				_, err = out.Events[0].GetFields()
				require.NoError(t, err, "existing selected VariableSpecification grammar")
				events = append(events, out.Events...)
			}
			events = append(events, s.Close("end")...)
			mmsValuesCheck(t, events, a, false)
		})
	}
}

func mmsAccessAnswer(t *testing.T, id string) mmsISOAnswer {
	t.Helper()
	for _, c := range mmsAccessCases(t) {
		if c.ID == id {
			_, a := mmsAccessInput(t, c)
			return a
		}
	}
	t.Fatalf("missing access case %s", id)
	return mmsISOAnswer{}
}
func TestMMSAccessPrivateMemberOwnership(t *testing.T) {
	for _, id := range []string{"entry-defined-bytes", "entry-defined-descriptor-array", "entry-lookup-descriptor-structure", "bytes-reported"} {
		t.Run(id, func(t *testing.T) {
			a := mmsAccessAnswer(t, id)
			s := mmsValuesNew(t, DefaultParserBudget())
			var all []*ProtocolEvent
			for i, step := range a.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				clear(raw)
				for _, e := range out.Events {
					wire := bytes.Clone(e.Raw)
					f, err := e.GetFields()
					require.NoError(t, err)
					original := cloneSession(f)
					mutateRTPSQuery(f)
					mutateRTPSQuery(e.Session)
					clear(e.Raw)
					e.Raw = wire
					e.Session = cloneSession(original)
					after, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, original, after)
					all = append(all, e)
				}
			}
			all = append(all, s.Close("end")...)
			mmsValuesCheck(t, all, a, false)
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	a := mmsAccessAnswer(t, "entry-defined-descriptor-array")
	s := mmsValuesNew(t, DefaultParserBudget())
	events := mmsISOFeed(t, s, a, 5)
	require.Len(t, events, 5)
	snapshot := cloneSession(events[4].Session["Observed Context"].(map[string]any))
	closeEvents := s.Close("pending")
	require.Len(t, closeEvents, 1)
	rocEqualFields(t, map[string]any{"Observed Context": snapshot}, closeEvents[0].Session)
	require.Zero(t, closeEvents[0].ResponseTo)
	require.Zero(t, closeEvents[0].TransactionID)
	_, err := closeEvents[0].GetFields()
	rocTypedError(t, "NeedMore", err)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestMMSAccessDerivedMetadataBudget(t *testing.T) {
	a := mmsAccessAnswer(t, "retained-large-address")
	at := len(a.Steps) - 6
	s := mmsValuesNew(t, DefaultParserBudget()).(*captureSession)
	seed := mmsISOFeed(t, s, a, at)
	require.Len(t, seed, at)
	for _, e := range seed {
		_, err := e.GetFields()
		require.NoError(t, err)
	}
	raw, err := hex.DecodeString(a.Steps[at].Hex)
	require.NoError(t, err)
	foreign := int64(s.f.a.config.MaxBufferedBytes) - s.Stats().BufferedBytes - 32768 - s.f.mms.projectionFactor(s.f.a.budget)*int64(len(raw)) - 65536
	require.Greater(t, foreign, int64(0))
	require.True(t, s.f.a.reserveEvidence(foreign))
	before := s.Stats()
	require.Equal(t, ProbeAccept, s.Probe(raw).Verdict)
	require.Equal(t, before, s.Stats())
	out := s.Feed(0, time.Unix(100, 0), raw)
	require.Len(t, out.Events, 1)
	bad := out.Events[0]
	_, err = bad.GetFields()
	assertOwnedProtocolRefusal(t, bad, "ResourceExceeded", err)
	require.Zero(t, bad.ResponseTo)
	require.Zero(t, bad.TransactionID)
	require.Nil(t, bad.Session)
	require.Equal(t, foreign, s.Stats().BufferedBytes)
	late, _ := hex.DecodeString(a.Steps[at+1].Hex)
	later := s.Feed(1, time.Unix(101, 0), late)
	require.Empty(t, later.Events)
	rocTypedError(t, "FatalSessionError", later.Err)
	s.f.a.buffered.Add(-foreign)
	require.Empty(t, s.Close("end"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
