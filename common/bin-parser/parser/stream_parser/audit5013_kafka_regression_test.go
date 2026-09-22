package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func audit5013KafkaRequest(api, ver int16, body []byte) []byte {
	var w bytes.Buffer
	binary.Write(&w, binary.BigEndian, api)
	binary.Write(&w, binary.BigEndian, ver)
	binary.Write(&w, binary.BigEndian, int32(1))
	binary.Write(&w, binary.BigEndian, int16(0))
	w.Write(body)
	var out bytes.Buffer
	binary.Write(&out, binary.BigEndian, int32(w.Len()))
	out.Write(w.Bytes())
	return out.Bytes()
}
func TestAudit5013ControlMetadataV0(t *testing.T) {
	_, _, err := decodeKafkaFields(audit5013KafkaRequest(3, 0, []byte{0, 0, 0, 0}), "request")
	if err != nil {
		t.Fatal(err)
	}
}
func TestAudit5013MetadataV4AllowAutoTopicCreation(t *testing.T) {
	// Kafka MetadataRequest v4: topics empty + allow_auto_topic_creation=true.
	_, _, err := decodeKafkaFields(audit5013KafkaRequest(3, 4, []byte{0, 0, 0, 0, 1}), "request")
	if err != nil {
		t.Fatalf("valid MetadataRequest v4 rejected: %v", err)
	}
}
func TestAudit5013FetchV5LogStartOffsetIsInt64(t *testing.T) {
	var b bytes.Buffer
	for _, x := range []int32{-1, 100, 1, 1048576} {
		binary.Write(&b, binary.BigEndian, x)
	}
	b.WriteByte(0)                               // isolation_level
	binary.Write(&b, binary.BigEndian, int32(1)) // topics
	binary.Write(&b, binary.BigEndian, int16(1))
	b.WriteByte('t')
	binary.Write(&b, binary.BigEndian, int32(1))  // partitions
	binary.Write(&b, binary.BigEndian, int32(0))  // partition
	binary.Write(&b, binary.BigEndian, int64(0))  // fetch_offset
	binary.Write(&b, binary.BigEndian, int64(-1)) // log_start_offset
	binary.Write(&b, binary.BigEndian, int32(1024))
	_, _, err := decodeKafkaFields(audit5013KafkaRequest(1, 5, b.Bytes()), "request")
	if err != nil {
		t.Fatalf("valid FetchRequest v5 rejected: %v", err)
	}
}
func TestAudit5013ControlApiVersionsV1Context(t *testing.T) {
	// v1: error_code=0 + empty api_keys + throttle_time_ms=0.
	body := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	info, err := KafkaResponseBodyVersion(18, 1, body)
	require.Equal(t, int32(0), info["Throttle Time"])
	_, legacyErr := ParseKafkaResponseBody(18, body)
	require.Error(t, legacyErr, "v1 tail must not be silently accepted as v0")
	if err != nil {
		t.Fatalf("valid ApiVersionsResponse v1 rejected: %v", err)
	}
}

// Apache Kafka 3.9.0 message schemas (MetadataRequest.json / FetchRequest.json)
// independently define these version-dependent layouts. No broker capture is
// claimed for these handcrafted boundary vectors.
func TestFirstBatchT15(t *testing.T) {
	for ver := int16(0); ver <= 8; ver++ {
		t.Run(fmt.Sprintf("metadata-v%d", ver), func(t *testing.T) {
			for _, count := range []int32{0, -1} {
				var body bytes.Buffer
				binary.Write(&body, binary.BigEndian, count)
				if ver >= 4 {
					body.WriteByte(1)
				}
				if ver >= 8 {
					body.Write([]byte{1, 0})
				}
				wire := audit5013KafkaRequest(3, ver, body.Bytes())
				_, info, err := decodeKafkaFields(wire, "request")
				if count == -1 && ver == 0 {
					require.Error(t, err)
					continue
				}
				require.NoError(t, err)
				require.Equal(t, count == -1 || (count == 0 && ver == 0), info["All Topics"])
				if ver >= 4 {
					require.Equal(t, true, info["Allow Auto Topic Creation"])
				}
				if ver >= 8 {
					require.Equal(t, true, info["Include Cluster Authorized Operations"])
					require.Equal(t, false, info["Include Topic Authorized Operations"])
				}
				if ver >= 4 {
					_, _, err = decodeKafkaFields(audit5013KafkaRequest(3, ver, body.Bytes()[:body.Len()-1]), "request")
					require.Error(t, err)
				}
			}
		})
	}
	for ver := int16(0); ver <= 11; ver++ {
		t.Run(fmt.Sprintf("fetch-v%d", ver), func(t *testing.T) {
			var body bytes.Buffer
			i32 := func(n int32) { binary.Write(&body, binary.BigEndian, n) }
			i64 := func(n int64) { binary.Write(&body, binary.BigEndian, n) }
			str := func(s string) { binary.Write(&body, binary.BigEndian, int16(len(s))); body.WriteString(s) }
			i32(-1)
			i32(100)
			i32(1)
			if ver >= 3 {
				i32(1048576)
			}
			if ver >= 4 {
				body.WriteByte(0)
			}
			if ver >= 7 {
				i32(37)
				i32(2)
			}
			i32(1)
			str("topic")
			i32(1)
			i32(3)
			if ver >= 9 {
				i32(23)
			}
			i64(1<<40 + 7)
			if ver >= 5 {
				i64(-1)
			}
			i32(1024)
			if ver >= 7 {
				i32(1)
				str("old-topic")
				i32(1)
				i32(9)
			}
			if ver >= 11 {
				str("rack-a")
			}
			_, info, err := decodeKafkaFields(audit5013KafkaRequest(1, ver, body.Bytes()), "request")
			require.NoError(t, err)
			require.Equal(t, int64(1<<40+7), info["Fetch Offset"])
			require.Equal(t, int32(1024), info["Partition Max Bytes"])
			if ver >= 5 {
				require.Equal(t, int64(-1), info["Log Start Offset"])
			}
			if ver >= 7 {
				require.Equal(t, int32(37), info["Session ID"])
			}
			if ver >= 9 {
				require.Equal(t, int32(23), info["Current Leader Epoch"])
			}
			if ver >= 11 {
				require.Equal(t, "rack-a", info["Rack ID"])
			}
			for _, b := range [][]byte{body.Bytes()[:body.Len()-1], append(body.Bytes(), 0)} {
				_, _, err := decodeKafkaFields(audit5013KafkaRequest(1, ver, b), "request")
				require.Error(t, err)
			}
		})
	}
}
