package synscan

import (
	"context"
	"github.com/pkg/errors"
	"net"
)

type arpHandler func(ip net.IP, addr net.HardwareAddr)

func (s *Scanner) RegisterARPHandler(dst string, handler arpHandler) error {
	s.arpHandlerMutex.Lock()
	defer s.arpHandlerMutex.Unlock()

	_, ok := s.arpHandlers[dst]
	if ok {
		return errors.Errorf("existed handler for: %s", dst)
	}

	s.arpHandlers[dst] = handler
	return nil
}

func (s *Scanner) UnregisterARPHandler(dst string) {
	s.arpHandlerMutex.Lock()
	defer s.arpHandlerMutex.Unlock()

	delete(s.arpHandlers, dst)
}

func (s *Scanner) waitForArpResponse(ctx context.Context, dst net.IP) (net.HardwareAddr, error) {
	var targetHardware net.HardwareAddr

	foundResultCtx, cancel := context.WithCancel(ctx)

	err := s.RegisterARPHandler(dst.String(), func(ip net.IP, addr net.HardwareAddr) {
		if ip.String() == dst.String() {
			targetHardware = addr
			cancel()
		}
	})
	if err != nil {
		return nil, errors.Errorf("register arp handler failed: %s", err)
	}
	defer s.UnregisterARPHandler(dst.String())

	select {
	case <-foundResultCtx.Done():
		if targetHardware != nil {
			return targetHardware, nil
		} else {
			return nil, errors.Errorf("timeout or cannot found arp response for %v", dst.String())
		}
	}
}

func (s *Scanner) onARP(ip net.IP, hw net.HardwareAddr) {
	s.arpHandlerMutex.Lock()
	defer s.arpHandlerMutex.Unlock()

	for _, r := range s.arpHandlers {
		r(ip, hw)
	}
}
