package bin_parser

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const webDAVCorpusCapture = "testdata/protocol-corpus/captures/ndpi/ndpi-webdav.pcap"

type webDAVParsedMessage struct {
	payload []byte
	value   *base.NodeValue
}

type webDAVMultistatus struct {
	XMLName   xml.Name         `xml:"DAV: multistatus"`
	Responses []webDAVResponse `xml:"DAV: response"`
}

type webDAVResponse struct {
	Href      string           `xml:"DAV: href"`
	Propstats []webDAVPropstat `xml:"DAV: propstat"`
}

type webDAVPropstat struct {
	Properties webDAVProperties `xml:"DAV: prop"`
	Status     string           `xml:"DAV: status"`
}

type webDAVProperties struct {
	ResourceType  webDAVResourceType  `xml:"DAV: resourcetype"`
	CreationDate  string              `xml:"DAV: creationdate"`
	LastModified  string              `xml:"DAV: getlastmodified"`
	ETag          string              `xml:"DAV: getetag"`
	SupportedLock webDAVSupportedLock `xml:"DAV: supportedlock"`
	LockDiscovery *webDAVElement      `xml:"DAV: lockdiscovery"`
	ContentType   string              `xml:"DAV: getcontenttype"`
}

type webDAVResourceType struct {
	Collection *webDAVElement `xml:"DAV: collection"`
}

type webDAVSupportedLock struct {
	Entries []webDAVLockEntry `xml:"DAV: lockentry"`
}

type webDAVLockEntry struct {
	Scope webDAVLockScope `xml:"DAV: lockscope"`
	Type  webDAVLockType  `xml:"DAV: locktype"`
}

type webDAVLockScope struct {
	Exclusive *webDAVElement `xml:"DAV: exclusive"`
	Shared    *webDAVElement `xml:"DAV: shared"`
}

type webDAVLockType struct {
	Write *webDAVElement `xml:"DAV: write"`
}

type webDAVElement struct {
	XMLName xml.Name
}

func TestProtocolCorpusWebDAVFullCapture(t *testing.T) {
	captureData := readProtocolCorpusFile(t, ".", webDAVCorpusCapture)
	reader, err := pcapgo.NewNgReader(bytes.NewReader(captureData), pcapgo.NgReaderOptions{SkipUnknownVersion: true})
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())

	classCounts := make(map[string]int)
	methodCounts := make(map[string]int)
	statusCounts := make(map[string]int)
	streamMessageCounts := make(map[string]int)
	parsedFrames := make(map[int]webDAVParsedMessage)
	packetCount := 0

	for {
		frame, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		packetCount++

		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		if errorLayer := packet.ErrorLayer(); errorLayer != nil {
			t.Fatalf("frame %d decode failed: %v", packetCount, errorLayer.Error())
		}
		require.NotNilf(t, packet.NetworkLayer(), "frame %d has no network layer", packetCount)
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		require.NotNilf(t, tcpLayer, "frame %d has no TCP layer", packetCount)
		tcp := tcpLayer.(*layers.TCP)

		if len(tcp.Payload) == 0 {
			class, ok := webDAVControlClass(tcp)
			require.Truef(t, ok, "frame %d has an unclassified no-data TCP flag combination", packetCount)
			classCounts[class]++
			continue
		}

		node, value := webDAVParseHTTPExact(t, packetCount, tcp.Payload)
		request := value.Child("HTTP Request")
		response := value.Child("HTTP Response")
		require.NotEqualf(t, request != nil, response != nil, "frame %d must be exactly one HTTP message kind", packetCount)
		if request != nil {
			method := strVal(t, mustChild(t, request, "Method"))
			methodCounts[method]++
			classCounts["http-request"]++
		} else {
			status := strVal(t, mustChild(t, response, "Status"))
			statusCounts[status]++
			classCounts["http-response"]++
		}

		streamMessageCounts[webDAVStreamKey(packet, tcp)]++
		parsedFrames[packetCount] = webDAVParsedMessage{
			payload: append([]byte(nil), tcp.Payload...),
			value:   value,
		}
		require.NotNil(t, node)
	}

	require.Equal(t, 92, packetCount)
	require.Equal(t, map[string]int{
		"tcp-syn":         8,
		"tcp-syn-ack":     8,
		"tcp-ack":         28,
		"tcp-fin-ack":     15,
		"tcp-psh-fin-ack": 1,
		"http-request":    16,
		"http-response":   16,
	}, classCounts)
	require.Equal(t, map[string]int{
		"COPY":      2,
		"LOCK":      2,
		"MKCOL":     2,
		"MOVE":      2,
		"PROPFIND":  4,
		"PROPPATCH": 2,
		"UNLOCK":    2,
	}, methodCounts)
	require.Equal(t, map[string]int{
		"207": 1,
		"301": 1,
		"400": 5,
		"401": 7,
		"403": 1,
		"405": 1,
	}, statusCounts)
	require.Len(t, parsedFrames, 32)
	require.Len(t, streamMessageCounts, 8)
	streamKeys := make([]string, 0, len(streamMessageCounts))
	for stream := range streamMessageCounts {
		streamKeys = append(streamKeys, stream)
	}
	sort.Strings(streamKeys)
	for _, stream := range streamKeys {
		require.Equalf(t, 4, streamMessageCounts[stream], "stream %s message count", stream)
	}

	webDAVRequireRequest(t, parsedFrames[4], "PROPFIND", "/webdav", "HTTP/1.1", map[string]string{
		"Content-Length": "0",
		"Depth":          "0",
		"Host":           "104.156.149.6",
	})
	webDAVRequireRequest(t, parsedFrames[7], "PROPFIND", "/webdav/", "HTTP/1.1", map[string]string{
		"Content-Length": "0",
		"Depth":          "0",
		"Host":           "104.156.149.6",
	})
	webDAVRequireMultistatus(t, parsedFrames[9])
}

