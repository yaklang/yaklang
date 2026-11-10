package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const mitmControlTimeout = 10 * time.Second

// A session owns the client side of one MITMV2 stream. Only receive reads it;
// control sends existing protocol messages and waits for their acknowledgements.
type mitmV2Session struct {
	stream  ypb.Yak_MITMV2Client
	cancel  context.CancelFunc
	ready   chan struct{}
	done    chan struct{}
	onClose func()
	once    sync.Once
	mu      sync.Mutex
	err     error
	pending *mitmV2Control
}

type mitmV2Control struct {
	match func(*ypb.MITMV2Response) bool
	done  chan struct{}
}

func (s *mitmV2Session) stop(err error) {
	s.once.Do(func() {
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		s.cancel()
		if s.onClose != nil {
			s.onClose()
		}
		close(s.done)
	})
}

func (s *mitmV2Session) closedError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Errorf("MITM stream closed: %w", s.err)
}

func (s *mitmV2Session) receive() {
	ready := false
	for {
		rsp, err := s.stream.Recv()
		if err != nil {
			s.stop(err)
			return
		}
		if rsp.GetHaveMessage() && rsp.GetMessage() != nil {
			content := handleExecMessage(string(rsp.GetMessage().GetMessage()))
			if content != "" {
				appendBackgroundStreamLog("start_mitm_v2", content)
			}
			if !ready && strings.Contains(content, "starting mitm server") {
				ready = true
				close(s.ready)
			}
		}
		s.mu.Lock()
		if s.pending != nil && s.pending.match(rsp) {
			close(s.pending.done)
			s.pending = nil
		}
		s.mu.Unlock()
	}
}

func (s *mitmV2Session) waitReady(ctx context.Context) error {
	select {
	case <-s.done:
		return s.closedError()
	default:
	}
	select {
	case <-s.ready:
		return nil
	case <-s.done:
		return s.closedError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// A stream's Send uses the session context, not the individual tool context.
// Cancel the session if a send or acknowledgement times out: without request
// IDs, a late reply must not acknowledge a subsequent command.
func (s *mitmV2Session) send(ctx context.Context, req *ypb.MITMV2Request) error {
	result := make(chan error, 1)
	go func() { result <- s.stream.Send(req) }()
	select {
	case err := <-result:
		if err != nil {
			s.stop(err)
		}
		return err
	case <-s.done:
		return s.closedError()
	case <-ctx.Done():
		s.stop(ctx.Err())
		return ctx.Err()
	}
}

// The owning MCP server serializes commands, including persistence, across its
// sessions. The receive goroutine remains free to drain logs and match replies.
func (s *mitmV2Session) control(ctx context.Context, req *ypb.MITMV2Request, match func(*ypb.MITMV2Response) bool) error {
	if err := s.waitReady(ctx); err != nil {
		return err
	}
	pending := &mitmV2Control{match: match, done: make(chan struct{})}
	s.mu.Lock()
	s.pending = pending
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.pending = nil
		s.mu.Unlock()
	}()
	if err := s.send(ctx, req); err != nil {
		return err
	}
	select {
	case <-pending.done:
		return nil
	case <-s.done:
		return s.closedError()
	case <-ctx.Done():
		s.stop(ctx.Err())
		return ctx.Err()
	}
}

func (s *MCPServer) acquireMITMControl(ctx context.Context) (func(), error) {
	s.mitmMu.Lock()
	if s.mitmClosed {
		s.mitmMu.Unlock()
		return nil, fmt.Errorf("MCP server is closed")
	}
	if s.mitmControl == nil {
		s.mitmControl = make(chan struct{}, 1)
	}
	control := s.mitmControl
	s.mitmMu.Unlock()
	select {
	case control <- struct{}{}:
		return func() { <-control }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *MCPServer) updateMITMV2(ctx context.Context, req *ypb.MITMV2Request, match func(*ypb.MITMV2Response) bool, persist func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, mitmControlTimeout)
	defer cancel()
	release, err := s.acquireMITMControl(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mitmMu.Lock()
	closed := s.mitmClosed
	sessions := make([]*mitmV2Session, 0, len(s.mitmSessions))
	for session := range s.mitmSessions {
		sessions = append(sessions, session)
	}
	s.mitmMu.Unlock()
	if closed {
		return fmt.Errorf("MCP server is closed")
	}
	if err := persist(ctx); err != nil {
		return err
	}
	for _, session := range sessions {
		if err := session.control(ctx, req, match); err != nil {
			return fmt.Errorf("failed to update running MITM: %w", err)
		}
	}
	return nil
}

func (s *MCPServer) closeMITMSessions() {
	s.mitmMu.Lock()
	s.mitmClosed = true
	sessions := make([]*mitmV2Session, 0, len(s.mitmSessions))
	for session := range s.mitmSessions {
		sessions = append(sessions, session)
	}
	s.mitmMu.Unlock()
	for _, session := range sessions {
		session.stop(context.Canceled)
	}
}
