package bin_parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestProtocolCorpusHTTPApplicationsEveryCapturedMessage(t *testing.T) {
	captureIDs := []string{"gen-etcd", "gen-minio-s3", "gen-docker-api", "gen-redfish", "gen-wpad", "gen-wpad-proxy", "gen-prometheus", "gen-jsonrpc", "gen-xmlrpc"}
	frameCount, messageCount := 0, 0
	for _, id := range captureIDs {
		t.Run(id, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/"+id+".pcap")
			for _, frame := range frames {
				frameCount++
				packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
				require.Nil(t, packet.ErrorLayer())
				tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
				if len(tcp.Payload) == 0 {
					continue
				}
				messageCount++
				data := tcp.Payload
				node := protocolCorpusRequireBoundedRuleParse(t, data, "application-layer.http", "HTTP")
				reader := bufio.NewReader(bytes.NewReader(data))
				var body io.ReadCloser
				if bytes.HasPrefix(data, []byte("HTTP/")) {
					response, err := http.ReadResponse(reader, nil)
					require.NoError(t, err)
					protocolCorpusRequireValue(t, node, "Version", response.Proto)
					protocolCorpusRequireValue(t, node, "Status", fmt.Sprint(response.StatusCode))
					protocolCorpusRequireValue(t, node, "Message", strings.TrimPrefix(response.Status, fmt.Sprint(response.StatusCode)+" "))
					body = response.Body
				} else {
					request, err := http.ReadRequest(reader)
					require.NoError(t, err)
					protocolCorpusRequireValue(t, node, "Method", request.Method)
					protocolCorpusRequireValue(t, node, "Path", request.RequestURI)
					protocolCorpusRequireValue(t, node, "Version", request.Proto)
					body = request.Body
				}
				contents, err := io.ReadAll(body)
				require.NoError(t, err)
				require.NoError(t, body.Close())
				if len(contents) > 0 {
					protocolCorpusRequireValue(t, node, "Octets", string(contents))
				}
				remaining, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Empty(t, remaining)
				boundary := bytes.Index(data, []byte("\r\n\r\n"))
				require.GreaterOrEqual(t, boundary, 0)
				lines := strings.Split(string(data[:boundary]), "\r\n")[1:]
				items := protocolCorpusNodesNamed(node, "Item")
				require.Len(t, items, len(lines)+1, "include the empty header terminator")
				for i, line := range append(lines, "") {
					value, err := items[i].Result()
					require.NoError(t, err)
					require.Equal(t, line, strVal(t, value))
				}
				for cut := 0; cut < len(data); cut++ {
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(data[:cut]), "application-layer.http", "HTTP")
					require.Errorf(t, err, "cut %d/%d", cut, len(data))
				}
			}
		})
	}
	require.Equal(t, 37, frameCount)
	require.Equal(t, 10, messageCount)
}

