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

type mmsISOCase struct {
	ID, Capture, Answer, SHA256 string
	AnswerSHA                   string `json:"answer_sha256"`
	Packets                     int
}
type mmsISOAnswer struct {
	Steps []struct {
		Dir int
		Hex string
	}
	Events []struct {
		Raw           string
		Dir           int
		Fields        map[string]any
		Error         string
		ResponseTo    uint64 `json:"response_to"`
		TransactionID uint64 `json:"transaction_id"`
		Completeness  string
	}
	Packets int
}

func mmsISORead(t *testing.T, file string) []byte {
	t.Helper()
	var b []byte
	var err error
	if file != "manifest.json" && len(file) > 7 && file[:7] == "common/" {
		repo, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(repo, file))
	} else if root := os.Getenv("MMS_ISO_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, file))
	} else {
		b, err = trafficfixture.ReadFile("mms-iso/" + file)
	}
	require.NoError(t, err)
	return b
}
func mmsISOCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(mmsISORead(t, "manifest.json"), &v))
	require.Equal(t, 1, v.SchemaVersion)
	require.Len(t, v.Cases, 35)
	if os.Getenv("MMS_ISO_TEST_ROOT") == "" {
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
			f, ok := indexed["mms-iso/"+c.ID]
			require.True(t, ok)
			require.Equal(t, "mms-iso/"+c.Capture, f.Input.File)
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
func mmsISOInput(t *testing.T, c mmsISOCase) ([]byte, mmsISOAnswer) {
	t.Helper()
	raw, ans := mmsISORead(t, c.Capture), mmsISORead(t, c.Answer)
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
func mmsISOCheck(t *testing.T, events []*ProtocolEvent, a mmsISOAnswer, replay bool) {
	t.Helper()
	require.Len(t, events, len(a.Events))
	for i, e := range events {
		want := a.Events[i]
		require.Equal(t, "mms", e.Protocol)
		require.Equal(t, mmsISOProfile, e.Profile)
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
func mmsISONew(t *testing.T, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(41000, 102), WithSessionMMSISO())
	require.NoError(t, err)
	return s
}
func TestMMSISOSealedSession(t *testing.T) {
	for _, c := range mmsISOCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsISOInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s := mmsISONew(t, DefaultParserBudget())
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
					mmsISOCheck(t, events, a, false)
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
func TestMMSISOSealedReplay(t *testing.T) {
	for _, c := range mmsISOCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := mmsISOInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", worker, deferred, observer), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							opts := []CaptureOption{WithProtocolMMSISO(102), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
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
							mmsISOCheck(t, events, a, true)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
						})
					}
				}
			}
		})
	}
}

func mmsISOCaseAnswer(t *testing.T, id string) mmsISOAnswer {
	for _, c := range mmsISOCases(t) {
		if c.ID == id {
			_, a := mmsISOInput(t, c)
			return a
		}
	}
	t.Fatalf("missing %s", id)
	return mmsISOAnswer{}
}
func mmsISOFeed(t *testing.T, s ProtocolSession, a mmsISOAnswer, n int) []*ProtocolEvent {
	t.Helper()
	var out []*ProtocolEvent
	for i, v := range a.Steps[:n] {
		w, err := hex.DecodeString(v.Hex)
		require.NoError(t, err)
		out = append(out, s.Feed(v.Dir, time.Unix(int64(i+1), 0), w).Events...)
	}
	return out
}
func TestMMSISOBudgetsAndLifetime(t *testing.T) {
	a := mmsISOCaseAnswer(t, "normal-read-error")
	for _, b := range []ParserBudget{{MaxRecursionDepth: 2}, {MaxCollectionElements: 8}} {
		s := mmsISONew(t, b)
		events := mmsISOFeed(t, s, a, 3)
		require.Len(t, events, 3)
		f, err := events[2].GetFields()
		require.Nil(t, f)
		assertOwnedProtocolRefusal(t, events[2], "ResourceExceeded", err)
		require.Zero(t, events[2].ResponseTo)
		require.Zero(t, events[2].TransactionID)
		require.Zero(t, s.Stats().BufferedBytes)
		w, _ := hex.DecodeString(a.Steps[3].Hex)
		require.Empty(t, s.Feed(1, time.Unix(4, 0), w).Events)
		require.Empty(t, s.Close("end"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
	// Outstanding requests and incomplete TSDUs diagnose exactly once at Close.
	for _, id := range []string{"normal-read-error", "cotp-segmented-request"} {
		x := mmsISOCaseAnswer(t, id)
		s := mmsISONew(t, DefaultParserBudget())
		es := mmsISOFeed(t, s, x, 5)
		require.Len(t, es, 5)
		require.Positive(t, s.Stats().BufferedBytes)
		ended := s.Close("end")
		require.Len(t, ended, 1)
		require.Equal(t, "incomplete", ended[0].Completeness)
		require.Zero(t, ended[0].ResponseTo)
		require.Zero(t, ended[0].TransactionID)
		_, err := ended[0].GetFields()
		rocTypedError(t, "NeedMore", err)
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
	// The unlabelled ISO DT continuation cannot be guessed after a timeout.
	x := mmsISOCaseAnswer(t, "cotp-segmented-request")
	s := mmsISONew(t, DefaultParserBudget())
	mmsISOFeed(t, s, x, 5)
	w, _ := hex.DecodeString(x.Steps[5].Hex)
	r := s.Feed(0, time.Unix(36, 0), w)
	require.Len(t, r.Events, 1)
	_, err := r.Events[0].GetFields()
	assertOwnedProtocolRefusal(t, r.Events[0], "ContextRequired", err)
	require.Zero(t, r.Events[0].ResponseTo)
	require.Zero(t, r.Events[0].TransactionID)
	require.Zero(t, s.Stats().BufferedBytes)
	require.Empty(t, s.Close("end"))
	// A fresh observed transport is independent of the refused flow.
	normal := mmsISONew(t, DefaultParserBudget())
	events := mmsISOFeed(t, normal, a, len(a.Steps))
	require.Empty(t, normal.Close("end"))
	mmsISOCheck(t, events, a, false)
	require.Zero(t, normal.Stats().BufferedBytes)
}
func TestMMSISOProjectionBudgetAndSelection(t *testing.T) {
	a := mmsISOCaseAnswer(t, "normal-read-error")
	w, _ := hex.DecodeString(a.Steps[0].Hex)
	window := 256 + 32768 + 512*len(w)
	s := mmsISONew(t, ParserBudget{MaxMessageBytes: 4096, MaxFrameBytes: 4096, MaxBufferedBytes: window + 1000}).(*captureSession)
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
	require.Empty(t, s.Close("end"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(window+1000))
	for _, opts := range [][]ProtocolSessionOption{{WithSessionTransport("udp"), WithSessionMMSISO()}, {WithSessionMMSISO(), WithSessionTransport("udp")}} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
	c := NewDefaultConfig()
	require.Error(t, WithProtocolMMSISO(0)(c))
	var events []*ProtocolEvent
	raw, _ := mmsISOInput(t, mmsISOCases(t)[0])
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	for _, e := range events {
		require.NotEqual(t, mmsISOProfile, e.Profile)
	}
	// UDP routing never applies this TCP-only option.
	u, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 102))
	require.NoError(t, err)
	r = u.Feed(0, time.Unix(1, 0), w)
	for _, e := range r.Events {
		require.NotEqual(t, mmsISOProfile, e.Profile)
	}
	u.Close("end")
}
