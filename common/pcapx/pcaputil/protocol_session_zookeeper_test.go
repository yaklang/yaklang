package pcaputil

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func zookeeperTestFrame(body []byte) []byte {
	wire := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(wire[:4], uint32(len(body)))
	copy(wire[4:], body)
	return wire
}

func zookeeperTestInt32(value int32) []byte {
	var wire [4]byte
	binary.BigEndian.PutUint32(wire[:], uint32(value))
	return wire[:]
}

func zookeeperTestInt64(value uint64) []byte {
	var wire [8]byte
	binary.BigEndian.PutUint64(wire[:], value)
	return wire[:]
}

func zookeeperTestString(value string) []byte {
	wire := zookeeperTestInt32(int32(len(value)))
	return append(wire, value...)
}

func zookeeperTestResponseHeader(xid int32, zxid uint64, errCode int32) []byte {
	body := append([]byte(nil), zookeeperTestInt32(xid)...)
	body = append(body, zookeeperTestInt64(zxid)...)
	body = append(body, zookeeperTestInt32(errCode)...)
	return body
}

func zookeeperTestRequest(xid, opcode int32, path string) []byte {
	body := append([]byte(nil), zookeeperTestInt32(xid)...)
	body = append(body, zookeeperTestInt32(opcode)...)
	if path != "" {
		body = append(body, zookeeperTestString(path)...)
		body = append(body, 0) // watch=false
	}
	return zookeeperTestFrame(body)
}

func zookeeperTestSteps() []sessionStep {
	clientConnect := zookeeperConnectProbeFixture([]byte("winlab-session!!"))
	serverConnect := zookeeperConnectResponseProbeFixture([]byte("winlab-session!!"))

	ping := append([]byte(nil), zookeeperTestInt32(-2)...)
	ping = append(ping, zookeeperTestInt32(zookeeperOpcodePing)...)
	pingResponse := zookeeperTestResponseHeader(0, 2, 0)

	childrenRequest := zookeeperTestRequest(1, zookeeperOpcodeGetChildren, "/")
	childrenResponse := zookeeperTestResponseHeader(1, 2, 0)
	childrenResponse = append(childrenResponse, zookeeperTestInt32(2)...)
	childrenResponse = append(childrenResponse, zookeeperTestString("lab")...)
	childrenResponse = append(childrenResponse, zookeeperTestString("znode")...)

	dataRequest := zookeeperTestRequest(2, zookeeperOpcodeGetData, "/lab")
	dataResponse := zookeeperTestResponseHeader(2, 3, 0)
	dataResponse = append(dataResponse, zookeeperTestString("winlab")...)
	stat := make([]byte, 68)
	binary.BigEndian.PutUint64(stat[:8], 1)
	binary.BigEndian.PutUint64(stat[8:16], 3)
	binary.BigEndian.PutUint32(stat[52:56], 6)
	dataResponse = append(dataResponse, stat...)

	closeRequest := append([]byte(nil), zookeeperTestInt32(3)...)
	closeRequest = append(closeRequest, zookeeperTestInt32(zookeeperOpcodeCloseSession)...)
	closeResponse := zookeeperTestResponseHeader(3, 3, 0)
	return []sessionStep{
		{0, clientConnect}, {1, serverConnect},
		{0, zookeeperTestFrame(ping)}, {1, zookeeperTestFrame(pingResponse)},
		{0, childrenRequest}, {1, zookeeperTestFrame(childrenResponse)},
		{0, dataRequest}, {1, zookeeperTestFrame(dataResponse)},
		{0, zookeeperTestFrame(closeRequest)}, {1, zookeeperTestFrame(closeResponse)},
	}
}

func TestZooKeeperProbeRequiresAValidConnectHandshake(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	connect := zookeeperConnectProbeFixture([]byte("winlab-session!!"))
	p := s.Probe(connect)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "zookeeper", p.Protocol)

	response := zookeeperConnectResponseProbeFixture([]byte("winlab-session!!"))
	p = s.Probe(response)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "zookeeper", p.Protocol)

	for n := 1; n < len(connect); n++ {
		require.Equal(t, ProbeNeedMore, probeZooKeeper(connect[:n], 64).Verdict, "valid connect prefix contradicted at length=%d", n)
	}
	for n := 1; n < len(response); n++ {
		require.Equal(t, ProbeNeedMore, probeZooKeeper(response[:n], 64).Verdict, "valid response prefix contradicted at length=%d", n)
	}
	for name, malformed := range map[string][]byte{
		"nonzero protocol version": func() []byte {
			w := append([]byte(nil), connect...)
			binary.BigEndian.PutUint32(w[4:8], 1)
			return w
		}(),
		"zero timeout": func() []byte {
			w := append([]byte(nil), connect...)
			binary.BigEndian.PutUint32(w[16:20], 0)
			return w
		}(),
		"reconnect session id": func() []byte {
			w := append([]byte(nil), connect...)
			binary.BigEndian.PutUint64(w[20:28], 0x5013)
			return w
		}(),
		"inconsistent password length": func() []byte {
			w := append([]byte(nil), connect...)
			binary.BigEndian.PutUint32(w[28:32], 15)
			return w
		}(),
		"invalid read-only flag": func() []byte {
			w := append([]byte(nil), connect...)
			w[len(w)-1] = 2
			return w
		}(),
	} {
		require.NotEqual(t, ProbeAccept, probeZooKeeper(malformed, len(malformed)).Verdict, name)
		require.NotEqual(t, "zookeeper", s.Probe(malformed).Protocol, name)
	}
	near := kafkaProduceV0SimilarPrefixFixture()
	require.Len(t, near, len(connect))
	require.NotEqual(t, "zookeeper", s.Probe(near).Protocol, "same-sized valid Kafka Produce request")
	for name, packet := range map[string][]byte{
		"PostgreSQL startup":        pgSessionUntyped(196608, []byte("user\x00test\x00\x00")),
		"PostgreSQL SSL request":    pgSessionUntyped(80877103, nil),
		"PostgreSQL cancel request": pgSessionUntyped(80877102, make([]byte, 8)),
	} {
		require.Equal(t, ProbeReject, probeZooKeeper(packet, len(packet)).Verdict, name)
		p := s.Probe(packet)
		require.Equal(t, ProbeAccept, p.Verdict, name)
		require.Equal(t, "postgresql", p.Protocol, name)
	}
	gss := pgSessionUntyped(80877104, nil)
	require.Equal(t, ProbeReject, probeZooKeeper(gss, len(gss)).Verdict, "PostgreSQL GSSENC request")
}

