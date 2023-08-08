package suricata

import (
	"bufio"
	"bytes"
	"github.com/google/gopacket"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/exp/slices"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type HTTPConfig struct {
	// deprecated and not implemented
	Uricontent string

	// not set 0
	// equal 1
	// bigger than 2
	// smaller than 3
	// between 4
	UrilenOp   int
	UrilenNum1 int
	UrilenNum2 int
}

func httpHandler(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	httpraw := c.PK.ApplicationLayer().LayerContents()
	if !c.Must(len(httpraw) != 0) {
		return nil
	}

	// spliter
	spliter := newHTTPSpliter(c.PK)
	if spliter == nil {
		return errors.New("parse httpraw as http request failed")
	}
	// prefilter
	if c.Rule.ContentRuleConfig.PrefilterRule != nil {
		// keyword prefilter not implement yet
	}
	// fast pattern
	idx := slices.IndexFunc(c.Rule.ContentRuleConfig.ContentRules, func(rule *ContentRule) bool {
		return rule.FastPattern
	})
	if idx != -1 {
		fastPatternRule := c.Rule.ContentRuleConfig.ContentRules[idx]
		if fastPatternRule.Modifier == FileData {
			// filedata has its individual matcher
			c.Attach(newFileDataMatcher(fastPatternRule, spliter.GetParsed()))
		} else {
			c.Attach(newPayloadMatcher(fastPatternCopy(fastPatternRule), spliter.Get(fastPatternRule.Modifier)))
		}
		err := c.Next()
		if c.IsRejected() {
			return err
		}
	}
	// beside payload matcher
	var err error
	if spliter.GetRequest() != nil {
		err = httpReqMatcher(c, spliter)
	} else {
		err = httpResMatcher(c, spliter)
	}
	if c.IsRejected() {
		return err
	}
	// loop
	for _, r := range c.Rule.ContentRuleConfig.ContentRules {
		if r.Modifier == FileData {
			// filedata has its individual matcher
			c.Attach(newFileDataMatcher(r, spliter.GetResponse()))
		} else {
			c.Attach(newPayloadMatcher(r, spliter.Get(r.Modifier)))
		}
		err := c.Next()
		if c.IsRejected() {
			return err
		}
	}
	return nil
}

func httpReqMatcher(c *matchContext, spliter *httpSpliter) error {
	if cf := c.Rule.ContentRuleConfig.HTTPConfig; cf != nil {
		if cf.Uricontent != "" {
			log.Errorf("uricontent has been deprecated and not implemented yet")
		}
		uri := spliter.Get(HTTPUri)
		switch cf.UrilenOp {
		case 1:
			if !c.Must(c.Rule.ContentRuleConfig.HTTPConfig.UrilenNum1 == len(uri)) {
				return nil
			}
		case 2:
			if !c.Must(c.Rule.ContentRuleConfig.HTTPConfig.UrilenNum1 > len(uri)) {
				return nil
			}
		case 3:
			if !c.Must(c.Rule.ContentRuleConfig.HTTPConfig.UrilenNum1 < len(uri)) {
				return nil
			}
		case 4:
			if !c.Must(c.Rule.ContentRuleConfig.HTTPConfig.UrilenNum1 < len(uri) && c.Rule.ContentRuleConfig.HTTPConfig.UrilenNum2 > len(uri)) {
				return nil
			}
		default:
			// not set
		}
	}
	return nil
}

func httpResMatcher(c *matchContext, spliter *httpSpliter) error {
	return nil
}

type httpSpliter struct {
	PK gopacket.Packet

	// cache
	raw []byte
	req *http.Request
	res *http.Response
}

// if success, return value not nil
func newHTTPSpliter(pk gopacket.Packet) *httpSpliter {
	payload := pk.ApplicationLayer().LayerContents()
	if lowhttp.IsResp(payload) {
		res, err := lowhttp.ParseBytesToHTTPResponse(payload)
		if err != nil {
			log.Errorf("parse httpraw as http response failed: %s", err.Error())
			return nil
		}
		return &httpSpliter{
			PK:  pk,
			res: res,
			raw: payload,
		}
	}
	request, err := lowhttp.ParseBytesToHttpRequest(payload)
	if err != nil {
		log.Errorf("parse httpraw as http request failed: %s", err.Error())
		return nil
	}
	return &httpSpliter{
		PK:  pk,
		req: request,
		raw: payload,
	}
}

func (h *httpSpliter) GetRaw() []byte {
	return h.raw
}

func (h *httpSpliter) GetParsed() any {
	if h.req != nil {
		return h.req
	}
	return h.res
}

func (h *httpSpliter) GetRequest() *http.Request {
	return h.req
}

