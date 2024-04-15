package utils

import (
	"context"
	"github.com/pkg/errors"
	"net"
	"time"
)

func WaitConnect(addr string, timeout float64) error {
	ch := make(chan int)
	ctx, cancle := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := net.Dial("tcp", addr)
				if err == nil {
					ch <- 1
					return
				}
				if conn == nil {
					conn.Close()
				}
				//println("连接失败")
				time.Sleep(100 * time.Microsecond)
			}
		}
	}()

	select {
	case <-time.After(FloatSecondDuration(timeout)):
		cancle()
		//println("超时")
		return errors.New("Connection attempt timed out")
	case <-ch:
		cancle()
		return nil
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
