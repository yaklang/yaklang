package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// JSON spelling is not a number's semantic value. Rational normalization keeps
// uint64/int64 and IEEE subnormal values exact rather than casting to float64;
// encoded bytes and IEEE bit strings remain separately compared in full.
func bacnetRPNumberTree(v any) any {
	switch x := v.(type) {
	case json.Number:
		q, ok := new(big.Rat).SetString(string(x))
		if ok {
			return q.RatString()
		}
		return x
	case map[string]any:
		if x["Type"] == "Real" || x["Type"] == "Double" {
			if q, ok := x["Value"].(json.Number); ok {
				f, err := q.Float64()
				if err == nil {
					x["Value"] = fmt.Sprintf("ieee64:%016x", math.Float64bits(f))
				}
			}
		}
		for k, q := range x {
			x[k] = bacnetRPNumberTree(q)
		}
		return x
	case []any:
		for i, q := range x {
			x[i] = bacnetRPNumberTree(q)
		}
		return x
	}
	return v
}
func bacnetRPEqual(t *testing.T, want, got any) {
	t.Helper()
	trees := []any{}
	for _, v := range []any{want, got} {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		var x any
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		require.NoError(t, d.Decode(&x))
		trees = append(trees, bacnetRPNumberTree(x))
	}
	require.Equal(t, trees[0], trees[1])
}

func bacnetRPRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if root := os.Getenv("BACNET_RP_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("bacnet-read-property/" + name)
	}
	require.NoError(t, err)
	return b
}

type bacnetRPAnswer struct {
	Events []struct {
		ID                 uint64
		Dir                int
		Raw, Status, Error string
		Fields, Session    map[string]any
		ErrorKind          ProtocolErrorKind `json:"error_kind"`
		ResponseTo         uint64            `json:"response_to"`
		TransactionID      uint64            `json:"transaction_id"`
	}
	Packets int
}

func bacnetRPCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct{ Cases []mmsISOCase }
	require.NoError(t, json.Unmarshal(bacnetRPRead(t, "manifest.json"), &m))
	require.Len(t, m.Cases, 41)
	if os.Getenv("BACNET_RP_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		frozen := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				frozen[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := frozen["bacnet-read-property/"+c.ID]
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
func bacnetRPInput(t *testing.T, c mmsISOCase) ([]byte, bacnetRPAnswer) {
	t.Helper()
	raw, a := bacnetRPRead(t, c.Capture), bacnetRPRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(a)))
	var answer bacnetRPAnswer
	d := json.NewDecoder(bytes.NewReader(a))
	d.UseNumber()
	require.NoError(t, d.Decode(&answer))
	require.Equal(t, c.Packets, answer.Packets)
	require.Len(t, answer.Events, c.Packets)
	return raw, answer
}
func bacnetRPCheck(t *testing.T, a bacnetRPAnswer, events []*ProtocolEvent, deferred, capture bool) {
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
		require.Equal(t, bacnetReadPropertyProfile, e.Profile)
		require.Equal(t, "explicit-decode-as", e.Admission)
		dir := w.Dir
		if capture {
			dir = w.Dir ^ a.Events[0].Dir
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
func TestBACnetReadPropertySealedReplay(t *testing.T) {
	for _, c := range bacnetRPCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := bacnetRPInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%v-o%v", worker, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithProtocolDecodeAs("udp", 47808, "bacnet-read-property"), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							bacnetRPCheck(t, a, events, deferred, true)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
							require.Equal(t, uint64(c.Packets), stats.Messages)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.Unknown)
						})
					}
				}
			}
		})
	}
}
func TestBACnetReadPropertySealedSession(t *testing.T) {
	for _, c := range bacnetRPCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := bacnetRPInput(t, c)
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("deferred=%v", deferred), func(t *testing.T) {
					s := bacnetRPSession(t, deferred)
					var events []*ProtocolEvent
					for i, st := range a.Events {
						raw, err := hex.DecodeString(st.Raw)
						require.NoError(t, err)
						out := s.Feed(st.Dir, time.Unix(1700000000, int64(i+1)*1234), raw)
						require.Len(t, out.Events, 1)
						events = append(events, out.Events...)
						clear(raw)
					}
					s.Close("capture complete")
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
					bacnetRPCheck(t, a, events, deferred, false)
				})
			}
		})
	}
}
