package match

import (
	"bufio"
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type HttpFlow struct {
	ReqInstance *http.Request
	Src         string
	SrcPort     int
	Dst         string
	DstPort     int
	Req         []byte
	Rsp         []byte

	TrafficFlow *pcaputil.TrafficFlow
	packets     []gopacket.Packet
	parseOnce   *sync.Once
}

func (h *HttpFlow) ToRequestPacket() []gopacket.Packet {
	if h.parseOnce == nil {
		h.parseOnce = new(sync.Once)
	}
	h.parseOnce.Do(func() {
		h.packets = h.createRequestPacket()
	})
	return h.packets
}

func (h *HttpFlow) createRequestPacket() []gopacket.Packet {
	packets := make([]gopacket.Packet, 0, 2)

	// 如果 TrafficFlow 不为空，优先使用其中的网络信息
	if h.TrafficFlow != nil {
		h.Src = h.TrafficFlow.ClientConn.LocalIP().String()
		h.Dst = h.TrafficFlow.ClientConn.RemoteIP().String()
		h.SrcPort = h.TrafficFlow.ClientConn.LocalPort()
		h.DstPort = h.TrafficFlow.ClientConn.RemotePort()
	}
	if h.Src == "" {
		h.Src = utils.GetLocalIPAddress()
	}

	if h.Dst == "" {
		h.Dst = utils.GetRandomIPAddress()
	}

	if h.DstPort <= 0 {
		h.DstPort = 80
	}

	if h.SrcPort <= 0 {
		h.SrcPort = 10000 + rand.Intn(30000)
	}

	if len(h.Req) > 0 {
		bytes, err := pcapx.PacketBuilder(
			pcapx.WithEthernet_NextLayerType("ip"),
			pcapx.WithEthernet_SrcMac("00:00:00:00:00:00"),
			pcapx.WithEthernet_DstMac("00:00:00:00:00:00"),
			pcapx.WithIPv4_SrcIP(h.Src),
			pcapx.WithIPv4_DstIP(h.Dst),
			pcapx.WithTCP_SrcPort(h.SrcPort),
			pcapx.WithTCP_DstPort(h.DstPort),
			pcapx.WithPayload(h.Req),
		)
		if err != nil {
			log.Errorf("build packet failed: %v", err)
		}
		if len(bytes) > 0 {
			packet := gopacket.NewPacket(bytes, layers.LayerTypeEthernet, gopacket.NoCopy)
			if packet.ErrorLayer() == nil {
				packets = append(packets, packet)
			}
		}
	}

	if len(h.Rsp) > 0 {
		bytes, err := pcapx.PacketBuilder(
			pcapx.WithEthernet_NextLayerType("ip"),
			pcapx.WithEthernet_SrcMac("00:00:00:00:00:00"),
			pcapx.WithEthernet_DstMac("00:00:00:00:00:00"),
			pcapx.WithIPv4_SrcIP(h.Dst),
			pcapx.WithIPv4_DstIP(h.Src),
			pcapx.WithTCP_SrcPort(h.DstPort),
			pcapx.WithTCP_DstPort(h.SrcPort),
			pcapx.WithPayload(h.Rsp),
		)
		if err != nil {
			log.Errorf("build packet failed: %v", err)
		}
		if len(bytes) > 0 {
			packet := gopacket.NewPacket(bytes, layers.LayerTypeEthernet, gopacket.NoCopy)
			if packet.ErrorLayer() == nil {
				packets = append(packets, packet)
			}
		}
	}

	return packets
}

func httpParser(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	// Stream inspection passes reassembled bytes through overridePayload while
	// the real packet still supplies IP/TCP headers. Packet matching keeps the
	// legacy behavior of accepting header blocks without a trailing blank line.
	payload := c.overridePayload
	requireComplete := len(payload) > 0
	if len(payload) == 0 {
		if c.PK == nil || c.PK.TransportLayer() == nil {
			return nil
		}
		payload = c.PK.TransportLayer().LayerPayload()
	}
	provider := newHTTPBufferProviderFromPayload(c.PK, payload, requireComplete)
	if !c.Must(provider != nil) {
		return nil
	}

	// register buffer provider
	c.SetBufferProvider(provider.Get)
	c.Value["data"] = provider.Parsed()
	c.Value["isReq"] = provider.GetRequest() != nil

	return nil
}

func attachHTTPMatcher(c *matchContext) {
	if c.Rule.ContentRuleConfig.HTTPConfig != nil {
		c.Attach(httpReqMatcher)
	}
}

func httpReqMatcher(c *matchContext) error {
	if cf := c.Rule.ContentRuleConfig.HTTPConfig; cf != nil {
		isRequest, _ := c.Value["isReq"].(bool)
		if !c.Must(isRequest) {
			return nil
		}
		if cf.Uricontent != "" {
			log.Errorf("uricontent has been deprecated and not implemented yet")
		}
		if !c.Must(cf.Urilen.Match(len(c.GetBuffer(modifier.HTTPUri)))) {
			return nil
		}
	}
	return nil
}

func httpResMatcher(_ *matchContext) error {
	// 没有需要匹配的
	return nil
}

type httpProvider struct {
	PK gopacket.Packet

	// cache
	raw        []byte
	body       []byte
	bodyCached bool
	req        *http.Request
	res        *http.Response
}

// if success, return value not nil
func newHTTPBufferProvider(pk gopacket.Packet) *httpProvider {
	if pk == nil || pk.TransportLayer() == nil {
		return nil
	}
	return newHTTPBufferProviderFromPayload(pk, pk.TransportLayer().LayerPayload(), false)
}

func newHTTPBufferProviderFromPayload(pk gopacket.Packet, payload []byte, requireComplete bool) *httpProvider {
	if len(payload) == 0 {
		return nil
	}
	// Do not expose a partial HTTP stream message as matchable. Waiting for
	// the header block and a declared body prevents split request lines or
	// partial bodies from producing early or duplicate matches; packet
	// matching keeps the legacy tolerance.
	if requireComplete {
		if _, complete := HTTPMessageLength(payload); !complete {
			return nil
		}
	}
	a, b, c := lowhttp.GetHTTPPacketFirstLine(payload)
	if a == "" || b == "" || c == "" {
		return nil
	}
	if strings.HasPrefix(a, "HTTP/") {
		var res *http.Response
		var err error
		if requireComplete {
			// The bufio reader decodes a chunked response body; the legacy
			// packet parser intentionally keeps the raw wire body.
			res, err = lowhttp.ReadHTTPResponseFromBufioReader(bytes.NewReader(payload), &http.Request{Method: http.MethodGet})
		} else {
			res, err = lowhttp.ParseBytesToHTTPResponse(payload)
		}
		if err != nil {
			return nil
		}
		return &httpProvider{
			PK:  pk,
			res: res,
			raw: payload,
		}
	}
	var request *http.Request
	var err error
	if requireComplete {
		// The bufio reader decodes chunked bodies and stops at Content-Length.
		request, err = lowhttp.ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewReader(payload)))
	} else {
		request, err = lowhttp.ParseBytesToHttpRequest(payload)
	}
	if err != nil {
		return nil
	}
	return &httpProvider{
		PK:  pk,
		req: request,
		raw: payload,
	}
}