func webDAVParseHTTPExact(t *testing.T, frameNumber int, input []byte) (*base.Node, *base.NodeValue) {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(input)
	node, err := protocolCorpusParseRule(reader, protocolCorpusParseContract{
		RuleFile:  "application-layer/http.yaml",
		EntryNode: "HTTP",
		Layer:     "L7",
	})
	require.NoErrorf(t, err, "frame %d HTTP parse", frameNumber)
	require.Zero(t, reader.Len(), "frame %d parser left bytes unread", frameNumber)
	terminals, coverageErr := protocolCorpusTerminalCoverage(node, input)
	require.NoErrorf(t, coverageErr, "frame %d terminal coverage", frameNumber)
	require.Positivef(t, terminals, "frame %d produced no terminal fields", frameNumber)
	value, err := node.Result()
	require.NoErrorf(t, err, "frame %d structured result", frameNumber)
	require.NotNilf(t, value, "frame %d structured result is nil", frameNumber)
	return node, value
}

func webDAVControlClass(tcp *layers.TCP) (string, bool) {
	if tcp.RST || tcp.URG || tcp.ECE || tcp.CWR {
		return "", false
	}
	switch {
	case tcp.SYN && !tcp.ACK && !tcp.FIN && !tcp.PSH:
		return "tcp-syn", true
	case tcp.SYN && tcp.ACK && !tcp.FIN && !tcp.PSH:
		return "tcp-syn-ack", true
	case tcp.ACK && !tcp.SYN && !tcp.FIN && !tcp.PSH:
		return "tcp-ack", true
	case tcp.FIN && tcp.ACK && !tcp.SYN && !tcp.PSH:
		return "tcp-fin-ack", true
	case tcp.PSH && tcp.FIN && tcp.ACK && !tcp.SYN:
		return "tcp-psh-fin-ack", true
	default:
		return "", false
	}
}

func webDAVStreamKey(packet gopacket.Packet, tcp *layers.TCP) string {
	networkFlow := packet.NetworkLayer().NetworkFlow()
	left := fmt.Sprintf("%s:%d", networkFlow.Src(), uint16(tcp.SrcPort))
	right := fmt.Sprintf("%s:%d", networkFlow.Dst(), uint16(tcp.DstPort))
	if right < left {
		left, right = right, left
	}
	return left + "<->" + right
}

