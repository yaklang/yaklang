package bin_parser

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

const (
	soapCorpusCapturePath = "testdata/protocol-corpus/captures/ndpi/ndpi-soap.pcap"
	soap11EnvelopeNS      = "http://schemas.xmlsoap.org/soap/envelope/"
	soapCorpusDMSNS       = "http://schemas.microsoft.com/windowsmetadata/services/2007/09/18/dms"
	soapCorpusAction      = "http://schemas.microsoft.com/windowsmetadata/services/2007/09/18/dms/DeviceMetadataService/GetDeviceMetadata"
)

type soapCorpusTCPSegment struct {
	frame   int
	seq     uint32
	payload []byte
}

type soapCorpusHTTPRequest struct {
	method  string
	target  string
	version string
	headers map[string]string
	body    []byte
	action  string
	doc     soapCorpusDocument
}

type soapCorpusHTTPResponse struct {
	version string
	status  int
	reason  string
	headers map[string]string
	body    []byte
}

type soapCorpusDocument struct {
	envelope xml.Name
	header   xml.Name
	body     xml.Name
	request  xml.Name
}

func TestProtocolCorpusSOAPFullCapture(t *testing.T) {
	capture, err := os.Open(soapCorpusCapturePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, capture.Close()) })

	reader, err := pcapgo.NewReader(capture)
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())

	frameCounts := map[string]int{}
	segments := map[string][]soapCorpusTCPSegment{}
	packetCount := 0
	for {
		packetData, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		packetCount++

		packet := gopacket.NewPacket(packetData, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		if errorLayer := packet.ErrorLayer(); errorLayer != nil {
			t.Fatalf("frame %d decode failed: %v", packetCount, errorLayer.Error())
		}
		ipv4Layer := packet.Layer(layers.LayerTypeIPv4)
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		require.NotNilf(t, ipv4Layer, "frame %d must be IPv4", packetCount)
		require.NotNilf(t, tcpLayer, "frame %d must be TCP", packetCount)
		ipv4 := ipv4Layer.(*layers.IPv4)
		tcp := tcpLayer.(*layers.TCP)

		stream, direction, ok := soapCorpusClassifyFlow(ipv4.SrcIP.String(), uint16(tcp.SrcPort), ipv4.DstIP.String(), uint16(tcp.DstPort))
		require.Truef(t, ok, "frame %d has an unclassified flow %s:%d -> %s:%d", packetCount, ipv4.SrcIP, tcp.SrcPort, ipv4.DstIP, tcp.DstPort)
		frameCounts[stream]++
		if len(tcp.Payload) > 0 {
			key := stream + "/" + direction
			segments[key] = append(segments[key], soapCorpusTCPSegment{
				frame:   packetCount,
				seq:     tcp.Seq,
				payload: append([]byte(nil), tcp.Payload...),
			})
		}
	}

	require.Equal(t, 20, packetCount)
	require.Equal(t, map[string]int{"stream0": 14, "stream1": 1, "stream2": 5}, frameCounts,
		"every frame must belong to exactly one known stream")
	require.Len(t, segments["stream0/client-to-server"], 3)
	require.Len(t, segments["stream0/server-to-client"], 1)
	require.Len(t, segments["stream1/client-to-server"], 1)
	require.Len(t, segments["stream2/client-to-server"], 2)
	require.Len(t, segments["stream2/server-to-client"], 2)

	requestBytes, err := soapCorpusReassembleTCP(segments["stream0/client-to-server"])
	require.NoError(t, err)
	responseBytes, err := soapCorpusReassembleTCP(segments["stream0/server-to-client"])
	require.NoError(t, err)
	require.Len(t, requestBytes, 3958)
	require.Len(t, responseBytes, 296)

	request, err := soapCorpusParseRequest(requestBytes)
	require.NoError(t, err)
	require.Equal(t, "POST", request.method)
	require.Equal(t, "/fwlink/?LinkID=252669&clcid=0x409", request.target)
	require.Equal(t, "HTTP/1.1", request.version)
	require.Equal(t, `text/xml; charset="UTF-16LE"`, request.headers["content-type"])
	require.Equal(t, `"`+soapCorpusAction+`"`, request.headers["soapaction"])
	require.Equal(t, soapCorpusAction, request.action)
	require.Equal(t, "3612", request.headers["content-length"])
	require.Len(t, request.body, 3612)
	require.True(t, bytes.HasPrefix(request.body, []byte{0xff, 0xfe}), "SOAP body must retain its UTF-16LE BOM")
	require.Equal(t, xml.Name{Space: soap11EnvelopeNS, Local: "Envelope"}, request.doc.envelope)
	require.Equal(t, xml.Name{Space: soap11EnvelopeNS, Local: "Header"}, request.doc.header)
	require.Equal(t, xml.Name{Space: soap11EnvelopeNS, Local: "Body"}, request.doc.body)
	require.Equal(t, xml.Name{Space: soapCorpusDMSNS, Local: "DeviceMetadataBatchRequest"}, request.doc.request)

	response, err := soapCorpusParseResponse(responseBytes)
	require.NoError(t, err)
	require.Equal(t, "HTTP/1.1", response.version)
	require.Equal(t, 302, response.status)
	require.Equal(t, "Moved Temporarily", response.reason)
	require.Equal(t, "0", response.headers["content-length"])
	require.Equal(t, "http://dmd.metaservices.microsoft.com/metadata.svc", response.headers["location"])
	require.Empty(t, response.body)

	truncated, err := soapCorpusReassembleTCP(segments["stream1/client-to-server"])
	require.NoError(t, err)
	require.Len(t, truncated, 1452)
	require.Equal(t, requestBytes[:len(truncated)], truncated, "stream1 must be the stable prefix of the complete request")
	_, err = soapCorpusParseRequest(truncated)
	require.ErrorContains(t, err, "Content-Length 3612 does not match 1106 body bytes")

	stream2Client, err := soapCorpusReassembleTCP(segments["stream2/client-to-server"])
	require.NoError(t, err)
	stream2Server, err := soapCorpusReassembleTCP(segments["stream2/server-to-client"])
	require.NoError(t, err)
	require.Len(t, stream2Client, 1341)
	require.Len(t, stream2Server, 2301)
	soapCorpusRequireZMessage(t, stream2Client, "ZBBP.1.ResolveKID", 114)
	soapCorpusRequireZMessage(t, stream2Server, "ZBBP.1.DMSConfig", 1074)
	for direction, stream := range map[string][]byte{
		"client-to-server": stream2Client,
		"server-to-client": stream2Server,
	} {
		require.NotContains(t, string(stream), soap11EnvelopeNS, "%s stream2 direction must not be SOAP", direction)
		_, parseErr := soapCorpusParseRequest(stream)
		require.ErrorContains(t, parseErr, "HTTP header terminator", "%s stream2 direction must be rejected as non-SOAP", direction)
	}
}