func rawHeaderValue(raw []byte, name string) []byte {
	headerBlock, _, _ := bytes.Cut(raw, []byte("\r\n\r\n"))
	lines := bytes.Split(headerBlock, []byte("\r\n"))
	for _, line := range lines[1:] {
		key, value, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			continue
		}
		if bytes.EqualFold(bytes.TrimSpace(key), []byte(name)) {
			return bytes.TrimSpace(value)
		}
	}
	return nil
}

// HTTPMessageLength returns the byte length of the first complete HTTP message
// in payload. The boolean is false while the header block or the declared body
// is still incomplete, allowing the engine to wait for more stream bytes.
func HTTPMessageLength(payload []byte) (int, bool) {
	headerEnd := bytes.Index(payload, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		return 0, false
	}
	headerLength := headerEnd + 4
	body := payload[headerLength:]
	contentLength := -1
	chunked := false
	lines := bytes.Split(payload[:headerEnd], []byte("\r\n"))
	request, http11 := false, false
	if len(lines) > 0 {
		fields := bytes.Fields(lines[0])
		if len(fields) >= 3 && !bytes.HasPrefix(fields[0], []byte("HTTP/")) {
			request = true
			http11 = bytes.Equal(fields[len(fields)-1], []byte("HTTP/1.1"))
		}
	}
	for _, line := range lines[1:] {
		name, value, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			continue
		}
		switch {
		case strings.EqualFold(strings.TrimSpace(string(name)), "Content-Length"):
			parsed, err := strconv.Atoi(strings.TrimSpace(string(value)))
			if err != nil || parsed < 0 {
				return 0, false
			}
			contentLength = parsed
		case strings.EqualFold(strings.TrimSpace(string(name)), "Transfer-Encoding"):
			if request {
				// libhtp only frames a request as chunked when the value is
				// exactly "chunked" and HTTP/1.1 or newer is used. Other
				// values are invalid and do not describe a body.
				if http11 && bytes.EqualFold(bytes.TrimSpace(value), []byte("chunked")) {
					chunked = true
				}
			} else if strings.Contains(strings.ToLower(string(value)), "chunked") {
				chunked = true
			}
		}
	}
	if contentLength >= 0 {
		total := headerLength + contentLength
		return total, len(payload) >= total
	}
	if chunked {
		bodyLength, ok := chunkedBodyLength(body)
		if !ok {
			return 0, false
		}
		return headerLength + bodyLength, true
	}
	return headerLength, true
}

