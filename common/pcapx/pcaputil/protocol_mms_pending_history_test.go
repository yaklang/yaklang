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

type mmsPendingHistoryCase struct {
	mmsISOCase
	CollectionBudget        int    `json:"collection_budget"`
	DescriptionValidation   bool   `json:"description_validation"`
	CaptureAnswer           string `json:"capture_answer"`
	CaptureAnswerSHA        string `json:"capture_answer_sha256"`
	CaptureCollectionBudget int    `json:"capture_collection_budget"`
}

func mmsPendingHistoryProfile(c mmsPendingHistoryCase) string {
	if c.DescriptionValidation {
		return "mms-iso-described-data"
	}
	return mmsServicesProfile
}
func mmsPendingHistoryNew(t *testing.T, c mmsPendingHistoryCase) ProtocolSession {
	b := DefaultParserBudget()
	b.MaxCollectionElements = c.CollectionBudget
	if c.DescriptionValidation {
		return mmsDescriptionsNew(t, b)
	}
	return mmsValuesNew(t, b)
}
func mmsPendingHistoryReadFile(t *testing.T, file string) []byte {
	t.Helper()
	var b []byte
	var err error
	if file != "manifest.json" && len(file) > 7 && file[:7] == "common/" {
		repo, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(repo, file))
	} else if root := os.Getenv("MMS_PENDING_HISTORY_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, file))
	} else {
		b, err = trafficfixture.ReadFile("mms-pending-history/" + file)
	}
	require.NoError(t, err)
	return b
}
func mmsPendingHistoryCases(t *testing.T) []mmsPendingHistoryCase {
	t.Helper()
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsPendingHistoryCase
	}
	require.NoError(t, json.Unmarshal(mmsPendingHistoryReadFile(t, "manifest.json"), &v))
	require.Equal(t, 1, v.SchemaVersion)
	require.Len(t, v.Cases, 12)
	if os.Getenv("MMS_PENDING_HISTORY_TEST_ROOT") == "" {
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
			f, ok := indexed["mms-pending-history/"+c.ID]
			require.True(t, ok)
			require.Equal(t, "mms-pending-history/"+c.Capture, f.Input.File)
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
func mmsPendingHistoryInput(t *testing.T, c mmsPendingHistoryCase) ([]byte, mmsISOAnswer) {
	t.Helper()
	raw, ans := mmsPendingHistoryReadFile(t, c.Capture), mmsPendingHistoryReadFile(t, c.Answer)
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
func TestMMSPendingHistorySealedSession(t *testing.T) {
	for _, c := range mmsPendingHistoryCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsPendingHistoryInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s := mmsPendingHistoryNew(t, c)
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
					if len(events) != len(a.Events) {
						for _, e := range events {
							_, err := e.GetFields()
							t.Logf("event=%d error=%#v", e.ID, err)
						}
					}
					mmsValuesCheckProfile(t, events, a, false, mmsPendingHistoryProfile(c))
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
func TestMMSPendingHistorySealedReplay(t *testing.T) {
	for _, c := range mmsPendingHistoryCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			require.Equal(t, DefaultParserBudget().MaxCollectionElements, c.CaptureCollectionBudget)
			captureCase := c
			captureCase.Answer, captureCase.AnswerSHA = c.CaptureAnswer, c.CaptureAnswerSHA
			raw, a := mmsPendingHistoryInput(t, captureCase)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", worker, deferred, observer), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							selectProfile := WithProtocolMMSServiceValues(102)
							if c.DescriptionValidation {
								selectProfile = WithProtocolMMSDescriptionValidation(102)
							}
							opts := []CaptureOption{selectProfile, WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
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
							mmsValuesCheckProfile(t, events, a, true, mmsPendingHistoryProfile(c))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
						})
					}
				}
			}
		})
	}
}