func (h *httpSpliter) GetResponse() *http.Response {
	return h.res
}

// Get part of http.
func (h *httpSpliter) Get(modi Modifier) []byte {
	if h.req != nil {
		return h.getReq(modi)
	}
	if h.res != nil {
		return h.getRes(modi)
	}
	return nil
}

func (h *httpSpliter) getReq(modi Modifier) []byte {
	switch modi {
	case HTTPUri:
		return []byte(h.req.URL.Path)
	case HTTPUriRaw:
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
	case HTTPMethod:
		return []byte(h.req.Method)
	case HTTPRequestLine:
		idx := bytes.Index(h.raw, []byte("\r\n"))
		if idx == -1 {
			return nil
		}
		return h.raw[:idx+2]
	case HTTPRequestBody:
		all, err := io.ReadAll(h.req.Body)
		if err != nil {
			return nil
		}
		return all
	case HTTPUserAgent:
		return []byte(h.req.UserAgent())
	case HTTPHost:
		return []byte(h.req.Host)
	case HTTPHostRaw:
		st := bytes.Index(h.raw, []byte("\r\nHost: "))
		if st == -1 {
			return nil
		}
		ed := bytes.Index(h.raw[st+8:], []byte("\r\n"))
		if ed == -1 {
			return nil
		}
		return h.raw[st+8 : st+8+ed]
	case HTTPAccept:
		return []byte(h.req.Header.Get("Accept"))
	case HTTPAcceptLang:
		return []byte(h.req.Header.Get("Accept-Language"))
	case HTTPAcceptEnc:
		return []byte(h.req.Header.Get("Accept-Encoding"))
	case HTTPReferer:
		return []byte(h.req.Header.Get("Referer"))
	case HTTPHeader:
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
	case HTTPHeaderRaw:
		ed := bytes.Index(h.raw, []byte("\r\n\r\n"))
		st := bytes.Index(h.raw, []byte("\r\n"))
		if ed-st <= 2 {
			return nil
		}
		return h.raw[st+2 : ed+2]
	case HTTPCookie:
		return []byte(h.req.Header.Get("Cookie"))
	case HTTPConnection:
		return []byte(h.req.Header.Get("Connection"))
	case HTTPContentType:
		return []byte(h.req.Header.Get("Content-Type"))
	case HTTPContentLen:
		return []byte(h.req.Header.Get("Content-Length"))
	case HTTPStart:
		idx := bytes.Index(h.raw, []byte("\r\n\r\n"))
		if idx == -1 {
			return nil
		}
		return lowhttp.FixHTTPRequestOut(h.raw[:idx+4])
	case HTTPProtocol:
		return []byte(h.req.Proto)
	case HTTPHeaderNames:
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
	case Default:
		return h.raw
	}
	return nil
}

func (h *httpSpliter) getRes(modi Modifier) []byte {
	switch modi {
	case HTTPStatMsg:
		ss := strings.SplitN(h.res.Status, " ", 2)
		if len(ss) != 2 {
			return nil
		}
		return []byte(ss[1])
	case HTTPStatCode:
		return []byte(strconv.Itoa(h.res.StatusCode))
	case HTTPResponseLine:
		ed := bytes.Index(h.raw, []byte("\r\n"))
		if ed == -1 {
			return nil
		}
		return h.raw[:ed+2]
	case HTTPHeader:
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
	case HTTPHeaderRaw:
		ed := bytes.Index(h.raw, []byte("\r\n\r\n"))
		st := bytes.Index(h.raw, []byte("\r\n"))
		if ed-st <= 2 {
			return nil
		}
		return h.raw[st+2 : ed+2]
	case HTTPCookie:
		return []byte(h.res.Header.Get("Set-Cookie"))
	case HTTPResponseBody:
		idx := bytes.Index(h.raw, []byte("\r\n\r\n"))
		if idx == -1 {
			return nil
		}
		return h.raw[idx+4:]
	case HTTPServer:
		return []byte(h.res.Header.Get("Server"))
	case HTTPLocation:
		return []byte(h.res.Header.Get("Location"))
	case HTTPContentType:
		return []byte(h.res.Header.Get("Content-Type"))
	case HTTPContentLen:
		return []byte(h.res.Header.Get("Content-Length"))
	case HTTPStart:
		idx := bytes.Index(h.raw, []byte("\r\n\r\n"))
		if idx == -1 {
			return nil
		}
		return lowhttp.FixHTTPRequestOut(h.raw[:idx+4])
	case HTTPProtocol:
		return []byte(h.res.Proto)
	case HTTPHeaderNames:
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
	case HTTPConnection:
		return []byte(h.res.Header.Get("Connection"))
	case Default:
		return h.raw
	}
	return nil
}
