package pcaputil

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func kafkaBE16(v int16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(v))
	return b[:]
}

func kafkaBE32(v int32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	return b[:]
}

func kafkaBE64(v int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	return b[:]
}

func kafkaStr(s string) []byte {
	return append(kafkaBE16(int16(len(s))), s...)
}

func kafkaFrame(payload []byte) []byte {
	return append(kafkaBE32(int32(len(payload))), payload...)
}

func kafkaRequest(api, ver int16, corr int32, client string, body []byte) []byte {
	p := append(kafkaBE16(api), kafkaBE16(ver)...)
	p = append(p, kafkaBE32(corr)...)
	if client == "" {
		p = append(p, 0xff, 0xff)
	} else {
		p = append(p, kafkaStr(client)...)
	}
	return kafkaFrame(append(p, body...))
}

func kafkaResponse(corr int32, body []byte) []byte {
	return kafkaFrame(append(kafkaBE32(corr), body...))
}

func TestKafkaProbeDefersPeerHTTP2Settings(t *testing.T) {
	for _, value := range []uint32{0, 8192, 65535} {
		payload := append(kafkaBE16(1), kafkaBE32(int32(value))...)
		settings := h2TestFrame(4, 0, 0, payload)
		for n := 1; n <= len(settings); n++ {
			require.NotEqual(t, ProbeAccept, probeKafka(settings[:n], 64).Verdict, "value=%d prefix=%d", value, n)
		}
	}
	// This genuine request header has the same apparent six-byte SETTINGS
	// length. Additional Kafka bytes must resolve it without excluding API 0.
	req := kafkaRequest(0, 0, 1, "", make([]byte, 1530))
	require.Equal(t, 1544, len(req))
	require.Equal(t, ProbeNeedMore, probeKafka(req[:14], 64).Verdict)
	require.Equal(t, ProbeAccept, probeKafka(req[:16], 64).Verdict)
	bad := kafkaRequest(18, 0, 1, "", nil)
	binary.BigEndian.PutUint16(bad[12:14], 1)
	require.Equal(t, ProbeReject, probeKafka(bad, 64).Verdict)
}

func kafkaRecordBatch(count int32, compressed bool) []byte {
	rest := kafkaBE32(-1) // leader epoch
	rest = append(rest, 2)
	rest = append(rest, 0, 0, 0, 0)
	attr := byte(0)
	if compressed {
		attr = 1
	}
	rest = append(rest, 0, attr)
	rest = append(rest, kafkaBE32(0)...)
	rest = append(rest, make([]byte, 8)...)
	rest = append(rest, make([]byte, 8)...)
	rest = append(rest, kafkaBE64(-1)...)
	rest = append(rest, 0xff, 0xff)
	rest = append(rest, kafkaBE32(-1)...)
	rest = append(rest, kafkaBE32(count)...)
	binary.BigEndian.PutUint32(rest[5:9], crc32.Checksum(rest[9:], crc32.MakeTable(crc32.Castagnoli)))
	batch := kafkaBE64(0)
	batch = append(batch, kafkaBE32(int32(len(rest)))...)
	return append(batch, rest...)
}