func hasChunkedEncoding(values []string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), "chunked") {
			return true
		}
	}
	return false
}

// decodeChunkedBody decodes Transfer-Encoding: chunked wire bytes. lowhttp's
// fixed-content-length request reader retains the wire body, so the matcher
// decodes it lazily for HTTP request/response body buffers.
func decodeChunkedBody(body []byte) ([]byte, bool) {
	var decoded bytes.Buffer
	offset := 0
	for {
		lineEnd := bytes.Index(body[offset:], []byte("\r\n"))
		if lineEnd < 0 {
			return nil, false
		}
		sizeText := string(body[offset : offset+lineEnd])
		if semi := strings.IndexByte(sizeText, ';'); semi >= 0 {
			sizeText = sizeText[:semi]
		}
		size, err := strconv.ParseInt(strings.TrimSpace(sizeText), 16, 64)
		if err != nil || size < 0 {
			return nil, false
		}
		offset += lineEnd + 2
		if size == 0 {
			return decoded.Bytes(), true
		}
		if size > int64(len(body)-offset) {
			return nil, false
		}
		decoded.Write(body[offset : offset+int(size)])
		offset += int(size)
		if !bytes.HasPrefix(body[offset:], []byte("\r\n")) {
			return nil, false
		}
		offset += 2
	}
}

func chunkedBodyLength(body []byte) (int, bool) {
	offset := 0
	for {
		lineEnd := bytes.Index(body[offset:], []byte("\r\n"))
		if lineEnd < 0 {
			return 0, false
		}
		sizeText := string(body[offset : offset+lineEnd])
		if semi := strings.IndexByte(sizeText, ';'); semi >= 0 {
			sizeText = sizeText[:semi]
		}
		size, err := strconv.ParseInt(strings.TrimSpace(sizeText), 16, 64)
		if err != nil || size < 0 {
			return 0, false
		}
		offset += lineEnd + 2
		if size == 0 {
			if bytes.HasPrefix(body[offset:], []byte("\r\n")) {
				return offset + 2, true
			}
			trailerEnd := bytes.Index(body[offset:], []byte("\r\n\r\n"))
			if trailerEnd < 0 {
				return 0, false
			}
			return offset + trailerEnd + 4, true
		}
		if size > int64(len(body)-offset) {
			return 0, false
		}
		offset += int(size)
		if !bytes.HasPrefix(body[offset:], []byte("\r\n")) {
			return 0, false
		}
		offset += 2
	}
}

func (h *httpProvider) GetRaw() []byte {
	return h.raw
}

func (h *httpProvider) Parsed() any {
	if h.req != nil {
		return h.req
	}
	if h.res != nil {
		return h.res
	}
	return nil
}

func (h *httpProvider) GetRequest() *http.Request {
	if h.req == nil {
		return nil
	}
	return h.req
}

func (h *httpProvider) GetResponse() *http.Response {
	if h.res == nil {
		return nil
	}
	return h.res
}

