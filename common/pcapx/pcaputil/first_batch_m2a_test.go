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
	"path/filepath"
	"testing"
	"time"
)

func TestFirstBatchT15(t *testing.T) {
	t.Run("native-api-version-oracles", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/protocol-sessions/first-batch-m2a/native-oracle.json")
		require.NoError(t, err)
		var rows []struct {
			Protocol string
			API      int16
			Version  int16
			Request  string  `json:"request_hex"`
			Response *string `json:"response_hex"`
		}
		require.NoError(t, json.Unmarshal(raw, &rows))
		count := 0
		for _, r := range rows {
			if r.Protocol != "kafka" || r.Response == nil {
				continue
			}
			wire, err := hex.DecodeString(*r.Response)
			require.NoError(t, err)
			info, err := stream_parser.KafkaResponseBodyVersion(r.API, r.Version, wire[8:])
			require.NoError(t, err, "api=%d v=%d", r.API, r.Version)
			require.Equal(t, r.Version, info["API Version"])
			if r.API == 1 {
				parts := info["Topic Results"].([]map[string]any)[0]["Partitions"].([]map[string]any)
				b := parts[0]["Batches"].([]map[string]any)
				require.Len(t, b, 10)
				for i, batch := range b {
					records := batch["Records"].([]map[string]any)
					require.Equal(t, []byte(fmt.Sprintf("m2-v%d-gzip%d", 3+i/2, i%2)), records[0]["Value"])
					require.Equal(t, int64(i), batch["Base Offset"])
					require.Equal(t, true, batch["CRC Valid"])
					require.Equal(t, int32(1), batch["Records Count"])
				}
			}
			count++
		}
		require.Equal(t, 31, count)
	})
	t.Run("response-version-pipeline", func(t *testing.T) {
		s := newReviewSession(t, ParserBudget{})
		ts := time.Unix(1, 0)
		require.Nil(t, s.Feed(0, ts, kafkaRequest(18, 0, 1, "x", nil)).Err)
		require.Nil(t, s.Feed(0, ts, kafkaRequest(18, 2, 2, "x", nil)).Err)
		r := s.Feed(1, ts, kafkaResponse(2, append(append(kafkaBE16(0), kafkaBE32(0)...), kafkaBE32(12)...)))
		require.Nil(t, r.Err)
		require.Equal(t, int16(2), r.Events[0].Session["API Version"])
		require.Equal(t, int32(12), r.Events[0].Session["Throttle Time"])
		require.NotZero(t, r.Events[0].ResponseTo)
		require.Nil(t, s.Feed(1, ts, kafkaResponse(1, append(kafkaBE16(0), kafkaBE32(0)...))).Err)
	})
	t.Run("crc-original-negative", func(t *testing.T) {
		w := kafkaRecordBatch(0, false)
		clear(w[17:21])
		_, err := stream_parser.ParseKafkaRecordSet(w)
		require.ErrorContains(t, err, "CRC")
		w = kafkaRecordBatch(0, false)
		_, err = stream_parser.ParseKafkaRecordSet(append(w, 1))
		require.Error(t, err)
	})
}
func TestFirstBatchM2ANativeReplay(t *testing.T) {
	wire, err := os.ReadFile("testdata/protocol-sessions/first-batch-m2a/m2-native.pcap")
	require.NoError(t, err)
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers%d-deferred%v", workers, deferred), func(t *testing.T) {
				var events []*ProtocolEvent
				err := ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(workers), WithBinParserDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
				require.NoError(t, err)
				counts := map[string]int{}
				matches := map[string]int{}
				pushes := 0
				for _, e := range events {
					if e.Protocol != "kafka" && e.Protocol != "redis" {
						continue
					}
					require.Contains(t, []string{"decoded", "deferred"}, e.Status, "%s: %s", e.Protocol, e.Error)
					counts[e.Protocol]++
					if e.ResponseTo != 0 {
						matches[e.Protocol]++
					}
					if e.Session["Push"] == true {
						pushes++
					}
					fields, err := e.GetFields()
					require.NoError(t, err)
					require.NotNil(t, fields)
				}
				require.Equal(t, 63, counts["kafka"])
				require.Equal(t, 31, matches["kafka"])
				require.GreaterOrEqual(t, counts["redis"], 34)
				require.GreaterOrEqual(t, pushes, 4)
			})
		}
	}
}
func respCommand(args ...string) []byte {
	b := []byte(fmt.Sprintf("*%d\r\n", len(args)))
	for _, a := range args {
		b = append(b, []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(a), a))...)
	}
	return b
}
func TestFirstBatchT14M2(t *testing.T) {
	t.Run("types-all-cuts", func(t *testing.T) {
		for _, wire := range []string{"|1\r\n+ttl\r\n:12\r\n", "%1\r\n+key\r\n~2\r\n#t\r\n_\r\n", "=7\r\ntxt:hey\r\n", "!3\r\nbad\r\n", ",inf\r\n", "(+123456789012345678901234567890\r\n"} {
			_, err := DecodeRESPValue([]byte(wire), ParserBudget{})
			require.NoError(t, err)
			for i := 1; i < len(wire); i++ {
				n, err := redisFrameLength([]byte(wire[:i]), 0)
				require.NoError(t, err)
				require.Zero(t, n)
			}
		}
	})
	t.Run("attributes-push-pipeline", func(t *testing.T) {
		s := newReviewSession(t, ParserBudget{})
		ts := time.Unix(1, 0)
		r := s.Feed(0, ts, append(respCommand("GET", "one"), respCommand("GET", "two")...))
		require.Nil(t, r.Err)
		one, two := r.Events[0].ID, r.Events[1].ID
		r = s.Feed(1, ts, []byte("|1\r\n+ttl\r\n:12\r\n$1\r\na\r\n>2\r\n+invalidate\r\n_\r\n$1\r\nb\r\n"))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 4)
		require.Equal(t, one, r.Events[1].ResponseTo)
		require.NotNil(t, r.Events[1].Session["Attributes"])
		require.Zero(t, r.Events[2].ResponseTo)
		require.Equal(t, two, r.Events[3].ResponseTo)
	})
	t.Run("auth-display-redacted", func(t *testing.T) {
		s := newReviewSession(t, ParserBudget{})
		r := s.Feed(0, time.Time{}, respCommand("AUTH", "private-test-secret"))
		require.Nil(t, r.Err)
		display, err := json.Marshal(r.Events[0].Session)
		require.NoError(t, err)
		require.NotContains(t, string(display), "private-test-secret")
		require.Contains(t, string(r.Events[0].Raw), "private-test-secret")
	})
	t.Run("limits-expiry", func(t *testing.T) {
		s := newReviewSession(t, ParserBudget{})
		ts := time.Unix(1, 0)
		require.Nil(t, s.Feed(0, ts, respCommand("PING")).Err)
		r := s.Feed(1, ts.Add(31*time.Second), []byte("+PONG\r\n"))
		require.Nil(t, r.Err)
		require.Zero(t, r.Events[0].ResponseTo)
		_, err := DecodeRESPValue([]byte("*2\r\n:1\r\n:2\r\n"), ParserBudget{MaxCollectionElements: 1})
		require.Error(t, err)
	})
}

