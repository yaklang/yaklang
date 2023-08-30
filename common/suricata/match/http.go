package match

import (
	"bufio"
	"bytes"
	"github.com/google/gopacket"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/exp/slices"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func httpIniter(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	// buffer provider
	provider := newHTTPBufferProvider(c.PK)
	if provider == nil {
		return errors.New("parse httpraw as http request failed")
	}

	// prefilter
	if c.Rule.ContentRuleConfig.PrefilterRule != nil {
		// keyword prefilter not implement yet
	}

	// register buffer provider
	c.SetBufferProvider(provider.Get)

	// fast pattern
	idx := slices.IndexFunc(c.Rule.ContentRuleConfig.ContentRules, func(rule *rule.ContentRule) bool {
		return rule.FastPattern
	})
	if idx != -1 {
		fastPatternRule := c.Rule.ContentRuleConfig.ContentRules[idx]
		if fastPatternRule.Modifier == modifier.FileData {
			// filedata has its individual matcher
			c.Attach(newFileDataMatcher(fastPatternRule, provider.Parsed()))
		} else {
			c.Attach(
				newPayloadMatcher(
					fastPatternCopy(fastPatternRule),
					fastPatternRule.Modifier),
			)
		}
		err := c.Next()
		if c.IsRejected() {
			return err
		}
	}
	// http match
	var err error
	if provider.GetRequest() != nil {
		err = httpReqMatcher(c, provider)
	} else {
		err = httpResMatcher(c, provider)
	}
	if c.IsRejected() {
		return err
	}

	// payload match
	for _, r := range c.Rule.ContentRuleConfig.ContentRules {
		if r.Modifier == modifier.FileData {
			// filedata has its individual matcher
			c.Attach(newFileDataMatcher(r, provider.GetResponse()))
		} else {
			c.Attach(newPayloadMatcher(r, r.Modifier))
		}
	}
	return nil
}

func httpReqMatcher(c *matchContext, spliter *httpProvider) error {
	if cf := c.Rule.ContentRuleConfig.HTTPConfig; cf != nil {
		if cf.Uricontent != "" {
			log.Errorf("uricontent has been deprecated and not implemented yet")
		}
		if !c.Must(cf.UrilenOp.Match(len(spliter.Get(modifier.HTTPUri)))) {
			return nil
		}
	}
	return nil
}

func httpResMatcher(c *matchContext, spliter *httpProvider) error {
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
	if lowhttp.IsResp(payload) {
		res, err := lowhttp.ParseBytesToHTTPResponse(payload)
		if err != nil {
			log.Errorf("parse httpraw as http response failed: %s", err.Error())
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
		log.Errorf("parse httpraw as http request failed: %s", err.Error())
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
	return h.res
}

func (h *httpProvider) GetRequest() *http.Request {
	return h.req
}

func (h *httpProvider) GetResponse() *http.Response {
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
		return lowhttp.FixHTTPRequestOut(h.raw[:idx+4])
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
		return lowhttp.FixHTTPRequestOut(h.raw[:idx+4])
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
