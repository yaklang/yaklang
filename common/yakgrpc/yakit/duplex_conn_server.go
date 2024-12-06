package yakit

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

var YakitDuplexConnectionServer = &DuplexConnectionServer{
	Handlers:           make(map[string]DuplexConnectionHandler),
	serverHandlerMutex: sync.Mutex{},
}

type DuplexConnectionHandler func(context.Context, *ypb.DuplexConnectionRequest) error
type DuplexConnectionServer struct {
	Handlers           map[string]DuplexConnectionHandler
	serverHandlerMutex sync.Mutex
}

func (s *DuplexConnectionServer) RegisterHandler(handlerName string, handler DuplexConnectionHandler) {
	s.serverHandlerMutex.Lock()
	defer s.serverHandlerMutex.Unlock()
	s.Handlers[handlerName] = handler
}

func (s *DuplexConnectionServer) UnRegisterHandler(handlerName string) {
	s.serverHandlerMutex.Lock()
	defer s.serverHandlerMutex.Unlock()
	delete(s.Handlers, handlerName)
}

func (s *DuplexConnectionServer) Server(ctx context.Context, stream ypb.Yak_DuplexConnectionServer) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			req, err := stream.Recv()
			if err != nil {
				log.Errorf("DuplexConnectionServer Failed to receive request: %v", err)
				return
			}
			if handler, ok := s.Handlers[req.GetMessageType()]; ok {
				err := handler(ctx, req)
				if err != nil {
					log.Errorf("handle process request error : %v", err)
				}
			}

		}
	}
}