func TestFirstBatchM2ARedisResponseArrays(t *testing.T) {
	s := newReviewSession(t, ParserBudget{})
	ts := time.Unix(1, 0)
	request := s.Feed(0, ts, respCommand("GET", "array"))
	require.Nil(t, request.Err)
	r := s.Feed(1, ts, respCommand("message", "channel", "value"))
	require.Nil(t, r.Err)
	require.Equal(t, request.Events[0].ID, r.Events[0].ResponseTo)
	require.NotEqual(t, true, r.Events[0].Session["Push"])
	// A timeout loses positional correlation, so late responses must never attach
	// to a later request on this same connection.
	require.Nil(t, s.Feed(0, ts, respCommand("GET", "old")).Err)
	require.Nil(t, s.Feed(0, ts.Add(time.Minute), respCommand("GET", "new")).Err)
	r = s.Feed(1, ts.Add(time.Minute), []byte("+old\r\n"))
	require.Nil(t, r.Err)
	require.Zero(t, r.Events[0].ResponseTo)
}
func TestFirstBatchM2AKafkaBounds(t *testing.T) {
	// Aggregate arrays retain every topic/partition rather than the last scalar.
	body := kafkaBE32(2)
	for _, topic := range []string{"one", "two"} {
		body = append(body, kafkaStr(topic)...)
		body = append(body, kafkaBE32(2)...)
		for p := int32(0); p < 2; p++ {
			body = append(body, kafkaBE32(p)...)
			body = append(body, kafkaBE16(0)...)
			body = append(body, kafkaBE64(int64(p+10))...)
		}
	}
	m, err := stream_parser.KafkaResponseBodyVersion(0, 0, body)
	require.NoError(t, err)
	topics := m["Topic Results"].([]map[string]any)
	require.Len(t, topics, 2)
	require.Len(t, topics[1]["Partitions"], 2)
	_, err = stream_parser.KafkaResponseBodyVersion(0, 0, append(body, 0))
	require.ErrorContains(t, err, "trailing")
	_, err = stream_parser.KafkaResponseBodyVersion(0, 0, kafkaBE32(-2))
	require.Error(t, err)
	batch := kafkaRecordBatch(0, false)
	batches, err := stream_parser.ParseKafkaRecordSet(append(append([]byte(nil), batch...), batch...))
	require.NoError(t, err)
	require.Len(t, batches, 2)
	s := newReviewSession(t, ParserBudget{MaxCollectionElements: 1})
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, kafkaRequest(18, 0, 1, "x", nil)).Err)
	require.NotNil(t, s.Feed(0, ts, kafkaRequest(18, 2, 2, "x", nil)).Err)
}
func FuzzFirstBatchM2A(f *testing.F) {
	for _, w := range [][]byte{respCommand("PING"), []byte("|1\r\n+k\r\n:1\r\n"), kafkaRecordBatch(0, false)} {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 65536 {
			return
		}
		_, _ = DecodeRESPValue(w, ParserBudget{MaxMessageBytes: 65536})
		_, _ = stream_parser.ParseKafkaRecordSet(w)
		for _, api := range []int16{0, 1, 3, 18} {
			_, _ = stream_parser.KafkaResponseBodyVersion(api, 0, w)
		}
	})
}
func TestFirstBatchM2ARedisUnsubscribeAll(t *testing.T) {
	s := newReviewSession(t, ParserBudget{})
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, respCommand("SUBSCRIBE", "a", "b")).Err)
	require.Nil(t, s.Feed(1, ts, []byte("*3\r\n$9\r\nsubscribe\r\n$1\r\na\r\n:1\r\n*3\r\n$9\r\nsubscribe\r\n$1\r\nb\r\n:2\r\n")).Err)
	r := s.Feed(0, ts, append(respCommand("UNSUBSCRIBE"), respCommand("PING")...))
	require.Nil(t, r.Err)
	unsub, ping := r.Events[0].ID, r.Events[1].ID
	r = s.Feed(1, ts, []byte("*3\r\n$11\r\nunsubscribe\r\n$1\r\na\r\n:1\r\n*3\r\n$11\r\nunsubscribe\r\n$1\r\nb\r\n:0\r\n+PONG\r\n"))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 3)
	require.Equal(t, unsub, r.Events[0].ResponseTo)
	require.Equal(t, unsub, r.Events[1].ResponseTo)
	require.Equal(t, ping, r.Events[2].ResponseTo)
}

func TestFirstBatchM2AManifest(t *testing.T) {
	dir := "testdata/protocol-sessions/first-batch-m2a"
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err)
	var m struct {
		Packets     int
		KernelDrops int `json:"kernel_drops"`
		Files       []struct {
			Path   string
			Bytes  int
			SHA256 string
		}
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, 235, m.Packets)
	require.Zero(t, m.KernelDrops)
	require.Len(t, m.Files, 3)
	for _, f := range m.Files {
		b, err := os.ReadFile(filepath.Join(dir, f.Path))
		require.NoError(t, err)
		require.Len(t, b, f.Bytes)
		require.Equal(t, f.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	}
}