func TestProtocolSessionKafkaAPIsAndRecordBatch(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	apiReq := kafkaRequest(18, 0, 1, "", nil)
	p := s.Probe(apiReq)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "kafka", p.Protocol)
	r := s.Feed(0, ts, apiReq)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "ApiVersions", r.Events[0].Session["API Name"])
	require.Equal(t, int32(1), r.Events[0].Session["Correlation ID"])
	apiResp := kafkaResponse(1, append(append(kafkaBE16(0), kafkaBE32(1)...), append(kafkaBE16(18), append(kafkaBE16(0), kafkaBE16(2)...)...)...))
	r = s.Feed(1, ts, apiResp)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Matched Request"])
	require.Equal(t, "ApiVersions", r.Events[0].Session["API Name"])

	mdReq := kafkaRequest(3, 0, 2, "test", append(kafkaBE32(1), kafkaStr("foo")...))
	r = s.Feed(0, ts, mdReq)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Metadata", r.Events[0].Session["API Name"])
	require.Equal(t, "foo", findSessionField(r.Events[0].Fields, "Topic Name"))
	mdBody := kafkaBE32(0)
	mdBody = append(mdBody, kafkaBE32(1)...)
	mdBody = append(mdBody, kafkaBE16(0)...)
	mdBody = append(mdBody, kafkaStr("foo")...)
	mdBody = append(mdBody, kafkaBE32(1)...)
	mdBody = append(mdBody, kafkaBE16(0)...)
	mdBody = append(mdBody, kafkaBE32(0)...)
	mdBody = append(mdBody, kafkaBE32(1)...)
	mdBody = append(mdBody, kafkaBE32(1)...)
	mdBody = append(mdBody, kafkaBE32(1)...)
	mdBody = append(mdBody, kafkaBE32(1)...)
	mdBody = append(mdBody, kafkaBE32(1)...)
	r = s.Feed(1, ts, kafkaResponse(2, mdBody))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Matched Request"])

	batch := kafkaRecordBatch(0, false)
	prodBody := kafkaBE16(1)
	prodBody = append(prodBody, kafkaBE32(1000)...)
	prodBody = append(prodBody, kafkaBE32(1)...)
	prodBody = append(prodBody, kafkaStr("t")...)
	prodBody = append(prodBody, kafkaBE32(1)...)
	prodBody = append(prodBody, kafkaBE32(0)...)
	prodBody = append(prodBody, kafkaBE32(int32(len(batch)))...)
	prodBody = append(prodBody, batch...)
	r = s.Feed(0, ts, kafkaRequest(0, 0, 3, "c", prodBody))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Produce", r.Events[0].Session["API Name"])
	require.Equal(t, int8(2), r.Events[0].Session["Magic"])
	require.Equal(t, int16(0), r.Events[0].Session["Compression"])
	require.Equal(t, int32(0), r.Events[0].Session["Records Count"])
	prodResp := kafkaBE32(1)
	prodResp = append(prodResp, kafkaStr("t")...)
	prodResp = append(prodResp, kafkaBE32(1)...)
	prodResp = append(prodResp, kafkaBE32(0)...)
	prodResp = append(prodResp, kafkaBE16(0)...)
	prodResp = append(prodResp, kafkaBE64(0)...)
	r = s.Feed(1, ts, kafkaResponse(3, prodResp))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Matched Request"])

	fetchBody := kafkaBE32(-1)
	fetchBody = append(fetchBody, kafkaBE32(500)...)
	fetchBody = append(fetchBody, kafkaBE32(1)...)
	fetchBody = append(fetchBody, kafkaBE32(1)...)
	fetchBody = append(fetchBody, kafkaStr("t")...)
	fetchBody = append(fetchBody, kafkaBE32(1)...)
	fetchBody = append(fetchBody, kafkaBE32(0)...)
	fetchBody = append(fetchBody, kafkaBE64(0)...)
	fetchBody = append(fetchBody, kafkaBE32(1024)...)
	r = s.Feed(0, ts, kafkaRequest(1, 0, 4, "c", fetchBody))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Fetch", r.Events[0].Session["API Name"])
	fetchResp := kafkaBE32(1)
	fetchResp = append(fetchResp, kafkaStr("t")...)
	fetchResp = append(fetchResp, kafkaBE32(1)...)
	fetchResp = append(fetchResp, kafkaBE32(0)...)
	fetchResp = append(fetchResp, kafkaBE16(0)...)
	fetchResp = append(fetchResp, kafkaBE64(0)...)
	fetchResp = append(fetchResp, kafkaBE32(int32(len(batch)))...)
	fetchResp = append(fetchResp, batch...)
	r = s.Feed(1, ts, kafkaResponse(4, fetchResp))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Matched Request"])
	require.Equal(t, int8(2), r.Events[0].Session["Magic"])

	cut := s.Feed(0, ts, apiReq[:5])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionKafkaFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	badAPI := kafkaRequest(99, 0, 1, "", nil)
	require.Equal(t, ProbeReject, s.Probe(badAPI).Verdict)
	r := s.Feed(0, ts, badAPI)
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, kafkaRequest(18, 0, 1, "", nil)).Err)
	batch := kafkaRecordBatch(0, true)
	prodBody := kafkaBE16(1)
	prodBody = append(prodBody, kafkaBE32(1000)...)
	prodBody = append(prodBody, kafkaBE32(1)...)
	prodBody = append(prodBody, kafkaStr("t")...)
	prodBody = append(prodBody, kafkaBE32(1)...)
	prodBody = append(prodBody, kafkaBE32(0)...)
	prodBody = append(prodBody, kafkaBE32(int32(len(batch)))...)
	prodBody = append(prodBody, batch...)
	r = s2.Feed(0, ts, kafkaRequest(0, 0, 2, "c", prodBody))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s3.Feed(0, ts, kafkaRequest(18, 0, 1, "", nil)[:6])
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)
}

func TestProtocolSessionKafkaFragmentation(t *testing.T) {
	req := kafkaRequest(18, 0, 1, "", nil)
	resp := kafkaResponse(1, append(kafkaBE16(0), kafkaBE32(0)...))
	steps := []sessionStep{{0, req}, {1, resp}}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		var names []string
		for _, st := range steps {
			for w := st.wire; len(w) > 0; {
				n := len(w)
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(st.dir, ts, w[:n])
				for _, e := range r.Events {
					if e.Status == "decoded" || e.Status == "deferred" {
						names = append(names, fmt.Sprint(e.Session["API Name"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