func TestProtocolCorpusSOAPRejectsMalformedMessages(t *testing.T) {
	validBody := soapCorpusEncodeUTF16LE(`<?xml version="1.0" encoding="UTF-16"?>` +
		`<s:Envelope xmlns:s="` + soap11EnvelopeNS + `">` +
		`<s:Header><h:marker xmlns:h="urn:header"/></s:Header>` +
		`<s:Body><DeviceMetadataBatchRequest xmlns="` + soapCorpusDMSNS + `"/></s:Body>` +
		`</s:Envelope>`)
	valid := soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, validBody, nil)
	_, err := soapCorpusParseRequest(valid)
	require.NoError(t, err)

	wrongNamespaceBody := soapCorpusEncodeUTF16LE(`<?xml version="1.0" encoding="UTF-16"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Header/><s:Body><DeviceMetadataBatchRequest xmlns="` + soapCorpusDMSNS + `"/></s:Body></s:Envelope>`)
	missingHeaderBody := soapCorpusEncodeUTF16LE(`<?xml version="1.0" encoding="UTF-16"?>` +
		`<s:Envelope xmlns:s="` + soap11EnvelopeNS + `">` +
		`<s:Body><DeviceMetadataBatchRequest xmlns="` + soapCorpusDMSNS + `"/></s:Body></s:Envelope>`)
	missingRequestBody := soapCorpusEncodeUTF16LE(`<?xml version="1.0" encoding="UTF-16"?>` +
		`<s:Envelope xmlns:s="` + soap11EnvelopeNS + `"><s:Header/><s:Body><Other xmlns="urn:other"/></s:Body></s:Envelope>`)
	malformedXMLBody := soapCorpusEncodeUTF16LE(`<?xml version="1.0" encoding="UTF-16"?><s:Envelope xmlns:s="` + soap11EnvelopeNS + `"><s:Header/><s:Body>`)
	extraRootBody := soapCorpusEncodeUTF16LE(`<?xml version="1.0" encoding="UTF-16"?>` +
		`<s:Envelope xmlns:s="` + soap11EnvelopeNS + `"><s:Header/><s:Body><DeviceMetadataBatchRequest xmlns="` + soapCorpusDMSNS + `"/></s:Body></s:Envelope><extra/>`)

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "truncated declared body", data: valid[:len(valid)-2], want: "does not match"},
		{name: "bytes after declared body", data: append(append([]byte(nil), valid...), 0, 0), want: "does not match"},
		{name: "LF-only header framing", data: bytes.ReplaceAll(valid, []byte("\r\n"), []byte("\n")), want: "HTTP header terminator"},
		{name: "duplicate content length", data: bytes.Replace(valid, []byte("\r\n\r\n"), []byte("\r\nContent-Length: 1\r\n\r\n"), 1), want: "duplicate HTTP header"},
		{name: "invalid content length", data: bytes.Replace(valid, []byte(fmt.Sprintf("Content-Length: %d", len(validBody))), []byte("Content-Length: 12x"), 1), want: "invalid Content-Length"},
		{name: "wrong method", data: soapCorpusBuildRequest("GET", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, validBody, nil), want: "method"},
		{name: "absolute request target", data: soapCorpusBuildRequest("POST", "http://example.test/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, validBody, nil), want: "origin-form"},
		{name: "wrong media type", data: soapCorpusBuildRequest("POST", "/metadata", `application/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, validBody, nil), want: "media type"},
		{name: "wrong charset", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-8"`, `"`+soapCorpusAction+`"`, validBody, nil), want: "charset"},
		{name: "unquoted action", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, soapCorpusAction, validBody, nil), want: "quoted URI"},
		{name: "missing action", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, "", validBody, nil), want: "SOAPAction"},
		{name: "missing UTF-16LE BOM", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, validBody[2:], nil), want: "BOM"},
		{name: "odd UTF-16LE body", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, append(append([]byte(nil), validBody...), 0), nil), want: "even byte length"},
		{name: "unpaired UTF-16 surrogate", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, []byte{0xff, 0xfe, 0x00, 0xd8}, nil), want: "unpaired high surrogate"},
		{name: "SOAP 1.2 namespace", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, wrongNamespaceBody, nil), want: "SOAP 1.1 Envelope"},
		{name: "missing Header", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, missingHeaderBody, nil), want: "Header then Body"},
		{name: "missing request element", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, missingRequestBody, nil), want: "DeviceMetadataBatchRequest"},
		{name: "malformed XML", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, malformedXMLBody, nil), want: "XML"},
		{name: "second root element", data: soapCorpusBuildRequest("POST", "/metadata", `text/xml; charset="UTF-16LE"`, `"`+soapCorpusAction+`"`, extraRootBody, nil), want: "content after root"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			_, parseErr := soapCorpusParseRequest(test.data)
			require.ErrorContains(t, parseErr, test.want)
		})
	}
}

