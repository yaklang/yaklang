package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/imcontrol"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type imAIReActBackend struct {
	server *Server
}

// StartAIReAct retains IM's asynchronous stream contract while routing the
// stream actor directly to ReActSessionRuntime. The bounded input/output queues
// and terminal error delivery match the former in-process gRPC bridge, without
// constructing a fake ypb.Yak_StartAIReActServer.
func (b *imAIReActBackend) StartAIReAct(ctx context.Context) (imcontrol.AIReActStream, error) {
	if b == nil || b.server == nil {
		return nil, fmt.Errorf("yakgrpc server is not configured")
	}
	runtime := b.server.getReActSessionRuntime()
	if runtime == nil {
		return nil, fmt.Errorf("AI ReAct session runtime is not configured")
	}
	ctx, cancel := context.WithCancel(ctx)
	stream := &imReActRuntimeStream{
		ctx:       ctx,
		cancel:    cancel,
		runtime:   runtime,
		toRuntime: make(chan *ypb.AIInputEvent, 32),
		fromSrv:   make(chan *ypb.AIOutputEvent, 128),
		done:      make(chan error, 1),
	}
	go stream.serve()
	return stream, nil
}

type imReActRuntimeStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	runtime   ReActSessionRuntime
	toRuntime chan *ypb.AIInputEvent
	fromSrv   chan *ypb.AIOutputEvent
	done      chan error
	closeOnce sync.Once
	outputMu  sync.Mutex
}

func (s *imReActRuntimeStream) Send(event *ypb.AIInputEvent) (err error) {
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
	case s.toRuntime <- event:
		return nil
	}
}

func (s *imReActRuntimeStream) Recv() (*ypb.AIOutputEvent, error) {
	ev, ok := <-s.fromSrv
	if ok {
		return ev, nil
	}
	if err, ok := <-s.done; ok && err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (s *imReActRuntimeStream) CloseSend() error {
	s.closeOnce.Do(func() {
		close(s.toRuntime)
		s.cancel()
	})
	return nil
}

func (s *imReActRuntimeStream) serve() {
	err := s.serveRuntime()
	close(s.fromSrv)
	s.done <- err
	close(s.done)
	s.cancel()
}

func (s *imReActRuntimeStream) serveRuntime() error {
	var first *ypb.AIInputEvent
	select {
	case <-s.ctx.Done():
		err := s.ctx.Err()
		log.Errorf("recv re-act first config msg failed: %v", err)
		return fmt.Errorf("recv first mgs failed: %v", err)
	case event, ok := <-s.toRuntime:
		if !ok {
			log.Errorf("recv re-act first config msg failed: %v", io.EOF)
			return fmt.Errorf("recv first mgs failed: %v", io.EOF)
		}
		first = event
	}
	if !first.GetIsStart() {
		log.Errorf("recv re-act first config msg is invalid: %v", first)
		return fmt.Errorf("first msg is not a start/config message, set IsStart to true")
	}

	connection, err := s.runtime.Connect(s.ctx, ConnectRequest{
		StartParams: first.GetParams(),
		options: &reActConnectOptions{
			loadBuiltinTools: true,
			onEventError: func(err error) {
				log.Errorf("send re-act event to stream failed: %v", err)
			},
		},
	}, func(output *schema.AiOutputEvent) error {
		if output == nil {
			return fmt.Errorf("nil AIOutputEvent")
		}
		// The former in-process gRPC path serialized stream.Send with sendMu.
		// Preserve that single-writer ordering for concurrent Runtime emitters.
		s.outputMu.Lock()
		defer s.outputMu.Unlock()
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case s.fromSrv <- output.ToGRPC():
			return nil
		}
	})
	if err != nil {
		return err
	}
	// Do not finish the stream actor while its Runtime connection is alive.
	// Session deletion may remove durable data immediately after the actor exits.
	defer connection.Close()
	createdRuntime := connection.CreatedRuntime()

	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-connection.Done():
			return nil
		case event, ok := <-s.toRuntime:
			if !ok {
				return nil
			}
			if event.GetIsStart() {
				continue
			}
			if err := connection.Send(event); err != nil {
				if !createdRuntime && event.GetIsSyncMessage() && event.GetSyncType() == aicommon.SYNC_TYPE_RECOVERY_HISTORY {
					log.Warnf("send attached recovery history failed: %v", err)
					continue
				}
				if createdRuntime {
					log.Errorf("ReAct event processing failed: %v", err)
				} else {
					log.Warnf("forward input to running session failed: %v", err)
				}
			}
		}
	}
}
