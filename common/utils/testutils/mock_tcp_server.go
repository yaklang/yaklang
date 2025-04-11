package testutils

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"time"
)

func DebugMockTCPHandlerFuncContext(ctx context.Context, handlerFunc handleTCPFunc) (string, int) {
	host := "127.0.0.1"
	port := utils.GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", utils.HostPort(host, port))
	if err != nil {
		panic(err)
	}
	go func() {
		select {
		case <-ctx.Done():
		}
		lis.Close()
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := lis.Accept()
				utils.TCPNoDelay(conn)
				if err != nil {
					log.Errorf("mock tcp server accept failed: %v", err)
					return
				}
				go handlerFunc(ctx, lis, conn)
			}
		}
	}()

	err = utils.WaitConnect(utils.HostPort(host, port), 3)
	if err != nil {
		panic(err)
	}
	return "127.0.0.1", port
}

func DebugMockTCP(rsp []byte) (string, int) {
	return DebugMockTCPHandlerFuncContext(utils.TimeoutContext(time.Second*30), func(ctx context.Context, lis net.Listener, conn net.Conn) {
		_, err := conn.Write(rsp)
		if err != nil {
			log.Errorf("write tcp failed: %v", err)
		}
		_ = conn.(*net.TCPConn).CloseWrite()
		//_ = lis.Close()
	},
	)
}

func DebugMockTCPEx(handleFunc handleTCPFunc) (string, int) {
	return DebugMockTCPHandlerFuncContext(utils.TimeoutContext(time.Minute*5), handleFunc)
}
