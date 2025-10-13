package yakgrpc

import (
	"context"
	"errors"
	"sync"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// WrapperHTTPFuzzerGroupStream adapts HTTPFuzzer responses to the group stream.
type WrapperHTTPFuzzerGroupStream struct {
	grpc.ServerStream
	fallback *httpFuzzerGroupFallback
}

func NewHTTPFuzzerGroupWrapper(fallback *httpFuzzerGroupFallback) *WrapperHTTPFuzzerGroupStream {
	return &WrapperHTTPFuzzerGroupStream{fallback: fallback, ServerStream: fallback.originStream}
}

func (w *WrapperHTTPFuzzerGroupStream) Send(r *ypb.FuzzerResponse) error {
	return w.fallback.Send(r)
}

type httpFuzzerGroupFallback struct {
	originStream ypb.Yak_HTTPFuzzerGroupServer

	sendMutex *sync.Mutex
	belong    *ypb.FuzzerRequest
}

func newHTTPFuzzerGroupFallback(senderMutex *sync.Mutex, req *ypb.FuzzerRequest, server ypb.Yak_HTTPFuzzerGroupServer) *httpFuzzerGroupFallback {
	belong := req
	if req.GetFuzzerIndex() != "" {
		belong = &ypb.FuzzerRequest{FuzzerIndex: req.GetFuzzerIndex(), FuzzerTabIndex: req.GetFuzzerTabIndex()}
	}
	return &httpFuzzerGroupFallback{
		sendMutex:    senderMutex,
		originStream: server,
		belong:       belong,
	}
}

func (h *httpFuzzerGroupFallback) Send(r *ypb.FuzzerResponse) error {
	if r == nil {
		log.Error("send empty FuzzerResponse")
		return nil
	}
	if h.originStream == nil {
		return utils.Errorf("httpFuzzerGroupFallback is not set for sender")
	}
	h.sendMutex.Lock()
	defer h.sendMutex.Unlock()
	return h.originStream.Send(&ypb.GroupHTTPFuzzerResponse{
		Request:  h.belong,
		Response: r,
	})
}

func cloneFuzzerRequest(req *ypb.FuzzerRequest) *ypb.FuzzerRequest {
	if req == nil {
		return nil
	}
	cloned, ok := proto.Clone(req).(*ypb.FuzzerRequest)
	if !ok || cloned == nil {
		return nil
	}
	return cloned
}

func (s *Server) HTTPFuzzerGroup(req *ypb.GroupHTTPFuzzerRequest, stream ypb.Yak_HTTPFuzzerGroupServer) error {
	requests := req.GetRequests()
	if len(requests) == 0 {
		return utils.Error("empty fuzzer request group")
	}

	ctx := stream.Context()
	limit := req.GetConcurrent()
	if limit <= 0 {
		limit = int64(len(requests))
	}
	swg := utils.NewSizedWaitGroup(int(limit), ctx)

	var (
		wg          sync.WaitGroup
		firstErr    error
		firstErrMux sync.Mutex
		senderMutex sync.Mutex
	)

	recordErr := func(err error) {
		if err == nil {
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		firstErrMux.Lock()
		if firstErr == nil {
			firstErr = err
		}
		firstErrMux.Unlock()
	}

	for _, origin := range requests {
		request := cloneFuzzerRequest(origin)
		if request == nil {
			log.Errorf("clone fuzzer request failed")
			continue
		}
		request.FuzzerSequenceIndex = uuid.NewString()

		wg.Add(1)
		go func(r *ypb.FuzzerRequest) {
			defer wg.Done()
			if err := swg.AddWithContext(ctx); err != nil {
				recordErr(err)
				return
			}
			defer swg.Done()

			fallback := newHTTPFuzzerGroupFallback(&senderMutex, r, stream)
			err := s.HTTPFuzzer(r, NewHTTPFuzzerGroupWrapper(fallback))
			if err != nil {
				recordErr(err)
			}
		}(request)
	}

	wg.Wait()
	return firstErr
}