func soapCorpusClassifyFlow(srcIP string, srcPort uint16, dstIP string, dstPort uint16) (string, string, bool) {
	switch {
	case srcIP == "192.168.2.100" && srcPort == 50100 && dstIP == "23.2.213.165" && dstPort == 80:
		return "stream0", "client-to-server", true
	case srcIP == "23.2.213.165" && srcPort == 80 && dstIP == "192.168.2.100" && dstPort == 50100:
		return "stream0", "server-to-client", true
	case srcIP == "192.168.2.100" && srcPort == 50100 && dstIP == "23.2.213.165" && dstPort == 4176:
		return "stream1", "client-to-server", true
	case srcIP == "23.2.213.165" && srcPort == 4176 && dstIP == "192.168.2.100" && dstPort == 50100:
		return "stream1", "server-to-client", true
	case srcIP == "85.154.114.113" && srcPort == 56028 && dstIP == "185.32.192.30" && dstPort == 80:
		return "stream2", "client-to-server", true
	case srcIP == "185.32.192.30" && srcPort == 80 && dstIP == "85.154.114.113" && dstPort == 56028:
		return "stream2", "server-to-client", true
	default:
		return "", "", false
	}
}

func soapCorpusReassembleTCP(segments []soapCorpusTCPSegment) ([]byte, error) {
	if len(segments) == 0 {
		return nil, fmt.Errorf("TCP direction has no data segments")
	}
	ordered := append([]soapCorpusTCPSegment(nil), segments...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].seq != ordered[j].seq {
			return ordered[i].seq < ordered[j].seq
		}
		return len(ordered[i].payload) > len(ordered[j].payload)
	})
	stream := append([]byte(nil), ordered[0].payload...)
	next := uint64(ordered[0].seq) + uint64(len(ordered[0].payload))
	for _, segment := range ordered[1:] {
		start := uint64(segment.seq)
		end := start + uint64(len(segment.payload))
		if start > next {
			return nil, fmt.Errorf("TCP gap before frame %d: next sequence %d, got %d", segment.frame, next, start)
		}
		overlap := next - start
		compared := overlap
		if compared > uint64(len(segment.payload)) {
			compared = uint64(len(segment.payload))
		}
		streamOffset := uint64(len(stream)) - overlap
		if !bytes.Equal(stream[streamOffset:streamOffset+compared], segment.payload[:compared]) {
			return nil, fmt.Errorf("TCP overlap differs in frame %d", segment.frame)
		}
		if end <= next {
			continue
		}
		stream = append(stream, segment.payload[overlap:]...)
		next = end
	}
	return stream, nil
}

