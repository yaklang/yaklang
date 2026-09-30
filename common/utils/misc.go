package utils

import (
	"context"
	"github.com/pkg/errors"
	"net"
	"time"
)

func WaitConnect(addr string, timeout float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), FloatSecondDuration(timeout))
	defer cancel()
	dialer := net.Dialer{}
	for {
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			// A readiness probe must not leave a connection behind. In particular,
			// a gRPC server cannot gracefully stop with an unfinished handshake.
			_ = conn.Close()
			return nil
		}
		timer := time.NewTimer(100 * time.Microsecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.New("Connection attempt timed out")
		case <-timer.C:
		}
	}
}

func GetLastElement[T any](list []T) T {
	l := len(list)
	if l == 0 {
		var zero T
		return zero
	} else {
		return list[l-1]
	}
}