func (h *httpProvider) bodyBuffer() []byte {
	if h.bodyCached {
		return h.body
	}
	h.bodyCached = true
	index := bytes.Index(h.raw, []byte("\r\n\r\n"))
	if index < 0 {
		return nil
	}
	body := h.raw[index+4:]
	if length, complete := HTTPMessageLength(h.raw); complete && length >= index+4 && length <= len(h.raw) {
		// Keep-alive streams contain subsequent messages after the first one;
		// the body buffer must stop at the first message boundary.
		body = h.raw[index+4 : length]
	}
	chunked := false
	if h.req != nil {
		chunked = hasChunkedEncoding(h.req.TransferEncoding)
	}
	if h.res != nil {
		chunked = hasChunkedEncoding(h.res.TransferEncoding)
	}
	if chunked {
		if decoded, ok := decodeChunkedBody(body); ok {
			body = decoded
		}
	}
	h.body = body
	return body
}

// Get part of http.
func (h *httpProvider) Get(modi modifier.Modifier) []byte {
	if h.req != nil {
		return h.getReq(modi)
	}
	if h.res != nil {
		return h.getRes(modi)
	}
	return nil
}

func rawHTTPHeaderValue(raw []byte, name string) []byte {
	lines := bytes.Split(raw, []byte("\r\n"))
	for _, line := range lines[1:] {
		index := bytes.IndexByte(line, ':')
		if index <= 0 || !strings.EqualFold(string(line[:index]), name) {
			continue
		}
		return bytes.TrimSpace(line[index+1:])
	}
	return nil
}

func (h *httpProvider) getReq(modi modifier.Modifier) []byte {
	switch modi {
	case modifier.HTTPUri:
		return []byte(h.req.RequestURI)
	case modifier.HTTPUriRaw:
		rd := bufio.NewReader(bytes.NewReader(h.raw))
		if _, err := rd.ReadBytes(' '); err != nil {
			return nil
		}
		uriraw, err := rd.ReadBytes(' ')
		if err != nil {
			return nil
		}
		if len(uriraw) == 0 {
			return nil
		}
		return uriraw[:len(uriraw)-1]
	case modifier.HTTPMethod:
		return []byte(h.req.Method)
	case modifier.HTTPRequestLine:
		idx := bytes.Index(h.raw, []byte("\r\n"))
		if idx == -1 {
			return nil
		}
		return h.raw[:idx+2]
	case modifier.HTTPRequestBody, modifier.FileData:
		return h.bodyBuffer()
	case modifier.HTTPUserAgent:
		return []byte(h.req.UserAgent())
	case modifier.HTTPHost:
		host := h.req.Host
		if parsedHost, _, err := net.SplitHostPort(host); err == nil {
			host = parsedHost
		} else if index := strings.LastIndexByte(host, ':'); index > 0 && !strings.Contains(host[index+1:], "]") {
			if _, err := strconv.Atoi(host[index+1:]); err == nil {
				host = host[:index]
			}
		}
		return []byte(strings.ToLower(host))
	case modifier.HTTPHostRaw:
		return rawHTTPHeaderValue(h.raw, "host")
	case modifier.HTTPAccept:
		return []byte(h.req.Header.Get("Accept"))
	case modifier.HTTPAcceptLang:
		return []byte(h.req.Header.Get("Accept-Language"))
	case modifier.HTTPAcceptEnc:
		return []byte(h.req.Header.Get("Accept-Encoding"))
	case modifier.HTTPReferer:
		return []byte(h.req.Header.Get("Referer"))
	case modifier.HTTPHeader:
		var bb bytes.Buffer
		var strs []string
		for k := range h.req.Header {
			strs = append(strs, k)
		}
		sort.Strings(strs)
		for _, k := range strs {
			bb.WriteString(k)
			bb.WriteString(": ")
			bb.WriteString(h.req.Header[k][0])
			for i := 1; i < len(h.req.Header[k]); i++ {
				bb.WriteString(", ")
				bb.WriteString(h.req.Header[k][i])
			}
			bb.WriteString("\r\n")
		}
		return bb.Bytes()
	case modifier.HTTPHeaderRaw:
		ed := bytes.Index(h.raw, []byte("\r\n\r\n"))
		st := bytes.Index(h.raw, []byte("\r\n"))
		if ed-st <= 2 {
			return nil
		}
		return h.raw[st+2 : ed+2]
	case modifier.HTTPCookie:
		return []byte(h.req.Header.Get("Cookie"))
	case modifier.HTTPConnection:
		return []byte(h.req.Header.Get("Connection"))
	case modifier.HTTPContentType:
		return []byte(h.req.Header.Get("Content-Type"))
	case modifier.HTTPContentLen:
		if value := rawHeaderValue(h.raw, "Content-Length"); value != nil {
			return value
		}
		return []byte(h.req.Header.Get("Content-Length"))
	case modifier.HTTPStart:
		idx := bytes.Index(h.raw, []byte("\r\n\r\n"))
		if idx == -1 {
			return nil
		}
		return lowhttp.FixHTTPRequest(h.raw[:idx+4])
	case modifier.HTTPProtocol:
		return []byte(h.req.Proto)
	case modifier.HTTPHeaderNames:
		var bb bytes.Buffer
		var names []string
		for k := range h.req.Header {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			bb.WriteString("\r\n")
			bb.WriteString(k)
		}
		bb.WriteString("\r\n\r\n")
		return bb.Bytes()
	case modifier.Default:
		return h.raw
	}
	return nil
}

