package crawlerx

import (
	"bytes"
	"context"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/ysmood/gson"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
)

func NewBrowserHijackRequests(browser *rod.Browser) *CrawlerRouter {
	return newCrawlerHijackRouter(browser, browser).initEvents()
}

func NewPageHijackRequests(page *rod.Page) *CrawlerRouter {
	return newCrawlerHijackRouter(page.Browser(), page).initEvents()
}

type CrawlerRouter struct {
	run      func()
	stop     func()
	handlers []*CrawlerHijackHandler
	enable   *proto.FetchEnable
	client   proto.Client
	browser  *rod.Browser
}

func newCrawlerHijackRouter(browser *rod.Browser, client proto.Client) *CrawlerRouter {
	return &CrawlerRouter{
		enable:   &proto.FetchEnable{},
		browser:  browser,
		client:   client,
		handlers: []*CrawlerHijackHandler{},
	}
}

func (router *CrawlerRouter) initEvents() *CrawlerRouter {
	ctx := router.browser.GetContext()
	if cta, ok := router.client.(proto.Contextable); ok {
		ctx = cta.GetContext()
	}
	var sessionID proto.TargetSessionID
	if tsa, ok := router.client.(proto.Sessionable); ok {
		sessionID = tsa.GetSessionID()
	}
	eventCtx, cancel := context.WithCancel(ctx)
	router.stop = cancel
	_ = router.enable.Call(router.client)
	router.run = BrowserEachEvent(router.browser.Context(eventCtx), sessionID, func(e *proto.FetchRequestPaused) bool {
		go func() {
			hijack := router.new(eventCtx, e)
			if hijack.Request.req == nil {
				err := hijack.Response.fail.Call(router.client)
				if err != nil {
					hijack.OnError(err)
				}
				return
			}
			for _, h := range router.handlers {
				if !h.regexp.MatchString(e.Request.URL) {
					continue
				}
				h.handler(hijack)
				if hijack.continueRequest != nil {
					hijack.continueRequest.RequestID = e.RequestID
					err := hijack.continueRequest.Call(router.client)
					if err != nil {
						hijack.OnError(err)
					}
					return
				}
				if hijack.Skip {
					continue
				}
				if hijack.Response.fail.ErrorReason != "" {
					err := hijack.Response.fail.Call(router.client)
					if err != nil {
						hijack.OnError(err)
					}
					return
				}
				err := hijack.Response.payload.Call(router.client)
				if err != nil {
					hijack.OnError(err)
					return
				}
			}
		}()
		return false
	})
	return router
}

func (router *CrawlerRouter) new(ctx context.Context, e *proto.FetchRequestPaused) *CrawlerHijack {
	headers := http.Header{}
	for k, v := range e.Request.Headers {
		headers[k] = []string{v.String()}
	}
	req, err := http.NewRequest(e.Request.Method, e.Request.URL, io.NopCloser(strings.NewReader(e.Request.PostData)))
	if err != nil {
		log.Debugf("check request error: %v!", err)
		return &CrawlerHijack{
			Request: &CrawlerHijackRequest{
				event: e,
				req:   nil,
			},
			Response: &CrawlerHijackResponse{
				payload: &proto.FetchFulfillRequest{
					ResponseCode: 200,
					RequestID:    e.RequestID,
				},
				fail: &proto.FetchFailRequest{
					RequestID:   e.RequestID,
					ErrorReason: proto.NetworkErrorReasonNameNotResolved,
				},
			},
			OnError: func(err error) {},

			browser: router.browser,
		}
	}
	req.Header = headers
	return &CrawlerHijack{
		Request: &CrawlerHijackRequest{
			event: e,
			req:   req.WithContext(ctx),
		},
		Response: &CrawlerHijackResponse{
			payload: &proto.FetchFulfillRequest{
				ResponseCode: 200,
				RequestID:    e.RequestID,
			},
			fail: &proto.FetchFailRequest{
				RequestID: e.RequestID,
			},
		},
		OnError: func(err error) {},

		browser: router.browser,
	}
}

func (router *CrawlerRouter) Add(pattern string, resourceType proto.NetworkResourceType, handler func(*CrawlerHijack)) error {
	router.enable.Patterns = append(router.enable.Patterns, &proto.FetchRequestPattern{
		URLPattern:   pattern,
		ResourceType: resourceType,
	})
	reg := regexp.MustCompile(proto.PatternToReg(pattern))
	router.handlers = append(router.handlers, &CrawlerHijackHandler{
		pattern: pattern,
		regexp:  reg,
		handler: handler,
	})
	return router.enable.Call(router.client)
}

func (router *CrawlerRouter) Run() {
	router.run()
}

func (router *CrawlerRouter) Stop() error {
	router.stop()
	return proto.FetchDisable{}.Call(router.client)
}

type CrawlerHijackHandler struct {
	pattern string
	regexp  *regexp.Regexp
	handler func(*CrawlerHijack)
}

type CrawlerHijack struct {
	Request  *CrawlerHijackRequest
	Response *CrawlerHijackResponse
	OnError  func(error)

	Skip bool

	continueRequest *proto.FetchContinueRequest

	CustomState interface{}

	browser *rod.Browser
}

func (hijack *CrawlerHijack) ContinueRequest(cq *proto.FetchContinueRequest) {
	hijack.continueRequest = cq
}

