package yak

import (
	"net/url"
	"sort"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type HotPatchPhaseContext struct {
	Request         []byte
	Response        []byte
	OriginRequest   []byte
	OriginResponse  []byte
	ClientResponse  []byte
	ArchiveResponse []byte

	IsHTTPS bool
	URL     string
	Method  string
	Path    string
	Source  string

	Flow *schema.HTTPFlow

	State map[string]any
	Meta  map[string]any
	Tags  map[string]string

	Dropped        bool
	Stopped        bool
	RetryRequested bool
	ArchiveSkipped bool
}

func NewHotPatchRequestPhaseContext(source string, isHTTPS bool, u string, originReq, req, originRsp, rsp []byte) *HotPatchPhaseContext {
	ctx := &HotPatchPhaseContext{
		IsHTTPS: isHTTPS,
		URL:     u,
		Source:  source,
		State:   make(map[string]any),
		Meta:    make(map[string]any),
		Tags:    make(map[string]string),
	}
	ctx.SetOriginRequest(originReq)
	ctx.SetRequest(req)
	ctx.SetOriginResponse(originRsp)
	ctx.SetResponse(rsp)
	return ctx
}

func NewHotPatchFlowArchiveContext(source string, flow *schema.HTTPFlow) *HotPatchPhaseContext {
	ctx := &HotPatchPhaseContext{
		State: make(map[string]any),
		Meta:  make(map[string]any),
		Tags:  make(map[string]string),
	}
	ctx.PrepareForArchivePhase(source, flow)
	return ctx
}

func (c *HotPatchPhaseContext) PrepareForRequestPhase(source string, isHTTPS bool, u string, originReq, req []byte) {
	if c == nil {
		return
	}
	c.beginRequestPhaseGroup()
	c.Source = source
	c.IsHTTPS = isHTTPS
	if u != "" {
		c.URL = u
	}
	if len(originReq) > 0 && len(c.OriginRequest) == 0 {
		c.SetOriginRequest(originReq)
	}
	c.SetRequest(req)
}

func (c *HotPatchPhaseContext) PrepareForResponsePhase(source string, isHTTPS bool, u string, originReq, req, originRsp, rsp []byte) {
	if c == nil {
		return
	}
	c.PrepareForRequestPhase(source, isHTTPS, u, originReq, req)
	if len(originRsp) > 0 && len(c.OriginResponse) == 0 {
		c.SetOriginResponse(originRsp)
	}
	c.SetResponse(rsp)
}

func (c *HotPatchPhaseContext) PrepareForArchivePhase(source string, flow *schema.HTTPFlow) {
	if c == nil {
		return
	}
	c.beginArchivePhaseGroup()
	c.Source = source
	c.Flow = flow
	if flow == nil {
		return
	}
	c.IsHTTPS = flow.IsHTTPS
	if flow.Url != "" {
		c.URL = flow.Url
	}
	req := []byte(flow.GetRequest())
	rsp := []byte(flow.GetResponse())
	if len(c.OriginRequest) == 0 && len(req) > 0 {
		c.SetOriginRequest(req)
	}
	if len(c.OriginResponse) == 0 && len(rsp) > 0 {
		c.SetOriginResponse(rsp)
	}
	if len(req) > 0 {
		c.SetRequest(req)
	}
	if len(rsp) > 0 {
		c.SetResponse(rsp)
	}
	if c.Method == "" {
		c.Method = flow.Method
	}
	if c.Path == "" {
		c.Path = flow.Path
	}
}

func (c *HotPatchPhaseContext) Drop(reason ...string) {
	c.Dropped = true
	c.Stopped = true
	if len(reason) > 0 && reason[0] != "" {
		c.SetMeta("dropReason", reason[0])
	}
}

func (c *HotPatchPhaseContext) Stop() {
	c.Stopped = true
}

func (c *HotPatchPhaseContext) Retry() {
	c.RetryRequested = true
}

func (c *HotPatchPhaseContext) SkipArchive() {
	c.ArchiveSkipped = true
}

func (c *HotPatchPhaseContext) SetClientResponse(raw any) {
	c.ClientResponse = hotPatchInterfaceToBytes(raw)
	c.Stopped = true
}

func (c *HotPatchPhaseContext) SetArchiveResponse(raw any) {
	c.ArchiveResponse = hotPatchInterfaceToBytes(raw)
}

func (c *HotPatchPhaseContext) SetState(key string, value any) {
	if key == "" {
		return
	}
	c.State[key] = value
}

func (c *HotPatchPhaseContext) SetMeta(key string, value any) {
	if key == "" {
		return
	}
	c.Meta[key] = value
}

func (c *HotPatchPhaseContext) SetTag(key string, value any) {
	if key == "" {
		return
	}
	c.Tags[key] = utils.InterfaceToString(value)
}

func (c *HotPatchPhaseContext) ApplyArchiveResultToFlow() {
	if c.Flow == nil {
		return
	}
	if len(c.ArchiveResponse) > 0 {
		c.Flow.SetResponse(string(c.ArchiveResponse))
	}
	for _, tag := range c.FormattedTags() {
		c.Flow.AddTag(tag)
	}
}

func (c *HotPatchPhaseContext) SetRequest(raw []byte) {
	c.Request = cloneHotPatchBytes(raw)
	c.RefreshRequestMetadata()
}

func (c *HotPatchPhaseContext) SetOriginRequest(raw []byte) {
	c.OriginRequest = cloneHotPatchBytes(raw)
}

func (c *HotPatchPhaseContext) SetResponse(raw []byte) {
	c.Response = cloneHotPatchBytes(raw)
}

func (c *HotPatchPhaseContext) SetOriginResponse(raw []byte) {
	c.OriginResponse = cloneHotPatchBytes(raw)
}

func (c *HotPatchPhaseContext) RefreshRequestMetadata() {
	prevURL := c.URL
	c.Method = ""
	c.Path = ""
	c.URL = ""
	c.fillRequestMetadata()
	if c.URL != "" || prevURL == "" {
		return
	}
	c.URL = prevURL
	c.fillRequestMetadata()
}

func (c *HotPatchPhaseContext) FormattedTags() []string {
	if len(c.Tags) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.Tags))
	for key := range c.Tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		value := c.Tags[key]
		if value == "" {
			out = append(out, key)
			continue
		}
		out = append(out, key+":"+value)
	}
	return out
}

func (c *HotPatchPhaseContext) fillRequestMetadata() {
	if c.Request != nil {
		if req, err := lowhttp.ParseBytesToHttpRequest(c.Request); err == nil && req != nil {
			if c.Method == "" {
				c.Method = req.Method
			}
			if c.Path == "" && req.URL != nil {
				c.Path = req.URL.EscapedPath()
			}
		}
		if c.URL == "" {
			if u, _ := lowhttp.ExtractURLFromHTTPRequestRaw(c.Request, c.IsHTTPS); u != nil {
				c.URL = u.String()
			}
		}
	}
	if c.URL == "" {
		return
	}
	if u, err := url.Parse(c.URL); err == nil && u != nil {
		if c.Path == "" {
			c.Path = u.EscapedPath()
		}
	}
}

func hotPatchInterfaceToBytes(raw any) []byte {
	switch ret := raw.(type) {
	case nil:
		return nil
	case []byte:
		return append([]byte(nil), ret...)
	case string:
		return []byte(ret)
	case []rune:
		return []byte(string(ret))
	default:
		return []byte(utils.InterfaceToString(raw))
	}
}

func cloneHotPatchBytes(raw []byte) []byte {
	return append([]byte(nil), raw...)
}
