package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func bacnetRoutesRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if root := os.Getenv("BACNET_ROUTES_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("bacnet-routed-read-property/" + name)
	}
	require.NoError(t, err)
	return b
}
func bacnetRoutesCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct{ Cases []mmsISOCase }
	require.NoError(t, json.Unmarshal(bacnetRoutesRead(t, "manifest.json"), &m))
	require.Len(t, m.Cases, 19)
	if os.Getenv("BACNET_ROUTES_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		frozen := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				frozen[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := frozen["bacnet-routed-read-property/"+c.ID]
			require.True(t, ok)
			require.Equal(t, c.SHA256, f.Input.SHA256)
			require.Equal(t, c.Packets, f.Facts.PacketCount)
			require.Len(t, f.Expectations, 1)
			var binding struct {
				Answer string `json:"answer_file"`
				SHA    string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(f.Expectations[0].PayloadConstraints, &binding))
			require.Equal(t, c.Answer, binding.Answer)
			require.Equal(t, c.AnswerSHA, binding.SHA)
		}
	}
	return m.Cases
}

type bacnetRoutesAnswer struct {
	Events []struct {
		ID                 uint64
		Dir                int
		CaptureDir         int `json:"capture_dir"`
		Raw, Status, Error string
		Fields, Session    map[string]any
		ErrorKind          ProtocolErrorKind `json:"error_kind"`
		ResponseTo         uint64            `json:"response_to"`
		TransactionID      uint64            `json:"transaction_id"`
	}
	Packets int
}

func bacnetRoutesInput(t *testing.T, c mmsISOCase) ([]byte, bacnetRoutesAnswer) {
	t.Helper()
	raw, ans := bacnetRoutesRead(t, c.Capture), bacnetRoutesRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ans)))
	var a bacnetRoutesAnswer
	d := json.NewDecoder(bytes.NewReader(ans))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	return raw, a
}
func bacnetRouteCheck(t *testing.T, a bacnetRoutesAnswer, events []*ProtocolEvent, deferred, capture bool) {
	t.Helper()
	require.Len(t, events, a.Packets)
	for i, e := range events {
		w := a.Events[i]
		raw, err := hex.DecodeString(w.Raw)
		require.NoError(t, err)
		require.Equal(t, w.ID, e.ID)
		require.Equal(t, w.ResponseTo, e.ResponseTo)
		require.Equal(t, w.TransactionID, e.TransactionID)
		require.Equal(t, raw, e.Raw)
		require.Equal(t, len(raw), e.Length)
		require.Equal(t, "udp", e.Transport)
		require.Equal(t, "bacnet", e.Protocol)
		require.Equal(t, bacnetRoutedReadProfile, e.Profile)
		require.Equal(t, "explicit-decode-as", e.Admission)
		dir := w.Dir
		if capture {
			dir = w.CaptureDir
		}
		require.Equal(t, dir, e.Direction)
		src, dst := "192.0.2.1:40000", "192.0.2.2:47808"
		if w.Dir == 1 {
			src, dst = dst, src
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		if capture {
			require.Equal(t, "captured", e.SourceBytes.Kind)
			require.Len(t, e.SourceBytes.PacketRefs, 1)
		}
		status := w.Status
		if deferred && w.ErrorKind == "" {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		require.Equal(t, w.Error, e.Error)
		if w.ErrorKind != "" {
			bacnetRPReject(t, e, w.ErrorKind)
			continue
		}
		require.Equal(t, "message", e.Completeness)
		require.Empty(t, e.Error)
		require.Empty(t, e.ExpertCode)
		fields, err := e.GetFields()
		require.NoError(t, err)
		bacnetRPEqual(t, w.Fields, fields)
		bacnetRPEqual(t, w.Session, e.Session)
		result, err := e.Decode()
		require.NoError(t, err)
		bacnetRPEqual(t, map[string]any{"fields": w.Fields, "session": w.Session}, result)
		fields["APDU"].([]byte)[0] = 255
		fields["Object Type"] = 999
		e.Fields = map[string]any{"injected": true}
		e.Structured = map[string]any{"fields": e.Fields}
		e.Session = e.Fields
		e.Protocol = "http"
		e.Status = "decoded"
		clear(e.Raw)
		result, err = e.Decode()
		require.NoError(t, err)
		bacnetRPEqual(t, map[string]any{"fields": w.Fields, "session": w.Session}, result)
	}
}

func TestBACnetRoutesSealedReplay(t *testing.T) {
	for _, c := range bacnetRoutesCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := bacnetRoutesInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%v-o%v", worker, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithProtocolDecodeAs("udp", 47808, "bacnet-routed-read-property"), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							bacnetRouteCheck(t, a, events, deferred, true)
							require.Equal(t, uint64(c.Packets), stats.Messages)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.Unknown)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
						})
					}
				}
			}
		})
	}
}
func TestBACnetRoutesSealedSession(t *testing.T) {
	for _, c := range bacnetRoutesCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := bacnetRoutesInput(t, c)
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("deferred=%v", deferred), func(t *testing.T) {
					s := bacnetRoutedSession(t)
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for i, st := range a.Events {
						w, err := hex.DecodeString(st.Raw)
						require.NoError(t, err)
						before := s.Stats()
						p := s.Probe(w)
						want := ProbeAccept
						if st.ErrorKind == ErrMalformedMessage || st.ErrorKind == ErrUnsupportedFeature {
							want = ProbeReject
						}
						require.Equal(t, want, p.Verdict)
						require.Equal(t, before, s.Stats())
						out := s.Feed(st.Dir, time.Unix(1700000000, int64(i+1)*10000), w)
						require.Len(t, out.Events, 1)
						events = append(events, out.Events...)
						clear(w)
					}
					s.Close("done")
					require.Zero(t, s.Stats().BufferedBytes)
					require.Empty(t, s.Close("again"))
					bacnetRouteCheck(t, a, events, deferred, false)
				})
			}
		})
	}
}
