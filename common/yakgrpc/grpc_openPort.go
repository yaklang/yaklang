package yakgrpc

import (
	"context"
	"io"
	"net"
	"sync"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yakgrpc/ypb"
)

func (s *Server) OpenPort(inputStream ypb.Yak_OpenPortServer) error {
	firstInput, err := inputStream.Recv()
	if err != nil {
		return utils.Errorf("recv first openPort input failed: %s", err)
	}

	var (
		host        = "0.0.0.0"
		port uint32 = 0
	)
	if firstInput.Host != "" {
		host = firstInput.Host
	}
	if firstInput.Port > 0 {
		port = firstInput.Port
	}

	// 处理监听端口的 TCP 连接
	addr := utils.HostPort(host, port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Errorf("listen %v failed: %s", addr, err)
		return err
	}
	defer lis.Close()

	// 控制生命周期
	ctx, cancel := context.WithCancel(inputStream.Context())
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			log.Infof("client closed addr listening: %s", addr)
			lis.Close()
			cancel()
			return
		}
	}()

	log.Infof("start to listening on: %s", addr)
	// 发送第一个等待消息
	err = inputStream.Send(&ypb.Output{
		Control: true,
		Waiting: true,
	})
	if err != nil {
		return utils.Errorf("start failed...")
	}

	// 处理对方的 Conn
	conn, err := lis.Accept()
	if err != nil {
		log.Errorf("accept from %v failed: %s", addr, err)
		return err
	}
	defer conn.Close()
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		}
	}()

	if firstInput.GetRaw() != nil {
		_, err = conn.Write(firstInput.GetRaw())
		if err != nil {
			return err
		}
	}
	// 发送连接信息
	_ = inputStream.Send(&ypb.Output{
		LocalAddr:  conn.LocalAddr().String(),
		RemoteAddr: conn.RemoteAddr().String(),
	})

	wg := new(sync.WaitGroup)
	wg.Add(2)
	streamerRWC := &OpenPortServerStreamerHelperRWC{
		stream:     inputStream,
		LocalAddr:  conn.LocalAddr().String(),
		RemoveAddr: conn.RemoteAddr().String(),
	}
	go func() {
		defer wg.Done()
		defer cancel()
		_, err := io.Copy(streamerRWC, conn)
		if err != nil {
			log.Errorf("stream copy from conn[%v] to grpcChannel failed: %s", conn.RemoteAddr(), err)
		}
		log.Infof("finished for conn %v <-- %v ", addr, conn.RemoteAddr())
		streamerRWC.stream.Send(&ypb.Output{
			Control: true,
			Closed:  true,
		})
	}()

	go func() {
		defer wg.Done()
		defer cancel()
		_, err := io.Copy(conn, streamerRWC)
		if err != nil {
			log.Errorf("stream copy from grpcChannel to conn[%v] failed: %s", conn.RemoteAddr(), err)
		}
		log.Infof("finished for conn %v --> %v ", addr, conn.RemoteAddr())
		streamerRWC.stream.Send(&ypb.Output{
			Control: true,
			Closed:  true,
		})
	}()
	wg.Wait()
	_ = lis.Close()
	return nil
}