func TestZooKeeperProbeRejectsContradictedPartialHandshakeFields(t *testing.T) {
	connect := zookeeperConnectProbeFixture([]byte("winlab-session!!"))
	response := zookeeperConnectResponseProbeFixture([]byte("winlab-session!!"))
	for name, candidate := range map[string]struct {
		wire []byte
		have int
	}{
		"client protocol version": {func() []byte { w := append([]byte(nil), connect...); w[7] = 1; return w }(), 8},
		"client session timeout": {func() []byte {
			w := append([]byte(nil), connect...)
			w[16] = 0
			w[17] = 0
			w[18] = 0
			w[19] = 0
			return w
		}(), 20},
		"client reconnect id": {func() []byte { w := append([]byte(nil), connect...); w[27] = 1; return w }(), 28},
		"client password length": {func() []byte {
			w := append([]byte(nil), connect...)
			binary.BigEndian.PutUint32(w[28:32], 15)
			return w
		}(), 32},
		"server protocol version": {func() []byte { w := append([]byte(nil), response...); w[7] = 1; return w }(), 8},
		"server session timeout": {func() []byte {
			w := append([]byte(nil), response...)
			w[8] = 0
			w[9] = 0
			w[10] = 0
			w[11] = 0
			return w
		}(), 28},
		"server session id": {func() []byte { w := append([]byte(nil), response...); clear(w[12:20]); return w }(), 20},
		"server password length": {func() []byte {
			w := append([]byte(nil), response...)
			binary.BigEndian.PutUint32(w[20:24], 15)
			return w
		}(), 28},
	} {
		require.Equal(t, ProbeReject, probeZooKeeper(candidate.wire[:candidate.have], 64).Verdict, name)
	}
}

func TestZooKeeperProbeRejectsHTTPKafkaAndPostgresNearMisses(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	http := []byte("GET /health HTTP/1.1\r\nHost: example.invalid\r\n\r\n")
	cases := map[string]struct {
		wire     []byte
		protocol string
	}{
		"HTTP start line":                        {http, "http"},
		"Kafka API request":                      {kafkaRequest(18, 0, 1, "", nil), "kafka"},
		"Kafka Produce with Connect-like prefix": {kafkaProduceV0SimilarPrefixFixture(), "kafka"},
		"PostgreSQL Startup":                     {pgSessionUntyped(196608, []byte("user\x00test\x00\x00")), "postgresql"},
		"PostgreSQL SSL request":                 {pgSessionUntyped(80877103, nil), "postgresql"},
		"PostgreSQL Cancel request":              {pgSessionUntyped(80877102, make([]byte, 8)), "postgresql"},
	}
	for name, candidate := range cases {
		require.Equal(t, ProbeReject, probeZooKeeper(candidate.wire, 64).Verdict, name)
		p := s.Probe(candidate.wire)
		require.Equal(t, ProbeAccept, p.Verdict, name)
		require.Equal(t, candidate.protocol, p.Protocol, name)
	}
}

func TestZooKeeperSupportedSessionFieldsAndFragmentation(t *testing.T) {
	steps := zookeeperTestSteps()
	assertFragmentation(t, steps, func(chunk int) []string {
		events, _ := sessionTestFlow(t, "zookeeper", steps, chunk, false)
		names := make([]string, 0, len(events))
		for _, event := range events {
			require.Equal(t, "zookeeper", event.Protocol)
			require.Equal(t, "decoded", event.Status, "%s: %s", event.Summary, event.Error)
			names = append(names, event.Protocol+":"+event.Session["Packet Name"].(string))
		}
		return names
	})

	events, _ := sessionTestFlow(t, "zookeeper", steps, 0, false)
	require.Len(t, events, 10)
	require.Equal(t, uint64(0x5013), events[1].Fields["Session ID"])
	require.Equal(t, "/", events[4].Fields["Path"])
	require.Equal(t, []string{"lab", "znode"}, events[5].Fields["Children"])
	require.Equal(t, "/lab", events[6].Fields["Path"])
	require.Equal(t, "winlab", events[7].Fields["Value"])
	require.Equal(t, "CloseSession", events[8].Fields["Packet Name"])
}