func soapCorpusParseRequest(input []byte) (soapCorpusHTTPRequest, error) {
	startLine, headers, body, err := soapCorpusParseHTTPMessage(input)
	if err != nil {
		return soapCorpusHTTPRequest{}, err
	}
	parts := strings.Split(startLine, " ")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return soapCorpusHTTPRequest{}, fmt.Errorf("invalid HTTP request line %q", startLine)
	}
	if parts[0] != "POST" {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAP request method must be POST, got %q", parts[0])
	}
	if !strings.HasPrefix(parts[1], "/") || strings.HasPrefix(parts[1], "//") {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAP request target must use origin-form, got %q", parts[1])
	}
	if parts[2] != "HTTP/1.1" {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAP request version must be HTTP/1.1, got %q", parts[2])
	}
	if headers["host"] == "" {
		return soapCorpusHTTPRequest{}, fmt.Errorf("HTTP/1.1 SOAP request is missing Host")
	}
	if _, present := headers["transfer-encoding"]; present {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAP fixture does not permit Transfer-Encoding")
	}
	contentLength, err := soapCorpusRequiredContentLength(headers)
	if err != nil {
		return soapCorpusHTTPRequest{}, err
	}
	if contentLength != len(body) {
		return soapCorpusHTTPRequest{}, fmt.Errorf("declared Content-Length %d does not match %d body bytes", contentLength, len(body))
	}
	contentType, ok := headers["content-type"]
	if !ok {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAP request is missing Content-Type")
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return soapCorpusHTTPRequest{}, fmt.Errorf("invalid SOAP Content-Type: %w", err)
	}
	if !strings.EqualFold(mediaType, "text/xml") {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAP 1.1 media type must be text/xml, got %q", mediaType)
	}
	if !strings.EqualFold(params["charset"], "UTF-16LE") {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAP fixture charset must be UTF-16LE, got %q", params["charset"])
	}
	actionHeader, ok := headers["soapaction"]
	if !ok || actionHeader == "" {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAP request is missing SOAPAction")
	}
	if len(actionHeader) < 2 || actionHeader[0] != '"' || actionHeader[len(actionHeader)-1] != '"' {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAPAction must be a quoted URI")
	}
	action := actionHeader[1 : len(actionHeader)-1]
	parsedAction, err := url.ParseRequestURI(action)
	if err != nil || parsedAction.Scheme == "" || parsedAction.Host == "" {
		return soapCorpusHTTPRequest{}, fmt.Errorf("SOAPAction must be a quoted absolute URI")
	}
	doc, err := soapCorpusParseUTF16Document(body)
	if err != nil {
		return soapCorpusHTTPRequest{}, err
	}
	return soapCorpusHTTPRequest{
		method: parts[0], target: parts[1], version: parts[2], headers: headers,
		body: append([]byte(nil), body...), action: action, doc: doc,
	}, nil
}