func (hijack *CrawlerHijack) LoadResponse(opts []lowhttp.LowhttpOpt, loadBody bool) error {
	opts = append(opts, lowhttp.WithRequest(hijack.Request.req), lowhttp.WithRedirectTimes(0))
	lowHttpResponse, err := lowhttp.HTTP(
		opts...,
	)
	if err != nil {
		return err
	}
	res, err := lowhttp.ParseBytesToHTTPResponse(lowHttpResponse.RawPacket)
	if err != nil {
		return err
	}
	hijack.Response.payload.ResponseCode = res.StatusCode
	list := []string{}
	for k, vs := range res.Header {
		for _, v := range vs {
			list = append(list, k, v)
		}
	}
	hijack.Response.SetHeader(list...)
	if loadBody {
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		hijack.Response.payload.Body = b
	}
	return nil
}

type CrawlerHijackRequest struct {
	event *proto.FetchRequestPaused
	req   *http.Request
}

func (hijack *CrawlerHijackRequest) Type() proto.NetworkResourceType {
	return hijack.event.ResourceType
}

func (hijack *CrawlerHijackRequest) Method() string {
	return hijack.event.Request.Method
}

func (hijack *CrawlerHijackRequest) URL() *url.URL {
	u, _ := url.Parse(hijack.event.Request.URL)
	return u
}

func (hijack *CrawlerHijackRequest) Header(key string) string {
	return hijack.event.Request.Headers[key].String()
}

func (hijack *CrawlerHijackRequest) Headers() proto.NetworkHeaders {
	return hijack.event.Request.Headers
}

func (hijack *CrawlerHijackRequest) Body() string {
	return hijack.event.Request.PostData
}

func (hijack *CrawlerHijackRequest) JSONBody() gson.JSON {
	return gson.NewFrom(hijack.Body())
}

func (hijack *CrawlerHijackRequest) Req() *http.Request {
	return hijack.req
}

func (hijack *CrawlerHijackRequest) SetContext(ctx context.Context) *CrawlerHijackRequest {
	hijack.req = hijack.req.WithContext(ctx)
	return hijack
}

func (hijack *CrawlerHijackRequest) SetBody(obj interface{}) *CrawlerHijackRequest {
	var b []byte
	switch body := obj.(type) {
	case []byte:
		b = body
	case string:
		b = []byte(body)
	default:
		b = utils.MustToJSONBytes(body)
	}
	hijack.req.Body = io.NopCloser(bytes.NewBuffer(b))
	return hijack
}

func (hijack *CrawlerHijackRequest) IsNavigation() bool {
	return hijack.Type() == proto.NetworkResourceTypeDocument
}

type CrawlerHijackResponse struct {
	payload *proto.FetchFulfillRequest
	fail    *proto.FetchFailRequest
}

func (hijack *CrawlerHijackResponse) Payload() *proto.FetchFulfillRequest {
	return hijack.payload
}

func (hijack *CrawlerHijackResponse) Body() string {
	return string(hijack.payload.Body)
}

func (hijack *CrawlerHijackResponse) Headers() http.Header {
	header := http.Header{}
	for _, h := range hijack.payload.ResponseHeaders {
		header.Add(h.Name, h.Value)
	}
	return header
}

func (hijack *CrawlerHijackResponse) SetHeader(pairs ...string) *CrawlerHijackResponse {
	for i := 0; i < len(pairs); i += 2 {
		hijack.payload.ResponseHeaders = append(hijack.payload.ResponseHeaders, &proto.FetchHeaderEntry{
			Name:  pairs[i],
			Value: pairs[i+1],
		})
	}
	return hijack
}

func (hijack *CrawlerHijackResponse) SetBody(obj interface{}) *CrawlerHijackResponse {
	switch body := obj.(type) {
	case []byte:
		hijack.payload.Body = body
	case string:
		hijack.payload.Body = []byte(body)
	default:
		hijack.payload.Body = utils.MustToJSONBytes(body)
	}
	return hijack
}

func (hijack *CrawlerHijackResponse) Fail(reason proto.NetworkErrorReason) *CrawlerHijackResponse {
	hijack.fail.ErrorReason = reason
	return hijack
}

func BrowserEachEvent(browser *rod.Browser, sessionID proto.TargetSessionID, callbacks ...interface{}) func() {
	cbMap := map[string]reflect.Value{}
	restores := []func(){}

	for _, cb := range callbacks {
		cbVal := reflect.ValueOf(cb)
		eType := cbVal.Type().In(0)
		name := reflect.New(eType.Elem()).Interface().(proto.Event).ProtoEvent()
		cbMap[name] = cbVal

		// Only enabled domains will emit events to cdp client.
		// We enable the domains for the event types if it's not enabled.
		// We restore the domains to their previous states after the wait ends.
		domain, _ := proto.ParseMethodName(name)
		if req := proto.GetType(domain + ".enable"); req != nil {
			enable := reflect.New(req).Interface().(proto.Request)
			restores = append(restores, browser.EnableDomain(sessionID, enable))
		}
	}

	browser, cancel := browser.WithCancel()
	messages := browser.Event()

	return func() {
		if messages == nil {
			panic("can't use wait function twice")
		}

		defer func() {
			cancel()
			messages = nil
			for _, restore := range restores {
				restore()
			}
		}()

		for msg := range messages {
			if !(sessionID == "" || msg.SessionID == sessionID) {
				continue
			}

			if cbVal, has := cbMap[msg.Method]; has {
				e := reflect.New(proto.GetType(msg.Method))
				msg.Load(e.Interface().(proto.Event))
				args := []reflect.Value{e}
				if cbVal.Type().NumIn() == 2 {
					args = append(args, reflect.ValueOf(msg.SessionID))
				}
				res := cbVal.Call(args)
				if len(res) > 0 {
					if res[0].Bool() {
						return
					}
				}
			}
		}
	}
}
