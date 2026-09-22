package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

type kafkaFlexOracle struct {
	API            int16
	Version        int16
	Correlation    int32
	Request        string         `json:"request_hex"`
	Response       *string        `json:"response_hex"`
	ResponseHeader int16          `json:"response_header_version"`
	Expected       map[string]any `json:"response"`
}

func kafkaFlexOracles(t testing.TB) []kafkaFlexOracle {
	t.Helper()
	b, e := os.ReadFile("testdata/protocol-sessions/kafka-flex/java-oracle.json")
	require.NoError(t, e)
	var rows []kafkaFlexOracle
	require.NoError(t, json.Unmarshal(b, &rows))
	require.Len(t, rows, 9)
	return rows
}
func kafkaFlexWire(t testing.TB, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	require.NoError(t, e)
	return b
}
func TestKafkaFlexibleJavaOracle(t *testing.T) {
	for i, row := range kafkaFlexOracles(t) {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if row.Response == nil {
				return
			}
			w := kafkaFlexWire(t, *row.Response)
			got, e := stream_parser.KafkaResponsePayloadVersion(row.API, row.Version, w[8:])
			require.NoError(t, e)
			require.Equal(t, row.ResponseHeader, got["Header Version"])
			require.EqualValues(t, row.Expected["throttleTimeMs"], got["Throttle Time"])
			switch row.API {
			case 18:
				entries := got["API Versions"].([]map[string]any)
				want := row.Expected["apiKeys"].([]any)
				require.Len(t, entries, len(want))
				for j, v := range entries {
					x := want[j].(map[string]any)
					for a, b := range map[string]string{"API Key": "apiKey", "Min Version": "minVersion", "Max Version": "maxVersion"} {
						require.EqualValues(t, x[b], v[a])
					}
				}
				require.EqualValues(t, row.Expected["finalizedFeaturesEpoch"], got["Finalized Features Epoch"])
				require.NotEmpty(t, got["Supported Features"])
				require.NotEmpty(t, got["Finalized Features"])
			case 3:
				require.Equal(t, row.Expected["clusterId"], got["Cluster ID"])
				require.EqualValues(t, row.Expected["controllerId"], got["Controller ID"])
				brokers := got["Brokers"].([]map[string]any)
				want := row.Expected["brokers"].([]any)
				require.Len(t, brokers, len(want))
				for j, b := range brokers {
					x := want[j].(map[string]any)
					require.Equal(t, x["host"], b["Host"])
					require.EqualValues(t, x["port"], b["Port"])
					require.Equal(t, x["rack"], b["Rack"])
				}
				topics := got["Topic Results"].([]map[string]any)
				expected := row.Expected["topics"].([]any)
				require.Len(t, topics, len(expected))
				for j, v := range topics {
					x := expected[j].(map[string]any)
					require.Equal(t, x["name"], v["Topic Name"])
					ps := v["Partitions"].([]map[string]any)
					xs := x["partitions"].([]any)
					require.Len(t, ps, len(xs))
					for k, p := range ps {
						xp := xs[k].(map[string]any)
						for a, b := range map[string]string{"Partition": "partitionIndex", "Leader": "leaderId", "Leader Epoch": "leaderEpoch", "Error Code": "errorCode"} {
							require.EqualValues(t, xp[b], p[a])
						}
					}
				}
			case 0, 1:
				topics := got["Topic Results"].([]map[string]any)
				require.Len(t, topics, 1)
				require.Equal(t, "zip-flex", topics[0]["Topic Name"])
				parts := topics[0]["Partitions"].([]map[string]any)
				require.Len(t, parts, 2)
				for j, p := range parts {
					require.Equal(t, int32(j), p["Partition"])
					require.Equal(t, int16(0), p["Error Code"])
					if row.API == 0 {
						require.Equal(t, int64(i-4), p["Base Offset"])
						continue
					}
					require.Equal(t, int64(2), p["High Watermark"])
					bs := p["Batches"].([]map[string]any)
					require.Len(t, bs, 2)
					for k, b := range bs {
						require.Equal(t, true, b["CRC Valid"])
						require.Equal(t, int16(k), b["Compression"])
						rs := b["Records"].([]map[string]any)
						require.Len(t, rs, 1)
						require.Equal(t, []byte(fmt.Sprintf("zip-flex-p%d-gzip%v", j, k == 1)), rs[0]["Value"])
					}
				}
			}
		})
	}
}
func TestKafkaFlexibleNativeReplay(t *testing.T) {
	wire, e := os.ReadFile("testdata/protocol-sessions/kafka-flex/kafka-flex-native.pcap")
	require.NoError(t, e)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d-%v", workers, deferred), func(t *testing.T) {
				var events []*ProtocolEvent
				require.NoError(t, ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(workers), WithBinParserDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
				require.Len(t, events, 17)
				expected := map[string]int{}
				for _, row := range kafkaFlexOracles(t) {
					expected[row.Request]++
					if row.Response != nil {
						expected[*row.Response]++
					}
				}

				matches, acks0 := 0, 0
				for _, ev := range events {
					require.Equal(t, "kafka", ev.Protocol)
					expected[hex.EncodeToString(ev.Raw)]--
					require.Contains(t, []string{"decoded", "deferred"}, ev.Status, "%s", ev.Error)
					_, e := ev.GetFields()
					require.NoError(t, e)
					require.Equal(t, true, ev.Session["Flexible"])
					if ev.ResponseTo != 0 {
						matches++
					}
					if ev.Session["Response Expected"] == false {
						acks0++
					}
				}
				for wire, count := range expected {
					require.Zero(t, count, "wire %s", wire)
				}
				require.Equal(t, 8, matches)
				require.Equal(t, 1, acks0)
			})
		}
	}
}
func TestKafkaFlexibleFragmentation(t *testing.T) {
	rows := kafkaFlexOracles(t)
	for _, chunk := range []int{1, 7, 0} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			s := newReviewSession(t, ParserBudget{})
			ts := time.Unix(1, 0)
			count := 0
			feed := func(dir int, w []byte) {
				step := chunk
				if step == 0 {
					step = len(w)
				}
				for n := 0; n < len(w); n += step {
					end := n + step
					if end > len(w) {
						end = len(w)
					}
					r := s.Feed(dir, ts, w[n:end])
					if r.Err != nil {
						require.Equal(t, ErrNeedMore, r.Err.Kind, "%v", r.Err)
					}
					for _, ev := range r.Events {
						require.Equal(t, "decoded", ev.Status, "%s", ev.Error)
						count++
					}
				}
			}
			for _, row := range rows {
				feed(0, kafkaFlexWire(t, row.Request))
				if row.Response != nil {
					feed(1, kafkaFlexWire(t, *row.Response))
				}
			}
			require.Equal(t, 17, count)
		})
	}
}