func soapCorpusParseResponse(input []byte) (soapCorpusHTTPResponse, error) {
	startLine, headers, body, err := soapCorpusParseHTTPMessage(input)
	if err != nil {
		return soapCorpusHTTPResponse{}, err
	}
	parts := strings.SplitN(startLine, " ", 3)
	if len(parts) != 3 || parts[0] != "HTTP/1.1" || len(parts[1]) != 3 || parts[2] == "" {
		return soapCorpusHTTPResponse{}, fmt.Errorf("invalid HTTP response line %q", startLine)
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil || status < 100 || status > 999 {
		return soapCorpusHTTPResponse{}, fmt.Errorf("invalid HTTP response status %q", parts[1])
	}
	contentLength, err := soapCorpusRequiredContentLength(headers)
	if err != nil {
		return soapCorpusHTTPResponse{}, err
	}
	if contentLength != len(body) {
		return soapCorpusHTTPResponse{}, fmt.Errorf("declared Content-Length %d does not match %d body bytes", contentLength, len(body))
	}
	return soapCorpusHTTPResponse{
		version: parts[0], status: status, reason: parts[2], headers: headers, body: append([]byte(nil), body...),
	}, nil
}

func soapCorpusParseHTTPMessage(input []byte) (string, map[string]string, []byte, error) {
	separator := []byte("\r\n\r\n")
	separatorAt := bytes.Index(input, separator)
	if separatorAt < 0 {
		return "", nil, nil, fmt.Errorf("missing HTTP header terminator")
	}
	headerBlock := input[:separatorAt]
	if bytes.IndexByte(headerBlock, 0) >= 0 {
		return "", nil, nil, fmt.Errorf("HTTP header contains NUL")
	}
	for i, b := range headerBlock {
		switch b {
		case '\r':
			if i+1 >= len(headerBlock) || headerBlock[i+1] != '\n' {
				return "", nil, nil, fmt.Errorf("HTTP header contains a bare carriage return")
			}
		case '\n':
			if i == 0 || headerBlock[i-1] != '\r' {
				return "", nil, nil, fmt.Errorf("HTTP header contains a bare line feed")
			}
		}
	}
	lines := strings.Split(string(headerBlock), "\r\n")
	if len(lines) < 2 || lines[0] == "" {
		return "", nil, nil, fmt.Errorf("HTTP message has no header fields")
	}
	headers := make(map[string]string, len(lines)-1)
	for _, line := range lines[1:] {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			return "", nil, nil, fmt.Errorf("invalid folded or empty HTTP header line")
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 || !soapCorpusHTTPToken(line[:colon]) {
			return "", nil, nil, fmt.Errorf("invalid HTTP header line %q", line)
		}
		name := strings.ToLower(line[:colon])
		if _, duplicate := headers[name]; duplicate {
			return "", nil, nil, fmt.Errorf("duplicate HTTP header %q", line[:colon])
		}
		value := strings.Trim(line[colon+1:], " \t")
		for _, r := range value {
			if r < 0x20 && r != '\t' || r == 0x7f {
				return "", nil, nil, fmt.Errorf("HTTP header %q contains a control byte", line[:colon])
			}
		}
		headers[name] = value
	}
	return lines[0], headers, input[separatorAt+len(separator):], nil
}

func soapCorpusHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		if !strings.ContainsRune("!#$%&'*+-.^_`|~", r) {
			return false
		}
	}
	return true
}