func TestProtocolCorpusHTTPFramingDoesNotLeakBetweenMessages(t *testing.T) {
	messages := [][]byte{
		[]byte("POST /first HTTP/1.1\r\nHost: example\r\ncOnTeNt-LeNgTh:\t3\r\nContent-Length: 3\r\n\r\none"),
		[]byte("GET /empty HTTP/1.1\r\nHost: example\r\n\r\n"),
		[]byte("POST /chunks HTTP/1.1\r\nHost: example\r\ntransfer-encoding: Chunked\r\n\r\n3;name=demo\r\ntwo\r\n0\r\nX-Note: done\r\n\r\n"),
		[]byte("GET /after-chunks HTTP/1.1\r\nHost: example\r\n\r\n"),
		[]byte("HTTP/1.1 304 Not Modified\r\nContent-Length: 88\r\n\r\n"),
		[]byte("HTTP/1.1 200 OK\r\ncontent-length: 5\r\n\r\nthree"),
	}
	for count := 1; count <= len(messages); count++ {
		t.Run(fmt.Sprintf("prefix-%d", count), func(t *testing.T) {
			protocolCorpusRequireBoundedRuleParse(t, bytes.Join(messages[:count], nil), "application-layer.http", "HTTPMessages")
		})
	}
	node := protocolCorpusRequireBoundedRuleParse(t, bytes.Join(messages, nil), "application-layer.http", "HTTPMessages")
	parsed := node.Children
	require.Len(t, parsed, len(messages))
	for index, body := range map[int]string{0: "one", 2: "two", 5: "three"} {
		protocolCorpusRequireValue(t, parsed[index], "Octets", body)
	}
	for _, index := range []int{1, 3, 4} {
		require.Nil(t, protocolCorpusFindNode(parsed[index], "Octets"), "body state leaked into message %d", index)
	}
	trailers := protocolCorpusFindNode(parsed[2], "Trailers")
	require.NotNil(t, trailers)
	items := protocolCorpusNodesNamed(trailers, "Item")
	require.Len(t, items, 2)
	value, err := items[0].Result()
	require.NoError(t, err)
	require.Equal(t, "X-Note: done", strVal(t, value))
	for cut := 0; cut < len(messages[2]); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(messages[2][:cut]), "application-layer.http", "HTTP")
		require.Errorf(t, err, "chunked cut %d", cut)
	}
	closeDelimited := []byte("HTTP/1.0 200 OK\r\n\r\nbody")
	reader := newProtocolCorpusBoundedReader(closeDelimited)
	closed, err := parser.ParseBinaryWithConfig(reader, "application-layer.http", map[string]any{"httpCloseDelimited": true}, "HTTP")
	require.NoError(t, err)
	require.Zero(t, reader.Len())
	protocolCorpusRequireValue(t, closed, "Octets", "body")
	for _, message := range []string{"HTTP/1.0 200 OK\r\n\r\n", "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"} {
		emptyReader := newProtocolCorpusBoundedReader([]byte(message))
		empty, err := parser.ParseBinaryWithConfig(emptyReader, "application-layer.http", map[string]any{"httpCloseDelimited": true}, "HTTP")
		require.NoError(t, err)
		require.Zero(t, emptyReader.Len())
		require.Nil(t, protocolCorpusFindNode(empty, "Octets"))
	}
	for _, method := range []string{"HEAD", "CONNECT"} {
		header := "HTTP/1.1 200 OK\r\nContent-Length: 99\r\n\r\n"
		contextReader := newProtocolCorpusBoundedReader([]byte(header + "next"))
		contextual, err := parser.ParseBinaryWithConfig(contextReader, "application-layer.http", map[string]any{"httpResponseToMethod": method}, "HTTP")
		require.NoError(t, err)
		require.Equal(t, 4, contextReader.Len(), "next response or tunnel bytes are not an HTTP body")
		require.Nil(t, protocolCorpusFindNode(contextual, "Octets"))
	}
}

func TestProtocolCorpusHTTPRejectsAmbiguousAndIncompleteFraming(t *testing.T) {
	for _, suffix := range []string{
		"Content-Length: -1\r\n\r\n",
		"Content-Length: 3x\r\n\r\none",
		"Content-Length: 3\r\ncontent-length: 4\r\n\r\none!",
		"Content-Length: 9999999999999999999999\r\n\r\n",
		"Content-Length: 3\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
		"Transfer-Encoding: chunked\r\n\r\n3x\r\none\r\n0\r\n\r\n",
		"Transfer-Encoding: chunked\r\n\r\n3\r\nonex\r\n0\r\n\r\n",
		"Transfer-Encoding: chunked\r\n\r\n0\r\nContent-Length: 3\r\n\r\n",
		"Bad(Name): value\r\n\r\n",
		"X-Note: value\x00\r\n\r\n",
		"Transfer-Encoding: chunked\r\n\r\n0\r\nBad(Name): value\r\n\r\n",
		"Transfer-Encoding: chunked\r\n\r\n0\r\nX-Note: value\x7f\r\n\r\n",
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte("POST / HTTP/1.1\r\n"+suffix)), "application-layer.http", "HTTP")
		require.Errorf(t, err, "%q", suffix)
	}
	_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader([]byte("POST / HTTP/1.1\r\nContent-Length: 4\r\n\r\ndata")), "application-layer.http", map[string]any{"httpBodyLimit": 3}, "HTTP")
	require.ErrorContains(t, err, "configured limit")
}

func TestProtocolCorpusLPDEveryCommandField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/gen-lpd.pcap")
	require.Len(t, frames, 4)
	messages := 0
	for _, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if len(tcp.Payload) == 0 {
			continue
		}
		messages++
		require.Equal(t, layers.TCPPort(515), tcp.DstPort)
		require.Equal(t, []byte("\x02printer\n"), tcp.Payload)
		node := protocolCorpusRequireBoundedRuleParse(t, tcp.Payload, "application-layer.lpd", "LPD")
		protocolCorpusRequireValue(t, node, "Command", uint64(2))
		protocolCorpusRequireValue(t, node, "Queue", "printer")
		for cut := 0; cut < len(tcp.Payload); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload[:cut]), "application-layer.lpd", "LPD")
			require.Error(t, err)
		}
	}
	require.Equal(t, 1, messages)
	for _, data := range [][]byte{[]byte("\x02\n"), []byte("\x02two names\n"), []byte("\x02printer\r\n"), []byte("\x00printer\n")} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(data), "application-layer.lpd", "LPD")
		require.Error(t, err)
	}
	protocolCorpusRequireBoundedRuleParse(t, []byte("\x01queue-2\n"), "application-layer.lpd", "LPD")
}
