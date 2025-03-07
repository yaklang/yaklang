package match

import (
	"bufio"
	"bytes"
	"io"
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
	var packets = make([]gopacket.Packet, 2)

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

	// buffer provider
	provider := newHTTPBufferProvider(c.PK)
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
	// http match
	if isreq, ok := c.Value["isReq"].(bool); ok && isreq {
		c.Attach(httpReqMatcher)
	} else {
		// suricata 文档没有 http resp 的非 sticky modifier
		// c.Attach(httpResMatcher)
	}
}

func httpReqMatcher(c *matchContext) error {
	if cf := c.Rule.ContentRuleConfig.HTTPConfig; cf != nil {
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
	raw []byte
	req *http.Request
	res *http.Response
}

// if success, return value not nil
func newHTTPBufferProvider(pk gopacket.Packet) *httpProvider {
	payload := pk.TransportLayer().LayerPayload()
	a, b, c := lowhttp.GetHTTPPacketFirstLine(payload)
	if a == "" || b == "" || c == "" {
		return nil
	}
	if strings.HasPrefix(a, "HTTP/") {
		res, err := lowhttp.ParseBytesToHTTPResponse(payload)
		if err != nil {
			return nil
		}
		return &httpProvider{
			PK:  pk,
			res: res,
			raw: payload,
		}
	}
	request, err := lowhttp.ParseBytesToHttpRequest(payload)
	if err != nil {
		return nil
	}
	return &httpProvider{
		PK:  pk,
		req: request,
		raw: payload,
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
	case modifier.HTTPRequestBody:
		all, err := io.ReadAll(h.req.Body)
		if err != nil {
			return nil
		}
		return all
	case modifier.HTTPUserAgent:
		return []byte(h.req.UserAgent())
	case modifier.HTTPHost:
		return []byte(h.req.Host)
	case modifier.HTTPHostRaw:
		st := bytes.Index(h.raw, []byte("\r\nHost: "))
		if st == -1 {
			return nil
		}
		ed := bytes.Index(h.raw[st+8:], []byte("\r\n"))
		if ed == -1 {
			return nil
		}
		return h.raw[st+8 : st+8+ed]
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
	case modifier.HTTPResponseBody:
		idx := bytes.Index(h.raw, []byte("\r\n\r\n"))
		if idx == -1 {
			return nil
		}
		return h.raw[idx+4:]
	case modifier.HTTPServer:
		return []byte(h.res.Header.Get("Server"))
	case modifier.HTTPLocation:
		return []byte(h.res.Header.Get("Location"))
	case modifier.HTTPContentType:
		return []byte(h.res.Header.Get("Content-Type"))
	case modifier.HTTPContentLen:
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