func webDAVRequireRequest(t *testing.T, message webDAVParsedMessage, method, path, version string, expectedHeaders map[string]string) {
	t.Helper()
	require.NotEmpty(t, message.payload)
	request := mustChild(t, message.value, "HTTP Request")
	require.Equal(t, method, strVal(t, mustChild(t, request, "Method")))
	require.Equal(t, path, strVal(t, mustChild(t, request, "Path")))
	require.Equal(t, version, strVal(t, mustChild(t, request, "Version")))
	headers := webDAVHeaders(t, request)
	for name, expected := range expectedHeaders {
		require.Equalf(t, expected, headers[name], "header %s", name)
	}
}

func webDAVRequireMultistatus(t *testing.T, message webDAVParsedMessage) {
	t.Helper()
	require.NotEmpty(t, message.payload)
	response := mustChild(t, message.value, "HTTP Response")
	require.Equal(t, "HTTP/1.1", strVal(t, mustChild(t, response, "Version")))
	require.Equal(t, "207", strVal(t, mustChild(t, response, "Status")))
	require.Equal(t, "Multi-Status", strVal(t, mustChild(t, response, "Message")))
	headers := webDAVHeaders(t, response)
	require.Equal(t, "838", headers["Content-Length"])
	require.Equal(t, "text/xml; charset=\"utf-8\"", headers["Content-Type"])

	body := []byte(strVal(t, mustChild(t, response, "Body", "Octets")))
	require.Len(t, body, 838)
	var multistatus webDAVMultistatus
	require.NoError(t, xml.Unmarshal(body, &multistatus))
	require.Equal(t, xml.Name{Space: "DAV:", Local: "multistatus"}, multistatus.XMLName)
	require.Len(t, multistatus.Responses, 1)
	davResponse := multistatus.Responses[0]
	require.Equal(t, "/webdav/", strings.TrimSpace(davResponse.Href))
	require.Len(t, davResponse.Propstats, 1)
	propstat := davResponse.Propstats[0]
	require.Equal(t, "HTTP/1.1 200 OK", strings.TrimSpace(propstat.Status))
	require.NotNil(t, propstat.Properties.ResourceType.Collection)
	require.Equal(t, "2023-02-21T17:14:50Z", strings.TrimSpace(propstat.Properties.CreationDate))
	require.Equal(t, "Tue, 21 Feb 2023 17:14:50 GMT", strings.TrimSpace(propstat.Properties.LastModified))
	require.Equal(t, "\"9000-5f538ead02c47\"", strings.TrimSpace(propstat.Properties.ETag))
	require.Equal(t, "httpd/unix-directory", strings.TrimSpace(propstat.Properties.ContentType))
	require.NotNil(t, propstat.Properties.LockDiscovery)
	require.Len(t, propstat.Properties.SupportedLock.Entries, 2)
	require.NotNil(t, propstat.Properties.SupportedLock.Entries[0].Scope.Exclusive)
	require.Nil(t, propstat.Properties.SupportedLock.Entries[0].Scope.Shared)
	require.NotNil(t, propstat.Properties.SupportedLock.Entries[0].Type.Write)
	require.Nil(t, propstat.Properties.SupportedLock.Entries[1].Scope.Exclusive)
	require.NotNil(t, propstat.Properties.SupportedLock.Entries[1].Scope.Shared)
	require.NotNil(t, propstat.Properties.SupportedLock.Entries[1].Type.Write)
}

func webDAVHeaders(t *testing.T, message *base.NodeValue) map[string]string {
	t.Helper()
	headers := mustChild(t, message, "Headers")
	result := make(map[string]string, len(headers.Children()))
	for _, item := range headers.Children() {
		line := strVal(t, item)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		require.Lenf(t, parts, 2, "malformed HTTP header %q", line)
		name := strings.TrimSpace(parts[0])
		_, duplicate := result[name]
		require.Falsef(t, duplicate, "duplicate HTTP header %q", name)
		result[name] = strings.TrimSpace(parts[1])
	}
	return result
}
