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

func mmsServiceRead(t *testing.T, file string) []byte {
	t.Helper()
	var b []byte
	var err error
	if file != "manifest.json" && len(file) > 7 && file[:7] == "common/" {
		repo, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(repo, file))
	} else if root := os.Getenv("MMS_SERVICES_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, file))
	} else {
		b, err = trafficfixture.ReadFile("mms-discovery-write/" + file)
	}
	require.NoError(t, err)
	return b
}
func mmsServiceCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(mmsServiceRead(t, "manifest.json"), &v))
	require.Equal(t, 1, v.SchemaVersion)
	require.Len(t, v.Cases, 86)
	if os.Getenv("MMS_SERVICES_TEST_ROOT") == "" {
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
			f, ok := indexed["mms-discovery-write/"+c.ID]
			require.True(t, ok)
			require.Equal(t, "mms-discovery-write/"+c.Capture, f.Input.File)
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
func mmsServiceInput(t *testing.T, c mmsISOCase) ([]byte, mmsISOAnswer) {
	t.Helper()
	raw, ans := mmsServiceRead(t, c.Capture), mmsServiceRead(t, c.Answer)
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
func TestMMSDiscoveryWriteSealedSession(t *testing.T) {
	for _, c := range mmsServiceCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := mmsServiceInput(t, c)
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
func TestMMSDiscoveryWriteSealedReplay(t *testing.T) {
	for _, c := range mmsServiceCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := mmsServiceInput(t, c)
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

func mmsServiceAnswer(t *testing.T, id string) mmsISOAnswer {
	t.Helper()
	for _, c := range mmsServiceCases(t) {
		if c.ID == id {
			_, a := mmsServiceInput(t, c)
			return a
		}
	}
	t.Fatalf("missing selected service case %s", id)
	return mmsISOAnswer{}
}
func TestMMSDiscoveryWriteQueryOwnershipAndClose(t *testing.T) {
	for _, id := range []string{"pagination-observed", "write-mixed-results"} {
		t.Run(id, func(t *testing.T) {
			a := mmsServiceAnswer(t, id)
			s := mmsValuesNew(t, DefaultParserBudget())
			events := mmsISOFeed(t, s, a, 5)
			require.Len(t, events, 5)
			request := events[4]
			fields, err := request.GetFields()
			require.NoError(t, err)
			mutateRTPSQuery(fields)
			mutateRTPSQuery(request.Session)
			clear(request.Raw)
			owned, err := request.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, a.Events[4].Fields, owned)
			w, err := hex.DecodeString(a.Steps[5].Hex)
			require.NoError(t, err)
			reply := s.Feed(1, time.Unix(6, 0), w)
			require.Len(t, reply.Events, 1)
			got, err := reply.Events[0].GetFields()
			require.NoError(t, err)
			rocEqualFields(t, a.Events[5].Fields, got)
			require.EqualValues(t, 5, reply.Events[0].ResponseTo)
			require.EqualValues(t, 5, reply.Events[0].TransactionID)
			mutateRTPSQuery(got)
			mutateRTPSQuery(reply.Events[0].Session)
			owned, err = reply.Events[0].GetFields()
			require.NoError(t, err)
			rocEqualFields(t, a.Events[5].Fields, owned)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			// Pending query/name context diagnoses once and releases even after caller edits.
			s = mmsValuesNew(t, DefaultParserBudget())
			events = mmsISOFeed(t, s, a, 5)
			mutateRTPSQuery(events[4].Session)
			closed := s.Close("end")
			require.Len(t, closed, 1)
			_, err = closed[0].GetFields()
			rocTypedError(t, "NeedMore", err)
			require.Zero(t, closed[0].ResponseTo)
			require.Zero(t, closed[0].TransactionID)
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("again"))
		})
	}
}
func TestMMSDiscoveryWriteSegmentedResourceRefusal(t *testing.T) {
	for _, id := range []string{"query-large-cursor", "write-large-bytes"} {
		t.Run(id, func(t *testing.T) {
			a := mmsServiceAnswer(t, id)
			b := DefaultParserBudget()
			b.MaxBufferedBytes = 2 << 20
			s := mmsValuesNew(t, b)
			// The observed handshake and two partial TSDU contributors fit this budget;
			// the complete reconstructed output must reserve before parsing/projecting.
			events := mmsISOFeed(t, s, a, 6)
			require.Len(t, events, 6)
			for _, e := range events {
				_, err := e.GetFields()
				require.NoError(t, err)
			}
			require.Greater(t, s.Stats().BufferedBytes, int64(0))
			w, err := hex.DecodeString(a.Steps[6].Hex)
			require.NoError(t, err)
			before := s.Stats()
			s.Probe(w)
			require.Equal(t, before, s.Stats())
			result := s.Feed(0, time.Unix(7, 0), w)
			require.Len(t, result.Events, 1)
			bad := result.Events[0]
			_, err = bad.GetFields()
			assertOwnedProtocolRefusal(t, bad, "ResourceExceeded", err)
			require.Zero(t, bad.ResponseTo)
			require.Zero(t, bad.TransactionID)
			require.Nil(t, bad.Session)
			later, err := hex.DecodeString(a.Steps[7].Hex)
			require.NoError(t, err)
			late := s.Feed(1, time.Unix(8, 0), later)
			require.Empty(t, late.Events)
			rocTypedError(t, "FatalSessionError", late.Err)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
