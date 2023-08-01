package yakgrpc

import (
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"net/http"
	"sync"
	"time"
)

type httpFuzzerFallback struct {
	grpc.ServerStream

	allowMultiResponse bool
	originSender       func(response *ypb.FuzzerSequenceResponse) error
	belong             *ypb.FuzzerRequest

	// resp
	firstResponse   *ypb.FuzzerResponse
	onFirstResponse func(response *ypb.FuzzerResponse)
	onEveryResponse func(response *ypb.FuzzerResponse)
}

func (h *httpFuzzerFallback) Send(r *ypb.FuzzerResponse) error {
	if r == nil {
		log.Error("send empty FuzzerResponse")
		return nil
	}

	if h.firstResponse == nil && r != nil && h.onFirstResponse != nil {
		h.firstResponse = r
		h.onFirstResponse(h.firstResponse)
	}

	if h.onEveryResponse != nil {
		h.onEveryResponse(r)
	}

	if h.originSender != nil {
		return h.originSender(&ypb.FuzzerSequenceResponse{
			Request:  h.belong,
			Response: r,
		})
	}
	return utils.Errorf("httpFuzzerFallback is not set for sender")
}

func newHTTPFuzzerFallback(req *ypb.FuzzerRequest, server ypb.Yak_HTTPFuzzerSequenceServer) *httpFuzzerFallback {
	var belong = req
	if req.GetFuzzerIndex() != "" {
		belong = &ypb.FuzzerRequest{FuzzerIndex: req.GetFuzzerIndex(), FuzzerTabIndex: req.GetFuzzerTabIndex()}
	}

	return &httpFuzzerFallback{
		ServerStream: server,
		originSender: server.Send,
		belong:       belong,
	}
}

func ConvertLowhttpResponseToFuzzerResponseBase(r *lowhttp.LowhttpResponse) *ypb.FuzzerResponse {
	var (
		method  string
		code    int64
		uid     = uuid.NewV4().String()
		headers []*ypb.HTTPHeader
		host    string
		body    []byte
	)
	host = utils.ExtractHost(r.RemoteAddr)

	lowhttp.SplitHTTPPacket(r.RawPacket, func(m string, requestUri string, proto string) error {
		method = m
		return nil
	}, nil)
	rsp, err := lowhttp.ParseBytesToHTTPResponse(r.RawPacket)
	if rsp == nil {
		return &ypb.FuzzerResponse{
			Method:     method,
			UUID:       uid,
			Host:       host,
			Timestamp:  time.Now().Unix(),
			RequestRaw: r.RawRequest,
			Ok:         false,
			Reason:     fmt.Sprintf("parse bytes to response instance failed: %v", err),
			IsHTTPS:    r.Https,
			Url:        r.Url,
			Proxy:      r.Proxy,
			RemoteAddr: r.RemoteAddr,
		}
	}
	code = int64(rsp.StatusCode)
	_, body = lowhttp.SplitHTTPPacketFast(r.RawPacket)
	for h, c := range rsp.Header {
		for _, v := range c {
			headers = append(headers, &ypb.HTTPHeader{
				Header: h,
				Value:  v,
			})
		}
	}
	var (
		fuzzerResponse = &ypb.FuzzerResponse{
			Method:                method,
			StatusCode:            int32(code),
			Host:                  host,
			ContentType:           rsp.Header.Get("Content-Type"),
			Headers:               headers,
			ResponseRaw:           r.RawPacket,
			BodyLength:            int64(len(body)),
			UUID:                  uid,
			Timestamp:             time.Now().Unix(),
			RequestRaw:            r.RawRequest,
			GuessResponseEncoding: Chardet(r.RawPacket),
			Ok:                    r.PortIsOpen,
			IsHTTPS:               r.Https,
			Url:                   r.Url,
			Proxy:                 r.Proxy,
			RemoteAddr:            r.RemoteAddr,
		}
	)
	if r.TraceInfo != nil {
		fuzzerResponse.DurationMs = r.TraceInfo.ServerTime.Milliseconds()
		fuzzerResponse.DNSDurationMs = r.TraceInfo.DNSTime.Milliseconds()
		fuzzerResponse.FirstByteDurationMs = r.TraceInfo.ServerTime.Milliseconds()
		fuzzerResponse.TotalDurationMs = r.TraceInfo.TotalTime.Milliseconds()
	}
	return fuzzerResponse
}

type fuzzerSequenceFlow struct {
	hijack func(request *ypb.FuzzerRequest) *ypb.FuzzerRequest
	origin *ypb.FuzzerRequest
	next   *ypb.FuzzerRequest
}