func soapCorpusRequiredContentLength(headers map[string]string) (int, error) {
	value, ok := headers["content-length"]
	if !ok || value == "" {
		return 0, fmt.Errorf("HTTP message is missing Content-Length")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid Content-Length %q", value)
		}
	}
	length, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return 0, fmt.Errorf("invalid Content-Length %q: %w", value, err)
	}
	return int(length), nil
}

func soapCorpusParseUTF16Document(body []byte) (soapCorpusDocument, error) {
	text, err := soapCorpusDecodeUTF16LE(body)
	if err != nil {
		return soapCorpusDocument{}, err
	}
	if !strings.HasPrefix(text, "<?xml") || !strings.Contains(text[:min(len(text), 100)], `encoding="UTF-16"`) {
		return soapCorpusDocument{}, fmt.Errorf("SOAP XML declaration must specify UTF-16")
	}

	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = true
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		if !strings.EqualFold(label, "UTF-16") {
			return nil, fmt.Errorf("unsupported XML encoding %q", label)
		}
		// The byte-level UTF-16LE decoder above has already normalized the
		// document to Go's UTF-8 strings while retaining the declaration.
		return input, nil
	}
	var (
		doc              soapCorpusDocument
		stack            []xml.Name
		envelopeChildren []xml.Name
		bodyChildren     []xml.Name
		rootClosed       bool
	)
	for {
		token, tokenErr := decoder.Token()
		if errors.Is(tokenErr, io.EOF) {
			break
		}
		if tokenErr != nil {
			return soapCorpusDocument{}, fmt.Errorf("invalid SOAP XML: %w", tokenErr)
		}
		switch token := token.(type) {
		case xml.StartElement:
			if rootClosed {
				return soapCorpusDocument{}, fmt.Errorf("SOAP XML has content after root element")
			}
			if len(stack) == 0 {
				if doc.envelope.Local != "" {
					return soapCorpusDocument{}, fmt.Errorf("SOAP XML has multiple root elements")
				}
				doc.envelope = token.Name
			} else if len(stack) == 1 {
				envelopeChildren = append(envelopeChildren, token.Name)
			} else if len(stack) == 2 && stack[1] == (xml.Name{Space: soap11EnvelopeNS, Local: "Body"}) {
				bodyChildren = append(bodyChildren, token.Name)
			}
			stack = append(stack, token.Name)
		case xml.EndElement:
			if len(stack) == 0 {
				return soapCorpusDocument{}, fmt.Errorf("invalid SOAP XML closing element")
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				rootClosed = true
			}
		case xml.CharData:
			if len(stack) == 0 && strings.TrimSpace(string(token)) != "" {
				return soapCorpusDocument{}, fmt.Errorf("SOAP XML has content after root element")
			}
		}
	}
	if len(stack) != 0 || !rootClosed {
		return soapCorpusDocument{}, fmt.Errorf("invalid SOAP XML: unclosed root element")
	}
	if doc.envelope != (xml.Name{Space: soap11EnvelopeNS, Local: "Envelope"}) {
		return soapCorpusDocument{}, fmt.Errorf("document root is not a SOAP 1.1 Envelope: %s %s", doc.envelope.Space, doc.envelope.Local)
	}
	if len(envelopeChildren) != 2 || envelopeChildren[0] != (xml.Name{Space: soap11EnvelopeNS, Local: "Header"}) || envelopeChildren[1] != (xml.Name{Space: soap11EnvelopeNS, Local: "Body"}) {
		return soapCorpusDocument{}, fmt.Errorf("SOAP Envelope must contain Header then Body exactly once")
	}
	if len(bodyChildren) != 1 || bodyChildren[0] != (xml.Name{Space: soapCorpusDMSNS, Local: "DeviceMetadataBatchRequest"}) {
		return soapCorpusDocument{}, fmt.Errorf("SOAP Body must contain exactly one DeviceMetadataBatchRequest")
	}
	doc.header = envelopeChildren[0]
	doc.body = envelopeChildren[1]
	doc.request = bodyChildren[0]
	return doc, nil
}

