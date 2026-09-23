package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestReplayPcapFileDecodesWinlabSCGICapture(t *testing.T) {
	var messages []*ProtocolEvent
	for _, event := range replayWinlab5013Protocols(t, "12-scgi.pcapng") {
		if event.Protocol == "scgi" {
			require.Equal(t, "decoded", event.Status, "%s: %s", event.Status, event.Error)
			require.Equal(t, "scgi-netstring-bounded", event.Profile)
			messages = append(messages, event)
		}
	}
	require.Len(t, messages, 4)
	require.Equal(t, []string{"request", "response", "request", "response"}, []string{
		messages[0].Fields["Role"].(string), messages[1].Fields["Role"].(string),
		messages[2].Fields["Role"].(string), messages[3].Fields["Role"].(string),
	})
	require.Equal(t, "GET", messages[0].Fields["Request Method"])
	require.Equal(t, "/lab/status", messages[0].Fields["Request URI"])
	require.Equal(t, "scgi-ok-5013", messages[1].Fields["Body"])
	require.Equal(t, "POST", messages[2].Fields["Request Method"])
	require.Equal(t, "/lab/point", messages[2].Fields["Request URI"])
	require.Equal(t, "point=7&value=42", messages[2].Fields["Body"])
	require.Equal(t, "stored", messages[3].Fields["Body"])
}

func TestReplayPcapFileDecodesWinlabMessagePackRPCCapture(t *testing.T) {
	var messages []*ProtocolEvent
	for _, event := range replayWinlab5013Protocols(t, "14-msgpack-rpc.pcapng") {
		if event.Protocol == "msgpack-rpc" {
			require.Equal(t, "decoded", event.Status, "%s: %s", event.Status, event.Error)
			require.Equal(t, "msgpack-rpc-array-bounded", event.Profile)
			messages = append(messages, event)
		}
	}
	require.Len(t, messages, 5)
	require.Equal(t, "request", messages[0].Fields["Message Type"])
	require.Equal(t, uint32(1), messages[0].Fields["Message ID"])
	require.Equal(t, "lab.status", messages[0].Fields["Method"])
	require.Equal(t, "response", messages[1].Fields["Message Type"])
	require.Equal(t, "ok-5013", messages[1].Fields["Result"])
	require.Equal(t, "request", messages[2].Fields["Message Type"])
	require.Equal(t, "lab.set", messages[2].Fields["Method"])
	require.Equal(t, "point", messages[2].Fields["Set Point"])
	require.Equal(t, int64(7), messages[2].Fields["Set Index"])
	require.Equal(t, int64(42), messages[2].Fields["Set Value"])
	require.Equal(t, true, messages[3].Fields["Result"])
	require.Equal(t, "notification", messages[4].Fields["Message Type"])
	require.Equal(t, "lab.event", messages[4].Fields["Method"])
	require.Equal(t, "job", messages[4].Fields["Event Name"])
	require.Equal(t, "7f3a", messages[4].Fields["Job ID"])
}

func TestReplayPcapSCGIRequiresStrictRequestEnvelope(t *testing.T) {
	valid := makeSCGITestRequest("GET", "/lab/status", nil, true)
	nearMatches := map[string][]byte{
		"missing-marker": makeSCGITestRequest("GET", "/lab/status", nil, false),
		"truncated-body": func() []byte {
			request := makeSCGITestRequest("POST", "/lab/point", []byte("point=7"), true)
			return request[:len(request)-2]
		}(),
		"wrong-netstring-comma": func() []byte {
			request := append([]byte(nil), valid...)
			comma := bytes.IndexByte(request, ',')
			request[comma] = ';'
			return request
		}(),
	}
	for name, sample := range nearMatches {
		t.Run(name, func(t *testing.T) {
			for _, event := range replaySCGIMsgpackTestStream(t, 4000, sample, nil, 7) {
				require.NotEqual(t, "scgi", event.Protocol, "%s: %s", event.Status, event.Summary)
			}
		})
	}
	t.Run("valid-envelope-on-another-port", func(t *testing.T) {
		for _, event := range replaySCGIMsgpackTestStream(t, 4001, valid, nil, 7) {
			require.NotEqual(t, "scgi", event.Protocol)
		}
	})
	t.Run("http-remains-http-on-scgi-port", func(t *testing.T) {
		request := []byte("GET /lab/status HTTP/1.1\r\nHost: example.test\r\n\r\n")
		var httpSeen, scgiSeen bool
		for _, event := range replaySCGIMsgpackTestStream(t, 4000, request, nil, 11) {
			httpSeen = httpSeen || event.Protocol == "http"
			scgiSeen = scgiSeen || event.Protocol == "scgi"
		}
		require.True(t, httpSeen)
		require.False(t, scgiSeen)
	})
}