func NewFuzzerSequenceFlow(request *ypb.FuzzerRequest) *fuzzerSequenceFlow {
	return &fuzzerSequenceFlow{origin: request}
}

func (f *fuzzerSequenceFlow) GetFuzzerRequest() *ypb.FuzzerRequest {
	if f.hijack == nil {
		return f.origin
	}
	f.hijack(f.origin)
	return f.origin
}

func (f *fuzzerSequenceFlow) FromFuzzerResponse(response *ypb.FuzzerResponse) *fuzzerSequenceFlow {
	f.hijack = func(request *ypb.FuzzerRequest) *ypb.FuzzerRequest {
		if request.InheritVariables {
			for _, k := range response.GetExtractedResults() {
				request.Params = append(request.Params, &ypb.FuzzerParamItem{
					Key:   k.Key,
					Value: k.Value,
					Type:  "raw",
				})
			}
		}

		if request.InheritCookies {
			var cookieFromReq = lowhttp.GetHTTPPacketCookies(response.GetRequestRaw())
			for _, f := range response.RedirectFlows {
				for k, v := range lowhttp.GetHTTPPacketCookies(f.GetRequest()) {
					cookieFromReq[k] = v
				}
			}
			for k, v := range lowhttp.GetHTTPPacketCookies(response.GetResponseRaw()) {
				cookieFromReq[k] = v
			}
			var reqBytes = request.RequestRaw
			if reqBytes == nil || len(reqBytes) <= 0 {
				reqBytes = []byte(request.Request)
			}
			var cookies []*http.Cookie
			for k, v := range cookieFromReq {
				cookies = append(cookies, &http.Cookie{Name: k, Value: v})
			}
			if len(cookies) > 0 {
				reqBytes = lowhttp.ReplaceHTTPPacketHeader(reqBytes, "Cookie", lowhttp.CookiesToString(cookies))
			}
			request.Request = ""
			request.RequestRaw = reqBytes
		}

		return nil
	}
	return f
}

func (s *Server) execFlow(flowMax int64, wg *sync.WaitGroup, f *fuzzerSequenceFlow, stream ypb.Yak_HTTPFuzzerSequenceServer) error {
	var req = f.GetFuzzerRequest()
	fallback := newHTTPFuzzerFallback(req, stream)
	if f.next != nil {
		var swg = utils.NewSizedWaitGroup(int(flowMax))

		fallback.onEveryResponse = func(response *ypb.FuzzerResponse) {
			var copiedReq = ypb.FuzzerRequest{}
			var raw, err = json.Marshal(f.next)
			if err != nil {
				log.Errorf("json marshal ypb.FuzzerRequest failed: %s", err)
				return
			}
			err = json.Unmarshal(raw, &copiedReq)
			if err != nil {
				log.Errorf("json unmarshal FuzzerRequest failed: %s", err)
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				swg.Add()
				err := s.execFlow(flowMax, wg, NewFuzzerSequenceFlow(&copiedReq).FromFuzzerResponse(response), stream)
				swg.Done()
				if err != nil {
					log.Errorf("execFlow: %v", err)
				}
			}()
		}
	}
	err := s.HTTPFuzzer(req, fallback)
	if err != nil {
		log.Error(err)
	}
	return err
}

func (s *Server) HTTPFuzzerSequence(seqreq *ypb.FuzzerRequests, stream ypb.Yak_HTTPFuzzerSequenceServer) error {
	reqs := seqreq.GetRequests()
	if len(reqs) <= 0 {
		return utils.Error("empty fuzzer request")
	}

	var sequenceFlow = make(chan *fuzzerSequenceFlow, len(reqs))
	var finalErr = make(chan error, 1)
	defer func() {
		close(finalErr)
	}()

	var maxFlow = seqreq.GetConcurrent()
	if maxFlow <= 0 {
		maxFlow = 5
	}

	var wg = new(sync.WaitGroup)
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		defer func() {
			close(sequenceFlow)
		}()

		var firstFlow *fuzzerSequenceFlow
		var lastFlow *fuzzerSequenceFlow
		for _, i := range reqs {
			flow := NewFuzzerSequenceFlow(i)
			if firstFlow == nil {
				firstFlow = flow
			}
			if lastFlow != nil {
				lastFlow.next = i
			}
			lastFlow = flow
		}

		if firstFlow == nil {
			finalErr <- utils.Errorf("BUG: empty first flow")
			return
		}

		finalErr <- s.execFlow(maxFlow, wg, firstFlow, stream)
	}()
	select {
	case err := <-finalErr:
		return err
	}
}
