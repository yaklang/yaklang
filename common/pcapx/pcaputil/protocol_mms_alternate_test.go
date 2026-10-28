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

type mmsAlternateCase struct {
	mmsISOCase
	DescriptionValidation bool `json:"description_validation"`
}

func mmsAlternateProfile(c mmsAlternateCase) string {
	if c.DescriptionValidation {
		return "mms-iso-described-data"
	}
	return mmsServicesProfile
}
func mmsAlternateNew(t *testing.T, c mmsAlternateCase, b ParserBudget) ProtocolSession {
	if c.DescriptionValidation {
		return mmsDescriptionsNew(t, b)
	}
	return mmsValuesNew(t, b)
}
func mmsAlternateOption(c mmsAlternateCase) CaptureOption {
	if c.DescriptionValidation {
		return WithProtocolMMSDescriptionValidation(102)
	}
	return WithProtocolMMSServiceValues(102)
}
func mmsAlternateReadFile(t *testing.T, file string) []byte {
	t.Helper()
	var b []byte
	var err error
	if file != "manifest.json" && len(file) > 7 && file[:7] == "common/" {
		repo, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(repo, file))
	} else if root := os.Getenv("MMS_ALTERNATE_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, file))
	} else {
		b, err = trafficfixture.ReadFile("mms-alternate-access/" + file)
	}
	require.NoError(t, err)
	return b
}
func mmsAlternateCases(t *testing.T) []mmsAlternateCase {
	t.Helper()
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsAlternateCase
	}
	require.NoError(t, json.Unmarshal(mmsAlternateReadFile(t, "manifest.json"), &v))
	require.Equal(t, 1, v.SchemaVersion)
	require.Len(t, v.Cases, 48)
	if os.Getenv("MMS_ALTERNATE_TEST_ROOT") == "" {
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
			f, ok := indexed["mms-alternate-access/"+c.ID]
			require.True(t, ok)
			require.Equal(t, "mms-alternate-access/"+c.Capture, f.Input.File)
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
func mmsAlternateInput(t *testing.T, c mmsAlternateCase) ([]byte, mmsISOAnswer) {
	t.Helper()
	raw, ans := mmsAlternateReadFile(t, c.Capture), mmsAlternateReadFile(t, c.Answer)
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
func TestMMSAlternateSealedSession(t *testing.T) {
	for _, c := range mmsAlternateCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsAlternateInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s := mmsAlternateNew(t, c, DefaultParserBudget())
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
					mmsValuesCheckProfile(t, events, a, false, mmsAlternateProfile(c))
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
func TestMMSAlternateSealedReplay(t *testing.T) {
	for _, c := range mmsAlternateCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := mmsAlternateInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", worker, deferred, observer), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							opts := []CaptureOption{mmsAlternateOption(c), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
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
							mmsValuesCheckProfile(t, events, a, true, mmsAlternateProfile(c))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
						})
					}
				}
			}
		})
	}
}

func mmsAlternateAnswer(t *testing.T, id string) (mmsAlternateCase, mmsISOAnswer) {
	t.Helper()
	for _, c := range mmsAlternateCases(t) {
		if c.ID == id {
			_, a := mmsAlternateInput(t, c)
			return c, a
		}
	}
	t.Fatalf("missing alternate case %s", id)
	return mmsAlternateCase{}, mmsISOAnswer{}
}
func TestMMSAlternatePrivateOwnership(t *testing.T) {
	for _, id := range []string{"defined-index", "lookup-component", "interleaved-replies", "opposite-direction-same-id"} {
		t.Run(id, func(t *testing.T) {
			c, a := mmsAlternateAnswer(t, id)
			s := mmsAlternateNew(t, c, DefaultParserBudget())
			var events []*ProtocolEvent
			for i, step := range a.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				clear(raw)
				for _, e := range out.Events {
					fields, err := e.GetFields()
					require.NoError(t, err)
					snapshot := cloneSession(fields)
					originalSession := cloneSession(e.Session)
					originalRaw := bytes.Clone(e.Raw)
					mutateRTPSQuery(fields)
					mutateRTPSQuery(e.Session)
					clear(e.Raw)
					e.Raw = originalRaw
					e.Session = originalSession
					owned, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, snapshot, owned)
					events = append(events, e)
				}
			}
			events = append(events, s.Close("end")...)
			mmsValuesCheckProfile(t, events, a, false, mmsAlternateProfile(c))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("again"))
		})
	}
}
func TestMMSAlternatePendingCloseAndBudget(t *testing.T) {
	c, a := mmsAlternateAnswer(t, "alternate-index")
	s := mmsAlternateNew(t, c, DefaultParserBudget())
	events := mmsISOFeed(t, s, a, 5)
	require.Len(t, events, 5)
	partial := a
	partial.Events = partial.Events[:5]
	mmsValuesCheck(t, events, partial, false)
	closed := s.Close("end")
	require.Len(t, closed, 1)
	e := closed[0]
	_, err := e.GetFields()
	assertOwnedProtocolRefusal(t, e, "NeedMore", err)
	require.EqualValues(t, 6, e.ID)
	require.Equal(t, 0, e.Direction)
	require.Empty(t, e.Raw)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	require.Zero(t, s.Stats().BufferedBytes)
	require.Empty(t, s.Close("again"))
	out := s.Feed(1, time.Unix(50, 0), []byte{3})
	require.Empty(t, out.Events)
	rocTypedError(t, "FatalSessionError", out.Err)
	// The same owned selector also consumes the shared derived-output budget.
	s2 := mmsAlternateNew(t, c, DefaultParserBudget()).(*captureSession)
	seed := mmsISOFeed(t, s2, a, 4)
	require.Len(t, seed, 4)
	raw, err := hex.DecodeString(a.Steps[4].Hex)
	require.NoError(t, err)
	free := int64(s2.f.a.config.MaxBufferedBytes) - s2.Stats().BufferedBytes - 1
	require.Positive(t, free)
	require.True(t, s2.f.a.reserveEvidence(free))
	before := s2.Stats()
	s2.Probe(raw)
	require.Equal(t, before, s2.Stats())
	result := s2.Feed(0, time.Unix(5, 0), raw)
	require.Len(t, result.Events, 1)
	e = result.Events[0]
	_, err = e.GetFields()
	assertOwnedProtocolRefusal(t, e, "ResourceExceeded", err)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	s2.f.a.buffered.Add(-free)
	if s2.f.mms != nil {
		require.Empty(t, s2.f.mms.pending[0])
		require.Empty(t, s2.f.mms.pending[1])
	}
	s2.Close("end")
	require.Zero(t, s2.Stats().BufferedBytes)
	require.Empty(t, s2.Close("again"))
}
