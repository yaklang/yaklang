package yaklib

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

// YakitGRPCServer is a local gRPC alternative to the HTTP webhook server used by Yak subprocesses
// to deliver progress/log/risk messages back to ScanNode.
//
// It intentionally reuses the existing JSON payload format produced by YakitMessageGenerator
// (YakitMessage -> {type, content}), but transports it through a long-lived gRPC stream to
// reduce per-message HTTP overhead.
type YakitGRPCServer struct {
	port int
	addr string
	lis  net.Listener
	srv  *grpc.Server

	progressHandler func(id string, progress float64)
	logHandler      func(level string, info string)
}

func SetYakitGRPCServer_ProgressHandler(h func(id string, progress float64)) func(s *YakitGRPCServer) {
	return func(s *YakitGRPCServer) {
		s.progressHandler = h
	}
}

func SetYakitGRPCServer_LogHandler(h func(level string, info string)) func(s *YakitGRPCServer) {
	return func(s *YakitGRPCServer) {
		s.logHandler = h
	}
}

func NewYakitGRPCServer(port int, opts ...func(server *YakitGRPCServer)) *YakitGRPCServer {
	var err error
	if port <= 0 {
		port, err = utils.GetRangeAvailableTCPPort(50000, 65535, 3)
		if err != nil {
			port = utils.GetRandomAvailableTCPPort()
		}
	}
	s := &YakitGRPCServer{port: port}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *YakitGRPCServer) Start() {
	if s == nil {
		return
	}
	if s.srv != nil {
		return
	}
	addr := utils.HostPort("127.0.0.1", s.port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		// Retry with a different port.
		s.port = utils.GetRandomAvailableTCPPort()
		addr = utils.HostPort("127.0.0.1", s.port)
		lis, err = net.Listen("tcp", addr)
		if err != nil {
			log.Errorf("yakit grpc webhook listen failed: %v", err)
			return
		}
	}
	s.addr = addr
	s.lis = lis
	s.srv = grpc.NewServer(
		grpc.MaxRecvMsgSize(64*1024*1024),
		grpc.MaxSendMsgSize(64*1024*1024),
	)
	s.srv.RegisterService(&yakitWebhookServiceDesc, s)
	go func() {
		if err := s.srv.Serve(lis); err != nil {
			log.Errorf("yakit grpc webhook serve failed: %v", err)
		}
	}()
}

func (s *YakitGRPCServer) Addr() string {
	if s == nil || s.addr == "" {
		return ""
	}
	return "grpc://" + s.addr
}

func (s *YakitGRPCServer) Shutdown() {
	if s == nil {
		return
	}
	if s.srv != nil {
		s.srv.Stop()
		s.srv = nil
	}
	if s.lis != nil {
		_ = s.lis.Close()
		s.lis = nil
	}
}

func (s *YakitGRPCServer) handleRaw(raw []byte) {
	if len(raw) == 0 {
		return
	}
	var msg YakitMessage
	_ = json.Unmarshal(raw, &msg)
	switch strings.ToLower(msg.Type) {
	case "progress", "prog":
		if s.progressHandler == nil {
			return
		}
		var prog YakitProgress
		if err := json.Unmarshal(msg.Content, &prog); err != nil {
			log.Errorf("unmarshal progress failed: %s", err)
			return
		}
		s.progressHandler(prog.Id, prog.Progress)
	case "log":
		if s.logHandler == nil {
			return
		}
		var logInfo YakitLog
		if err := json.Unmarshal(msg.Content, &logInfo); err != nil {
			log.Errorf("unmarshal log failed: %s", err)
			return
		}
		s.logHandler(logInfo.Level, logInfo.Data)
	}
}

// ---- gRPC service plumbing (manual ServiceDesc, reusing ypb.ExecResult as transport envelope) ----

var yakitWebhookServiceDesc = grpc.ServiceDesc{
	ServiceName: "yaklib.YakitWebhook",
	// HandlerType must be a pointer to an interface type, otherwise grpc.RegisterService
	// will panic when calling reflect.Type.Implements().
	HandlerType: (*yakitWebhookService)(nil),
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Push",
			Handler:       yakitWebhookPushHandler,
			ClientStreams: true,
		},
	},
	Methods: []grpc.MethodDesc{},
}

// We only need an interface type here to satisfy grpc.RegisterService's type checks.
// The actual request handling is fully driven by the StreamDesc handlers above.
type yakitWebhookService interface{}

func yakitWebhookPushHandler(srv interface{}, stream grpc.ServerStream) error {
	s, ok := srv.(*YakitGRPCServer)
	if !ok || s == nil {
		return utils.Error("invalid yakit grpc webhook server")
	}
	for {
		var in ypb.ExecResult
		err := stream.RecvMsg(&in)
		if err == io.EOF {
			_ = stream.SendMsg(&ypb.Empty{})
			return nil
		}
		if err != nil {
			return err
		}
		if len(in.Message) > 0 {
			s.handleRaw(in.Message)
		}
	}
}

type grpcWebhookSender struct {
	addr string

	mu     sync.Mutex
	conn   *grpc.ClientConn
	stream grpc.ClientStream
}

func newGRPCWebhookSender(addr string) *grpcWebhookSender {
	return &grpcWebhookSender{addr: addr}
}

func (s *grpcWebhookSender) closeLocked() {
	if s.stream != nil {
		_ = s.stream.CloseSend()
		s.stream = nil
	}
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

func (s *grpcWebhookSender) ensureStreamLocked(ctx context.Context) error {
	if s.addr == "" {
		return nil
	}
	if s.conn == nil {
		conn, err := grpc.DialContext(
			ctx,
			s.addr,
			grpc.WithInsecure(),
			grpc.WithDefaultCallOptions(
				grpc.MaxCallSendMsgSize(64*1024*1024),
				grpc.MaxCallRecvMsgSize(64*1024*1024),
			),
		)
		if err != nil {
			return err
		}
		s.conn = conn
	}
	if s.stream == nil {
		desc := &grpc.StreamDesc{ClientStreams: true, ServerStreams: false}
		st, err := s.conn.NewStream(ctx, desc, "/yaklib.YakitWebhook/Push")
		if err != nil {
			return err
		}
		s.stream = st
	}
	return nil
}

func (s *grpcWebhookSender) sendRawMessage(raw []byte) error {
	if s == nil || s.addr == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	if err := s.ensureStreamLocked(ctx); err != nil {
		s.closeLocked()
		return err
	}

	msg := &ypb.ExecResult{IsMessage: true, Message: raw}
	if err := s.stream.SendMsg(msg); err != nil {
		// Reset stream so next send can re-dial/re-open.
		s.closeLocked()
		return err
	}
	return nil
}