func TestReplayPcapMessagePackRPCRequiresStrictRequestShape(t *testing.T) {
	nearMatches := map[string][]byte{
		"method-prefix-only": {0x94, 0x00, 0x01, 0xab, 'l', 'a', 'b', '.', 's', 't', 'a', 't', 'u', 's', 'x', 0x90},
		"missing-params":     {0x93, 0x00, 0x01, 0xaa, 'l', 'a', 'b', '.', 's', 't', 'a', 't', 'u', 's'},
		"string-message-id":  {0x94, 0x00, 0xa2, 'i', 'd', 0xaa, 'l', 'a', 'b', '.', 's', 't', 'a', 't', 'u', 's', 0x90},
	}
	for name, sample := range nearMatches {
		t.Run(name, func(t *testing.T) {
			for _, event := range replaySCGIMsgpackTestStream(t, 19850, sample, nil, 2) {
				require.NotEqual(t, "msgpack-rpc", event.Protocol, "%s: %s", event.Status, event.Summary)
			}
		})
	}
	valid := msgpackStatusRequest()
	t.Run("valid-rpc-on-another-port", func(t *testing.T) {
		for _, event := range replaySCGIMsgpackTestStream(t, 19851, valid, nil, 1) {
			require.NotEqual(t, "msgpack-rpc", event.Protocol)
		}
	})
}

func TestReplayPcapSCGIAndMessagePackRPCReassembleSingleByteSegments(t *testing.T) {
	scgiRequest := makeSCGITestRequest("GET", "/lab/status", nil, true)
	scgiResponse := []byte("Status: 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\nscgi-ok-5013")
	var scgiEvents []*ProtocolEvent
	for _, event := range replaySCGIMsgpackTestStream(t, 4000, scgiRequest, scgiResponse, 1) {
		if event.Protocol == "scgi" {
			scgiEvents = append(scgiEvents, event)
		}
	}
	require.Len(t, scgiEvents, 2)
	require.Equal(t, "GET", scgiEvents[0].Fields["Request Method"])
	require.Equal(t, "scgi-ok-5013", scgiEvents[1].Fields["Body"])

	client := append(msgpackStatusRequest(), msgpackSetRequest()...)
	server := append(msgpackStatusResponse(), msgpackSetResponse()...)
	var rpcEvents []*ProtocolEvent
	for _, event := range replaySCGIMsgpackTestStream(t, 19850, client, server, 1) {
		if event.Protocol == "msgpack-rpc" {
			rpcEvents = append(rpcEvents, event)
		}
	}
	require.Len(t, rpcEvents, 4)
	require.Equal(t, "lab.status", rpcEvents[0].Fields["Method"])
	require.Equal(t, "lab.set", rpcEvents[1].Fields["Method"])
	require.Equal(t, int64(42), rpcEvents[1].Fields["Set Value"])
	require.Equal(t, "ok-5013", rpcEvents[2].Fields["Result"])
	require.Equal(t, true, rpcEvents[3].Fields["Result"])
}

func replaySCGIMsgpackTestStream(t *testing.T, port uint16, client, server []byte, chunk int) []*ProtocolEvent {
	t.Helper()
	if chunk < 1 {
		chunk = len(client) + len(server)
	}
	steps := []tcpStep{{seq: 99, syn: true}, {seq: 199, syn: true, reverse: true}}
	for offset := 0; offset < len(client); offset += chunk {
		end := min(offset+chunk, len(client))
		steps = append(steps, tcpStep{seq: 100 + uint32(offset), data: string(client[offset:end])})
	}
	for offset := 0; offset < len(server); offset += chunk {
		end := min(offset+chunk, len(server))
		steps = append(steps, tcpStep{seq: 200 + uint32(offset), data: string(server[offset:end]), reverse: true})
	}
	steps = append(steps,
		tcpStep{seq: 100 + uint32(len(client)), fin: true},
		tcpStep{seq: 200 + uint32(len(server)), fin: true, reverse: true},
	)
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(binTestPcap(t, steps, layers.TCPPort(port), false, true)),
		WithTCPReassemblyWorkers(1),
		WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }),
	)
	require.NoError(t, err)
	return events
}

func makeSCGITestRequest(method, uri string, body []byte, includeMarker bool) []byte {
	pairs := [][2]string{{"CONTENT_LENGTH", strconv.Itoa(len(body))}}
	if includeMarker {
		pairs = append(pairs, [2]string{"SCGI", "1"})
	}
	pairs = append(pairs, [2]string{"REQUEST_METHOD", method}, [2]string{"REQUEST_URI", uri})
	var env bytes.Buffer
	for _, pair := range pairs {
		env.WriteString(pair[0])
		env.WriteByte(0)
		env.WriteString(pair[1])
		env.WriteByte(0)
	}
	var wire bytes.Buffer
	wire.WriteString(fmt.Sprintf("%d:", env.Len()))
	wire.Write(env.Bytes())
	wire.WriteByte(',')
	wire.Write(body)
	return wire.Bytes()
}

func msgpackStatusRequest() []byte {
	return []byte{0x94, 0x00, 0x01, 0xaa, 'l', 'a', 'b', '.', 's', 't', 'a', 't', 'u', 's', 0x90}
}

func msgpackStatusResponse() []byte {
	return []byte{0x94, 0x01, 0x01, 0xc0, 0xa7, 'o', 'k', '-', '5', '0', '1', '3'}
}

func msgpackSetRequest() []byte {
	return []byte{0x94, 0x00, 0x02, 0xa7, 'l', 'a', 'b', '.', 's', 'e', 't', 0x93, 0xa5, 'p', 'o', 'i', 'n', 't', 0x07, 0x2a}
}

func msgpackSetResponse() []byte {
	return []byte{0x94, 0x01, 0x02, 0xc0, 0xc3}
}
