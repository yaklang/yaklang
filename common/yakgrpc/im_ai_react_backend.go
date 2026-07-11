package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/imcontrol"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/metadata"
)

type imAIReActBackend struct {
	server *Server
}

func (b *imAIReActBackend) StartAIReAct(ctx context.Context) (imcontrol.AIReActStream, error) {
	if b == nil || b.server == nil {
		return nil, fmt.Errorf("yakgrpc server is not configured")
	}
	ctx, cancel := context.WithCancel(ctx)
	client := &inProcessAIReActClientStream{
		ctx:      ctx,
		cancel:   cancel,
		toServer: make(chan *ypb.AIInputEvent, 32),
		fromSrv:  make(chan *ypb.AIOutputEvent, 128),
		done:     make(chan error, 1),
	}
	server := &inProcessAIReActServerStream{
		ctx:        ctx,
		toServer:   client.toServer,
		fromServer: client.fromSrv,
	}
	go func() {
		err := b.server.StartAIReAct(server)
		close(client.fromSrv)
		client.done <- err
		close(client.done)
		cancel()
	}()
	return client, nil
}

type inProcessAIReActClientStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	toServer  chan *ypb.AIInputEvent
	fromSrv   chan *ypb.AIOutputEvent
	done      chan error
	closeOnce sync.Once
}

func (s *inProcessAIReActClientStream) Send(event *ypb.AIInputEvent) (err error) {
	if event == nil {
		return fmt.Errorf("nil AIInputEvent")
	}
	defer func() {
		if recover() != nil {
			err = io.ErrClosedPipe
		}
	}()
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.toServer <- event:
		return nil
	}
}

func (s *inProcessAIReActClientStream) Recv() (*ypb.AIOutputEvent, error) {
	ev, ok := <-s.fromSrv
	if ok {
		return ev, nil
	}
	if err, ok := <-s.done; ok && err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (s *inProcessAIReActClientStream) CloseSend() error {
	s.closeOnce.Do(func() {
		close(s.toServer)
		s.cancel()
	})
	return nil
}

type inProcessAIReActServerStream struct {
	ctx        context.Context
	toServer   <-chan *ypb.AIInputEvent
	fromServer chan<- *ypb.AIOutputEvent
}

func (s *inProcessAIReActServerStream) Recv() (*ypb.AIInputEvent, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case ev, ok := <-s.toServer:
		if !ok {
			return nil, io.EOF
		}
		return ev, nil
	}
}

func (s *inProcessAIReActServerStream) Send(event *ypb.AIOutputEvent) error {
	if event == nil {
		return fmt.Errorf("nil AIOutputEvent")
	}
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.fromServer <- event:
		return nil
	}
}

func (s *inProcessAIReActServerStream) SetHeader(metadata.MD) error {
	return nil
}

func (s *inProcessAIReActServerStream) SendHeader(metadata.MD) error {
	return nil
}

func (s *inProcessAIReActServerStream) SetTrailer(metadata.MD) {}

func (s *inProcessAIReActServerStream) Context() context.Context {
	return s.ctx
}

func (s *inProcessAIReActServerStream) SendMsg(m any) error {
	ev, ok := m.(*ypb.AIOutputEvent)
	if !ok {
		return fmt.Errorf("unexpected AIReAct server send message type %T", m)
	}
	return s.Send(ev)
}

func (s *inProcessAIReActServerStream) RecvMsg(m any) error {
	ev, ok := m.(*ypb.AIInputEvent)
	if !ok {
		return fmt.Errorf("unexpected AIReAct server recv message type %T", m)
	}
	next, err := s.Recv()
	if err != nil {
		return err
	}
	*ev = *next
	return nil
}
