package yakgrpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

type WrapperHTTPFuzzerStream struct {
	grpc.ServerStream

	fallback *httpFuzzerFallback
}

func NewHTTPFuzzerFallbackWrapper(fallback *httpFuzzerFallback) *WrapperHTTPFuzzerStream {
	return &WrapperHTTPFuzzerStream{fallback: fallback, ServerStream: fallback.originStream}
}

func (w *WrapperHTTPFuzzerStream) Send(r *ypb.FuzzerResponse) error {
	return w.fallback.Send(r)
}

type httpFuzzerFallback struct {
	originStream ypb.Yak_HTTPFuzzerSequenceServer

	sendMutex          *sync.Mutex
	runtimeID          string
	allowMultiResponse bool
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

	if h.firstResponse == nil && h.onFirstResponse != nil {
		h.firstResponse = r
		h.onFirstResponse(h.firstResponse)
	}

	if h.onEveryResponse != nil {
		h.onEveryResponse(r)
	}

	if h.originStream != nil {
		h.sendMutex.Lock()
		defer h.sendMutex.Unlock()
		return h.originStream.Send(&ypb.FuzzerSequenceResponse{
			Request:  h.belong,
			Response: r,
		})
	}
	return utils.Errorf("httpFuzzerFallback is not set for sender")
}

func newHTTPFuzzerFallback(senderMutex *sync.Mutex, runtimeID string, req *ypb.FuzzerRequest, server ypb.Yak_HTTPFuzzerSequenceServer) *httpFuzzerFallback {
	belong := req
	if req.GetFuzzerIndex() != "" {
		belong = &ypb.FuzzerRequest{FuzzerIndex: req.GetFuzzerIndex(), FuzzerTabIndex: req.GetFuzzerTabIndex()}
	}

	return &httpFuzzerFallback{
		sendMutex:    senderMutex,
		originStream: server,
		belong:       belong,
		runtimeID:    runtimeID,
	}
}

func ConvertLowhttpResponseToFuzzerResponseBase(r *lowhttp.LowhttpResponse) *ypb.FuzzerResponse {
	var (
		method  string
		code    int64
		uid     = uuid.New().String()
		headers []*ypb.HTTPHeader
		host    string
		body    []byte
	)
	host = utils.ExtractHost(r.RemoteAddr)

	lowhttp.SplitHTTPPacket(r.RawRequest, func(m string, requestUri string, proto string) error {
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
	fuzzerResponse := &ypb.FuzzerResponse{
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
	next   *fuzzerSequenceFlow
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
			oldParams := lo.Map(response.GetExtractedResults(), func(item *ypb.KVPair, index int) *ypb.FuzzerParamItem {
				return &ypb.FuzzerParamItem{
					Key:          item.GetKey(),
					Value:        item.GetValue(),
					MarshalValue: item.GetMarshalValue(),
					Type:         "raw",
				}
			})
			request.Params = append(oldParams, request.Params...)
		}

		if request.InheritCookies {
			cookieFromReq := lowhttp.GetHTTPPacketCookies(response.GetRequestRaw())
			for _, f := range response.RedirectFlows {
				for k, v := range lowhttp.GetHTTPPacketCookies(f.GetRequest()) {
					cookieFromReq[k] = v
				}
			}
			for k, v := range lowhttp.GetHTTPPacketCookies(response.GetResponseRaw()) {
				cookieFromReq[k] = v
			}
			reqBytes := request.RequestRaw
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

func (s *Server) execFlow(senderMutex *sync.Mutex, flowMax int64, wg *sync.WaitGroup, f *fuzzerSequenceFlow, stream ypb.Yak_HTTPFuzzerSequenceServer) error {
	req := f.GetFuzzerRequest()
	req.FuzzerSequenceIndex = uuid.NewString()
	runtimeID := uuid.NewString()
	fallback := newHTTPFuzzerFallback(senderMutex, runtimeID, req, stream)
	if f.next != nil {
		swg := utils.NewSizedWaitGroup(int(flowMax))
		originRequest := f.next.origin
		fallback.onEveryResponse = func(response *ypb.FuzzerResponse) {
			/*
				copy fuzzer request
			*/
			copiedReq := ypb.FuzzerRequest{}
			raw, err := json.Marshal(originRequest)
			if err != nil {
				log.Errorf("json marshal ypb.FuzzerRequest failed: %s", err)
				return
			}
			err = json.Unmarshal(raw, &copiedReq)
			if err != nil {
				log.Errorf("json unmarshal FuzzerRequest failed: %s", err)
				return
			}

			copiedRsp := ypb.FuzzerResponse{}
			raw, err = json.Marshal(response)
			if err != nil {
				log.Errorf("json marshal ypb.FuzzerRequest failed: %s", err)
				return
			}
			err = json.Unmarshal(raw, &copiedRsp)
			if err != nil {
				log.Errorf("json unmarshal FuzzerRequest failed: %s", err)
				return
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				swg.Add()
				flow := NewFuzzerSequenceFlow(&copiedReq).FromFuzzerResponse(&copiedRsp)
				flow.next = f.next.next
				err := s.execFlow(senderMutex, flowMax, wg, flow, stream)
				swg.Done()
				if err != nil {
					log.Errorf("execFlow: %v", err)
				}
			}()
		}
	}
	err := s.HTTPFuzzer(req, NewHTTPFuzzerFallbackWrapper(fallback))
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
	sequenceFlow := make(chan *fuzzerSequenceFlow, len(reqs))
	finalErr := make(chan error, 1)
	defer func() {
		close(finalErr)
	}()

	senderMutex := new(sync.Mutex)
	maxFlow := seqreq.GetConcurrent()
	if maxFlow <= 0 {
		maxFlow = 5
	}

	wg := new(sync.WaitGroup)
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
				lastFlow.next = flow
			}
			lastFlow = flow
		}

		if firstFlow == nil {
			finalErr <- utils.Errorf("BUG: empty first flow")
			return
		}

		finalErr <- s.execFlow(senderMutex, maxFlow, wg, firstFlow, stream)
	}()
	select {
	case err := <-finalErr:
		return err
	}
}
