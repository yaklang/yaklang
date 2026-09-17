package stream_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func kafkaTestBE16(v int16) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(v))
	return b[:]
}

func kafkaTestBE32(v int32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	return b[:]
}

func kafkaTestFrame(payload []byte) []byte {
	return append(kafkaTestBE32(int32(len(payload))), payload...)
}

func TestKafkaFieldsApiVersionsRequest(t *testing.T) {
	payload := append(kafkaTestBE16(18), kafkaTestBE16(0)...)
	payload = append(payload, kafkaTestBE32(7)...)
	payload = append(payload, 0xff, 0xff)
	wire := kafkaTestFrame(payload)
	_, info, err := decodeKafkaFields(wire, "request")
	require.NoError(t, err)
	require.Equal(t, "ApiVersions", info["API Name"])
	require.Equal(t, int32(7), info["Correlation ID"])
	_, _, err = decodeKafkaFields(wire[:6], "request")
	require.Error(t, err)
	_, _, err = decodeKafkaFields(kafkaTestFrame(append(kafkaTestBE16(99), kafkaTestBE16(0)...)), "request")
	require.Error(t, err)
}

func TestKafkaParseResponseBodyApiVersions(t *testing.T) {
	body := kafkaTestBE16(0)
	body = append(body, kafkaTestBE32(1)...)
	body = append(body, kafkaTestBE16(18)...)
	body = append(body, kafkaTestBE16(0)...)
	body = append(body, kafkaTestBE16(2)...)
	info, err := ParseKafkaResponseBody(18, body)
	require.NoError(t, err)
	require.Equal(t, int16(0), info["Error Code"])
	require.Equal(t, []int16{18}, info["API Keys"])
}