func (h *httpProvider) getRes(modi modifier.Modifier) []byte {
	switch modi {
	case modifier.HTTPStatMsg:
		ss := strings.SplitN(h.res.Status, " ", 2)
		if len(ss) != 2 {
			return nil
		}
		return []byte(ss[1])
	case modifier.HTTPStatCode:
		return []byte(strconv.Itoa(h.res.StatusCode))
	case modifier.HTTPResponseLine:
		ed := bytes.Index(h.raw, []byte("\r\n"))
		if ed == -1 {
			return nil
		}
		return h.raw[:ed+2]
	case modifier.HTTPHeader:
		var bb bytes.Buffer
		var strs []string
		for k := range h.res.Header {
			strs = append(strs, k)
		}
		sort.Strings(strs)
		for _, k := range strs {
			bb.WriteString(k)
			bb.WriteString(": ")
			bb.WriteString(h.res.Header[k][0])
			for i := 1; i < len(h.res.Header[k]); i++ {
				bb.WriteString(", ")
				bb.WriteString(h.res.Header[k][i])
			}
			bb.WriteString("\r\n")
		}
		return bb.Bytes()
	case modifier.HTTPHeaderRaw:
		ed := bytes.Index(h.raw, []byte("\r\n\r\n"))
		st := bytes.Index(h.raw, []byte("\r\n"))
		if ed-st <= 2 {
			return nil
		}
		return h.raw[st+2 : ed+2]
	case modifier.HTTPCookie:
		return []byte(h.res.Header.Get("Set-Cookie"))
	case modifier.HTTPResponseBody, modifier.FileData:
		return h.bodyBuffer()
	case modifier.HTTPServer:
		return []byte(h.res.Header.Get("Server"))
	case modifier.HTTPLocation:
		return []byte(h.res.Header.Get("Location"))
	case modifier.HTTPContentType:
		return []byte(h.res.Header.Get("Content-Type"))
	case modifier.HTTPContentLen:
		if value := rawHeaderValue(h.raw, "Content-Length"); value != nil {
			return value
		}
		return []byte(h.res.Header.Get("Content-Length"))
	case modifier.HTTPStart:
		idx := bytes.Index(h.raw, []byte("\r\n\r\n"))
		if idx == -1 {
			return nil
		}
		return lowhttp.FixHTTPRequest(h.raw[:idx+4])
	case modifier.HTTPProtocol:
		return []byte(h.res.Proto)
	case modifier.HTTPHeaderNames:
		var bb bytes.Buffer
		var names []string
		for k := range h.res.Header {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			bb.WriteString("\r\n")
			bb.WriteString(k)
		}
		bb.WriteString("\r\n\r\n")
		return bb.Bytes()
	case modifier.HTTPConnection:
		return []byte(h.res.Header.Get("Connection"))
	case modifier.Default:
		return h.raw
	}
	return nil
}
