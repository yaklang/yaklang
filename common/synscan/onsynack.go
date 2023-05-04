package synscan

import (
	"context"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"net"
	"time"
)

type synAckHandler func(ip net.IP, port int)

func (s *Scanner) onSynAck(ip net.IP, port int) {
	s.synAckHandlerMutex.Lock()
	defer s.synAckHandlerMutex.Unlock()

	//if !utils.IsLoopback(ip.String()) {
	//	lrs, loopback, err := s.createRstTCP(ip, port, nil)
	//	if err != nil {
	//		log.Errorf("send rst tcp failed: %s", err)
	//		return
	//	}
	//	s.inject(loopback, lrs...)
	//}

	for _, handler := range s.synAckHandlers {
		handler(ip, port)
	}
}

func (s *Scanner) RegisterSynAckHandler(tag string, handler synAckHandler) error {
	s.synAckHandlerMutex.Lock()
	defer s.synAckHandlerMutex.Unlock()

	_, ok := s.synAckHandlers[tag]
	if ok {
		return errors.Errorf("existed handler for %v", tag)
	}

	s.synAckHandlers[tag] = handler
	return nil
}

func (s *Scanner) UnregisterSynAckHandler(tag string) {
	s.synAckHandlerMutex.Lock()
	defer s.synAckHandlerMutex.Unlock()

	delete(s.synAckHandlers, tag)
}

func (s *Scanner) waitOpenPort(ctx context.Context, handler synAckHandler, async bool) error {
	id, err := uuid.NewV4()
	if err != nil {
		return errors.Errorf("gen uuid v4 failed: %s", err)
	}
	err = s.RegisterSynAckHandler(id.String(), handler)
	if err != nil {
		return errors.Errorf("register failed: %s", err)
	}

	if async {
		go func() {
			select {
			case <-ctx.Done():
				s.UnregisterSynAckHandler(id.String())
				return
			}
		}()
		return nil
	} else {
		defer s.UnregisterSynAckHandler(id.String())
		select {
		case <-ctx.Done():
			return nil
		}
	}
	//
	//defer s.UnregisterSynAckHandler(id.String())
	//select {
	//case <-ctx.Done():
	//	return nil
	//}
}

func (s *Scanner) WaitOpenPort(ctx context.Context, handler synAckHandler) error {
	return s.waitOpenPort(ctx, handler, false)
}

func (s *Scanner) WaitOpenPortAsync(ctx context.Context, handler synAckHandler) error {
	return s.waitOpenPort(ctx, handler, true)
}

func (s *Scanner) WaitOpenPortWithTimeout(timeout time.Duration, handler synAckHandler) error {
	ctx, _ := context.WithTimeout(s.ctx, timeout)
	return s.WaitOpenPort(ctx, handler)
}