func TestKafkaFlexibleCorrelation(t *testing.T) {
	rows := kafkaFlexOracles(t)
	ts := time.Unix(1, 0)
	t.Run("legacy-flex-reordered", func(t *testing.T) {
		s := newReviewSession(t, ParserBudget{})
		legacy := s.Feed(0, ts, kafkaRequest(18, 0, 500, "legacy", nil))
		require.Nil(t, legacy.Err)
		flex := s.Feed(0, ts, kafkaFlexWire(t, rows[0].Request))
		require.Nil(t, flex.Err)
		rsp := s.Feed(1, ts, kafkaFlexWire(t, *rows[0].Response))
		require.Nil(t, rsp.Err)
		require.Equal(t, flex.Events[0].ID, rsp.Events[0].ResponseTo)
		require.Equal(t, int16(0), rsp.Events[0].Session["Header Version"])
		rsp = s.Feed(1, ts, kafkaResponse(500, []byte{0, 0, 0, 0, 0, 0}))
		require.Nil(t, rsp.Err)
		require.Equal(t, legacy.Events[0].ID, rsp.Events[0].ResponseTo)
		s.Close("test")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("pending-budget", func(t *testing.T) {
		s := newReviewSession(t, ParserBudget{MaxCollectionElements: 1})
		require.Nil(t, s.Feed(0, ts, kafkaFlexWire(t, rows[0].Request)).Err)
		r := s.Feed(0, ts, kafkaFlexWire(t, rows[1].Request))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		s.Close("test")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("response-header-unknown-tags", func(t *testing.T) {
		w := kafkaFlexWire(t, *rows[1].Response)
		payload := append([]byte{1, 72, 2, 3, 4}, w[9:]...)
		m, e := stream_parser.KafkaResponsePayloadVersion(3, 9, payload)
		require.NoError(t, e)
		tags := m["Response Header"].(map[string]any)["Tagged Fields"].([]map[string]any)
		require.Equal(t, uint32(72), tags[0]["Tag"])
		require.Equal(t, []byte{3, 4}, tags[0]["Data"])
	})
	for _, expiry := range []bool{false, true} {
		t.Run(fmt.Sprintf("reuse-expired-%v", expiry), func(t *testing.T) {
			s := newReviewSession(t, ParserBudget{})
			w := kafkaFlexWire(t, rows[0].Request)
			require.Nil(t, s.Feed(0, ts, w).Err)
			if expiry {
				ts = ts.Add(31 * time.Second)
			}
			r := s.Feed(0, ts, w)
			require.NotNil(t, r.Err)
			require.Equal(t, ErrContextRequired, r.Err.Kind)
			s.Close("test")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func BenchmarkKafkaNativeReplay(b *testing.B) {
	for _, fixture := range []string{"first-batch-m2a/m2-native.pcap", "kafka-flex/kafka-flex-native.pcap"} {
		wire, e := os.ReadFile("testdata/protocol-sessions/" + fixture)
		if e != nil {
			b.Fatal(e)
		}
		for _, deferred := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/deferred=%v", fixture, deferred), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(wire)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var bad atomic.Bool
					var kafkaCount atomic.Int64
					if e := ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(1), WithBinParserDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) {
						if e.Protocol == "kafka" {
							kafkaCount.Add(1)
						}
						if e.Status != "decoded" && e.Status != "deferred" {
							bad.Store(true)
						}
					})); e != nil {
						b.Fatal(e)
					}
					want := int64(63)
					if fixture == "kafka-flex/kafka-flex-native.pcap" {
						want = 17
					}
					if bad.Load() || kafkaCount.Load() != want {
						b.Fatalf("invalid replay: kafka=%d want=%d bad=%v", kafkaCount.Load(), want, bad.Load())
					}
				}
			})
		}
	}
}

func TestKafkaFlexibleManifest(t *testing.T) {
	raw, e := os.ReadFile("testdata/protocol-sessions/kafka-flex/manifest.json")
	require.NoError(t, e)
	var manifest struct {
		Files map[string]struct {
			SHA256 string
			Bytes  int
		}
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Len(t, manifest.Files, 3)
	for name, entry := range manifest.Files {
		raw, e := os.ReadFile("testdata/protocol-sessions/kafka-flex/" + name)
		require.NoError(t, e)
		sum := sha256.Sum256(raw)
		require.Equal(t, entry.SHA256, hex.EncodeToString(sum[:]))
		require.Len(t, raw, entry.Bytes)
	}
}
