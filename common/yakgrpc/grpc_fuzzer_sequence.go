package yakgrpc

import (
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"net/http"
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

	if h.originSender != nil {
		return h.originSender(&ypb.FuzzerSequenceResponse{
			Request:  h.belong,
			Response: r,
		})
	}
	return utils.Errorf("httpFuzzerFallback is not set for sender")
}

func newHTTPFuzzerFallback(req *ypb.FuzzerRequest, server ypb.Yak_HTTPFuzzerSequenceServer) *httpFuzzerFallback {
	return &httpFuzzerFallback{
		ServerStream: server,
		originSender: server.Send,
		belong:       req,
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

func (s *Server) HTTPFuzzerSequence(seqreq *ypb.FuzzerRequests, stream ypb.Yak_HTTPFuzzerSequenceServer) error {
	reqs := seqreq.GetRequests()
	if len(reqs) <= 0 {
		return utils.Error("empty fuzzer request")
	}

	var canBeInherited = make(map[int]*ypb.FuzzerRequest)
	for index, r := range reqs {
		if index == 0 {
			continue
		}

		if r.InheritVariables {
			canBeInherited[index-1] = r
		}
	}

	var nextVar = reqs[0].GetParams()
	var nextCookies = make(map[string]string)
	for index, r := range reqs {
		r := r

		// 是否继承？
		var inheritVars = r.InheritVariables

		// 只有一个 response
		var _, forceOnlyOneResponse = canBeInherited[index]
		var beInherited = forceOnlyOneResponse
		var response *ypb.FuzzerResponse
		r.ForceOnlyOneResponse = true

		if inheritVars {
			r.Params = nextVar
		}

		var reqBytes []byte
		if r.GetRequest() != "" {
			reqBytes = []byte(r.GetRequest())
		} else {
			reqBytes = r.GetRequestRaw()
		}
		if nextCookies != nil && len(nextCookies) > 0 {
			var cookie []*http.Cookie
			for k, v := range nextCookies {
				cookie = append(cookie, &http.Cookie{
					Name:  k,
					Value: v,
				})
			}
			reqBytes = lowhttp.ReplaceHTTPPacketHeader(reqBytes, "Cookie", lowhttp.CookiesToString(cookie))
		}
		r.RequestRaw = reqBytes
		r.Request = ""

		fallback := newHTTPFuzzerFallback(r, stream)
		if r.ForceOnlyOneResponse {
			fallback.onFirstResponse = func(rsp *ypb.FuzzerResponse) {
				response = rsp
			}
		}
		err := s.HTTPFuzzer(r, fallback)
		if err != nil {
			log.Errorf("exec[%v] request failed: %s", index, err)
			return err
		}

		if forceOnlyOneResponse && response == nil {
			return utils.Errorf("force only one response not executed successfully!")
		}

		var vars = make([]*ypb.FuzzerParamItem, len(r.GetParams()))
		for _, i := range r.GetParams() {
			vars = append(vars, &ypb.FuzzerParamItem{
				Key:   i.Key,
				Value: i.Value,
				Type:  "raw",
			})
		}
		if beInherited {
			for _, kv := range response.GetExtractedResults() {
				vars = append(vars, &ypb.FuzzerParamItem{
					Key:   kv.Key,
					Value: kv.Value,
					Type:  "raw",
				})
			}
			nextVar = vars
			var cookieFromReq = lowhttp.GetHTTPPacketCookies(response.GetRequestRaw())
			for _, f := range response.RedirectFlows {
				for k, v := range lowhttp.GetHTTPPacketCookies(f.GetRequest()) {
					cookieFromReq[k] = v
				}
			}
			for k, v := range lowhttp.GetHTTPPacketCookies(response.GetResponseRaw()) {
				cookieFromReq[k] = v
			}
			nextCookies = cookieFromReq
		} else {
			nextCookies = make(map[string]string)
			if len(reqs) > index+1 {
				nextVar = reqs[index+1].GetParams()
			}
		}
	}
	return nil
}
