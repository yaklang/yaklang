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

func mmsDescriptionsReadFile(t *testing.T, file string) []byte {
	t.Helper()
	var b []byte
	var err error
	if file != "manifest.json" && len(file) > 7 && file[:7] == "common/" {
		repo, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(repo, file))
	} else if root := os.Getenv("MMS_DESCRIPTIONS_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, file))
	} else {
		b, err = trafficfixture.ReadFile("mms-described-data/" + file)
	}
	require.NoError(t, err)
	return b
}
func mmsDescriptionsCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(mmsDescriptionsReadFile(t, "manifest.json"), &v))
	require.Equal(t, 1, v.SchemaVersion)
	require.Len(t, v.Cases, 67)
	if os.Getenv("MMS_DESCRIPTIONS_TEST_ROOT") == "" {
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
			f, ok := indexed["mms-described-data/"+c.ID]
			require.True(t, ok)
			require.Equal(t, "mms-described-data/"+c.Capture, f.Input.File)
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
func mmsDescriptionsInput(t *testing.T, c mmsISOCase) ([]byte, mmsISOAnswer) {
	t.Helper()
	raw, ans := mmsDescriptionsReadFile(t, c.Capture), mmsDescriptionsReadFile(t, c.Answer)
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
func TestMMSDescriptionsSealedSession(t *testing.T) {
	for _, c := range mmsDescriptionsCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsDescriptionsInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s := mmsDescriptionsNew(t, DefaultParserBudget())
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
					mmsValuesCheckProfile(t, events, a, false, "mms-iso-described-data")
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
func TestMMSDescriptionsSealedReplay(t *testing.T) {
	for _, c := range mmsDescriptionsCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := mmsDescriptionsInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", worker, deferred, observer), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							opts := []CaptureOption{WithProtocolMMSDescriptionValidation(102), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
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
							mmsValuesCheckProfile(t, events, a, true, "mms-iso-described-data")
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
						})
					}
				}
			}
		})
	}
}

func mmsDescriptionsNew(t *testing.T, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(41000, 102), WithSessionMMSDescriptionValidation())
	require.NoError(t, err)
	return s
}
func TestMMSDescriptionsConfigurationAndDefault(t *testing.T) {
	// Ordinary capture with no bin-parser configuration remains usable.
	raw := mmsDescriptionsReadFile(t, "captures/plain-named-observation.pcap")
	require.NoError(t, ReplayPcap(bytes.NewReader(raw)))
	for _, config := range []BinParserConfig{{ValidateMMSDescriptions: true}, {ValidateMMSDescriptions: true, DecodeMMSServiceValues: true}, {ValidateMMSDescriptions: true, MMSISOPorts: []uint16{102}}} {
		require.Error(t, ReplayPcap(bytes.NewReader(raw), WithBinParserConfig(config)))
	}
	for _, opts := range [][]ProtocolSessionOption{{WithSessionMMSDescriptionValidation(), WithSessionMMSServiceValues()}, {WithSessionMMSServiceValues(), WithSessionMMSDescriptionValidation()}} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), append([]ProtocolSessionOption{WithSessionTransport("tcp"), WithSessionPorts(41000, 102)}, opts...)...)
		require.NoError(t, err)
		require.True(t, s.(*captureSession).f.mms.validateDescriptions)
		require.Equal(t, "mms-iso-described-data", s.(*captureSession).f.mms.profile())
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	for _, opts := range [][]ProtocolSessionOption{{WithSessionTransport("udp"), WithSessionMMSDescriptionValidation()}, {WithSessionMMSDescriptionValidation(), WithSessionTransport("udp")}} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
	cfg := NewDefaultConfig()
	require.Error(t, WithProtocolMMSDescriptionValidation(0)(cfg))
	// Existing literal profile keeps its exact complete field/session contract.
	a := mmsAccessAnswer(t, "entry-read-descriptor-array")
	s := mmsValuesNew(t, DefaultParserBudget())
	events := mmsISOFeed(t, s, a, len(a.Steps))
	events = append(events, s.Close("end")...)
	mmsValuesCheck(t, events, a, false)
	require.Zero(t, s.Stats().BufferedBytes)
}

func mmsDescriptionsAnswer(t *testing.T, id string) mmsISOAnswer {
	t.Helper()
	for _, c := range mmsDescriptionsCases(t) {
		if c.ID == id {
			_, a := mmsDescriptionsInput(t, c)
			return a
		}
	}
	t.Fatalf("missing described case %s", id)
	return mmsISOAnswer{}
}
func TestMMSDescriptionsPrivateSchemaOwnership(t *testing.T) {
	for _, id := range []string{"read-array", "named-array-valid", "named-lookup-valid", "interleaved-described-ids"} {
		t.Run(id, func(t *testing.T) {
			a := mmsDescriptionsAnswer(t, id)
			s := mmsDescriptionsNew(t, DefaultParserBudget())
			var all []*ProtocolEvent
			for i, step := range a.Steps {
				w, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), w)
				clear(w)
				for _, event := range out.Events {
					raw := bytes.Clone(event.Raw)
					fields, err := event.GetFields()
					require.NoError(t, err)
					snapshot := cloneSession(fields)
					mutateRTPSQuery(fields)
					mutateRTPSQuery(event.Session)
					clear(event.Raw)
					event.Raw = raw
					event.Session = cloneSession(snapshot)
					after, err := event.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, snapshot, after)
					all = append(all, event)
				}
			}
			all = append(all, s.Close("end")...)
			mmsValuesCheckProfile(t, all, a, false, "mms-iso-described-data")
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestMMSDescriptionsRefusalAndPendingClose(t *testing.T) {
	a := mmsDescriptionsAnswer(t, "bad-read-array-element")
	s := mmsDescriptionsNew(t, DefaultParserBudget())
	seed := mmsISOFeed(t, s, a, 5)
	require.Len(t, seed, 5)
	w, err := hex.DecodeString(a.Steps[5].Hex)
	require.NoError(t, err)
	out := s.Feed(1, time.Unix(6, 0), w)
	require.Len(t, out.Events, 1)
	bad := out.Events[0]
	_, err = bad.GetFields()
	assertOwnedProtocolRefusal(t, bad, "MalformedMessage", err)
	require.Zero(t, bad.ResponseTo)
	require.Zero(t, bad.TransactionID)
	require.Nil(t, bad.Session)
	require.Zero(t, s.Stats().BufferedBytes)
	late := s.Feed(1, time.Unix(7, 0), w)
	require.Empty(t, late.Events)
	rocTypedError(t, "FatalSessionError", late.Err)
	require.Empty(t, s.Close("end"))
	require.Empty(t, s.Close("again"))
	a = mmsDescriptionsAnswer(t, "read-array")
	s = mmsDescriptionsNew(t, DefaultParserBudget())
	seed = mmsISOFeed(t, s, a, 5)
	require.Len(t, seed, 5)
	snapshot := cloneSession(seed[4].Session["Observed Context"].(map[string]any))
	closed := s.Close("pending")
	require.Len(t, closed, 1)
	rocEqualFields(t, map[string]any{"Observed Context": snapshot}, closed[0].Session)
	require.Zero(t, closed[0].ResponseTo)
	require.Zero(t, closed[0].TransactionID)
	_, err = closed[0].GetFields()
	rocTypedError(t, "NeedMore", err)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
