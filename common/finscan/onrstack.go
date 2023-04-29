package finscan

import (
	"context"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"net"
	"time"
)

type rstAckHandler func(ip net.IP, port int)
type noRspHandler func(ip net.IP, port int)

func (s *Scanner) onRstAck(ip net.IP, port int) {
	s.rstAckHandlerMutex.Lock()
	defer s.rstAckHandlerMutex.Unlock()

	for _, handler := range s.rstAckHandlers {
		handler(ip, port)
	}
}

func (s *Scanner) onNoRsp(ip net.IP, port int) {
	s.noRspHandlerMutex.Lock()
	defer s.noRspHandlerMutex.Unlock()

	for _, handler := range s.noRspHandlers {
		handler(ip, port)
	}
}

func (s *Scanner) RegisterRstAckHandler(tag string, handler rstAckHandler) error {
	s.rstAckHandlerMutex.Lock()
	defer s.rstAckHandlerMutex.Unlock()

	_, ok := s.rstAckHandlers[tag]
	if ok {
		return errors.Errorf("existed handler for %v", tag)
	}

	s.rstAckHandlers[tag] = handler
	return nil
}

func (s *Scanner) RegisterNoRspHandler(tag string, handler noRspHandler) error {
	s.noRspHandlerMutex.Lock()
	defer s.noRspHandlerMutex.Unlock()

	_, ok := s.noRspHandlers[tag]
	if ok {
		return errors.Errorf("existed handler for %v", tag)
	}

	s.noRspHandlers[tag] = handler
	return nil
}

func (s *Scanner) UnregisterRstAckHandler(tag string) {
	s.rstAckHandlerMutex.Lock()
	defer s.rstAckHandlerMutex.Unlock()

	delete(s.rstAckHandlers, tag)
}

func (s *Scanner) UnregisterNoRspHandler(tag string) {
	s.noRspHandlerMutex.Lock()
	defer s.noRspHandlerMutex.Unlock()

	delete(s.noRspHandlers, tag)
}

func (s *Scanner) waitOpenPort(ctx context.Context, handler rstAckHandler, async bool) error {
	id, err := uuid.NewV4()
	if err != nil {
		return errors.Errorf("gen uuid v4 failed: %s", err)
	}
	err = s.RegisterRstAckHandler(id.String(), handler)
	if err != nil {
		return errors.Errorf("register failed: %s", err)
	}

	if async {
		go func() {
			select {
			case <-ctx.Done():
				s.UnregisterRstAckHandler(id.String())
				return
			}
		}()
		return nil
	} else {
		defer s.UnregisterRstAckHandler(id.String())
		select {
		case <-ctx.Done():
			return nil
		}
	}

}

func (s *Scanner) WaitOpenPort(ctx context.Context, handler rstAckHandler) error {
	return s.waitOpenPort(ctx, handler, false)
}

func (s *Scanner) WaitOpenPortAsync(ctx context.Context, handler rstAckHandler) error {
	return s.waitOpenPort(ctx, handler, true)
}

func (s *Scanner) WaitOpenPortWithTimeout(timeout time.Duration, handler rstAckHandler) error {
	ctx, _ := context.WithTimeout(s.ctx, timeout)
	return s.WaitOpenPort(ctx, handler)
}