func soapCorpusDecodeUTF16LE(body []byte) (string, error) {
	if len(body) < 2 || body[0] != 0xff || body[1] != 0xfe {
		return "", fmt.Errorf("SOAP body is missing the UTF-16LE BOM")
	}
	if len(body)%2 != 0 {
		return "", fmt.Errorf("UTF-16LE body must have an even byte length")
	}
	var decoded strings.Builder
	decoded.Grow((len(body) - 2) / 2)
	for offset := 2; offset < len(body); offset += 2 {
		code := uint16(body[offset]) | uint16(body[offset+1])<<8
		switch {
		case code >= 0xd800 && code <= 0xdbff:
			if offset+3 >= len(body) {
				return "", fmt.Errorf("UTF-16LE body has an unpaired high surrogate at byte %d", offset)
			}
			low := uint16(body[offset+2]) | uint16(body[offset+3])<<8
			if low < 0xdc00 || low > 0xdfff {
				return "", fmt.Errorf("UTF-16LE body has an unpaired high surrogate at byte %d", offset)
			}
			decoded.WriteRune(utf16.DecodeRune(rune(code), rune(low)))
			offset += 2
		case code >= 0xdc00 && code <= 0xdfff:
			return "", fmt.Errorf("UTF-16LE body has an unpaired low surrogate at byte %d", offset)
		default:
			decoded.WriteRune(rune(code))
		}
	}
	return decoded.String(), nil
}

func soapCorpusRequireZMessage(t *testing.T, stream []byte, target string, trailerLength int) {
	t.Helper()
	endMarker := []byte("</ZMessage>\r\n")
	end := bytes.Index(stream, endMarker)
	require.NotEqual(t, -1, end, "non-SOAP stream is missing its ZMessage terminator")
	end += len(endMarker)
	xmlPrefix := stream[:end]
	trailer := stream[end:]
	require.Len(t, trailer, trailerLength)

	var root struct {
		XMLName           xml.Name
		Target            string `xml:"target,attr"`
		ParamHeaderLength int    `xml:"paramheaderlength,attr"`
	}
	require.NoError(t, xml.Unmarshal(xmlPrefix, &root))
	require.Equal(t, xml.Name{Local: "ZMessage"}, root.XMLName)
	require.Equal(t, target, root.Target)
	require.Equal(t, trailerLength, root.ParamHeaderLength)
}

func soapCorpusEncodeUTF16LE(text string) []byte {
	result := []byte{0xff, 0xfe}
	for _, code := range utf16.Encode([]rune(text)) {
		result = append(result, byte(code), byte(code>>8))
	}
	return result
}

func soapCorpusBuildRequest(method, target, contentType, action string, body []byte, extraHeaders []string) []byte {
	lines := []string{
		method + " " + target + " HTTP/1.1",
		"Host: example.test",
		"Content-Type: " + contentType,
		fmt.Sprintf("Content-Length: %d", len(body)),
	}
	if action != "" {
		lines = append(lines, "SOAPAction: "+action)
	}
	lines = append(lines, extraHeaders...)
	result := []byte(strings.Join(lines, "\r\n") + "\r\n\r\n")
	return append(result, body...)
}
