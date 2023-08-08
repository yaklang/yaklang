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
				_, err := net.Dial("tcp", addr)
				if err == nil {
					ch <- 1
					return
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
		return nil
	}
}
