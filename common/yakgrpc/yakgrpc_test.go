package yakgrpc

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type testServer struct {
	ypb.YakServer
}

func (t *testServer) OpenPort(inputStream ypb.Yak_OpenPortServer) error {
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

	for {
		// 处理对方的 Conn
		conn, err := lis.Accept()
		if err != nil {
			log.Errorf("accept from %v failed: %s", addr, err)
			return err
		}
		defer conn.Close()

		if firstInput.GetRaw() != nil {
			_, err = conn.Write(firstInput.GetRaw())
			if err != nil {
				return err
			}
		}

		ctx, cancel := context.WithCancel(inputStream.Context())
		defer cancel()
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			}
		}()

		streamerRWC := &OpenPortServerStreamerHelperRWC{
			stream: inputStream,
		}
		wg := new(sync.WaitGroup)
		wg.Add(2)
		go func() {
			defer wg.Done()
			defer cancel()
			_, err := io.Copy(streamerRWC, conn)
			if err != nil {
				log.Errorf("stream copy from conn[%v] to grpcChannel failed: %s", conn.RemoteAddr(), err)
			}
			log.Infof("finished for conn %v <-- %v ", addr, conn.RemoteAddr())
		}()

		go func() {
			defer wg.Done()
			defer cancel()
			_, err := io.Copy(conn, streamerRWC)
			if err != nil {
				log.Errorf("stream copy from grpcChannel to conn[%v] failed: %s", conn.RemoteAddr(), err)
			}
			log.Infof("finished for conn %v --> %v ", addr, conn.RemoteAddr())
		}()
		wg.Wait()
	}
}

func (t *testServer) MITM(stream ypb.Yak_MITMServer) error {
	mServer, err := crep.NewMITMServer(
		crep.MITM_SetHTTPRequestHijack(func(isHttps bool, req *http.Request) *http.Request {
			httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
				GotConn: func(info httptrace.GotConnInfo) {
					println(info.Conn.RemoteAddr())
					panic(1)
				},
			})

			req.URL.Path = fmt.Sprintf("/test-hijacked-by-yak-test-%v", utils.RandStringBytes(20))
			req.URL.RawQuery = fmt.Sprintf("q=1&b=%v", utils.RandStringBytes(20))
			return req
		}),
	)
	if err != nil {
		return err
	}

	err = mServer.Serve(context.Background(), "127.0.0.1:8084")
	if err != nil {
		return err
	}

	log.Error("mitm call...")
	return nil
}

func TestServer_MITM(t *testing.T) {
	go func() {
		mServer, err := crep.NewMITMServer(
			crep.MITM_SetHTTPRequestHijack(func(isHttps bool, req *http.Request) *http.Request {
				req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
					GotConn: func(info httptrace.GotConnInfo) {
						println(info.Conn.RemoteAddr())
					},
				}))

				req.URL.Path = fmt.Sprintf("/test-hijacked-by-yak-test-%v", utils.RandStringBytes(20))
				req.URL.RawQuery = fmt.Sprintf("q=1&b=%v", utils.RandStringBytes(20))
				return req
			}),
		)
		if err != nil {
			log.Error(err)
			return
		}

		err = mServer.Serve(context.Background(), "127.0.0.1:8084")
		if err != nil {
			log.Error(err)
			return
		}

		return
	}()
	time.Sleep(1 * time.Second)
}

func (t *testServer) Echo(c context.Context, req *ypb.EchoRequest) (*ypb.EchoResposne, error) {
	return &ypb.EchoResposne{Result: req.GetText()}, nil
}

func TestServer(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	go func() {
		server := &testServer{}

		grpcTrans := grpc.NewServer()
		ypb.RegisterYakServer(grpcTrans, server)

		lis, err := net.Listen("tcp", utils.HostPort("localhost", port))
		if err != nil {
			log.Error(err)
			return
		}
		err = grpcTrans.Serve(lis)
		if err != nil {
			log.Error(err)
			return
		}
	}()
	time.Sleep(time.Second * 2)

	conn, err := grpc.Dial(
		utils.HostPort("127.0.0.1", port),
		grpc.WithInsecure(),
		grpc.WithNoProxy(),
	)
	if err != nil {
		log.Error(err)
		t.FailNow()
		return
	}
	defer conn.Close()

	client := ypb.NewYakClient(conn)
	rsp, err := client.Echo(context.Background(), &ypb.EchoRequest{Text: "test"})
	if err != nil {
		log.Error(err)
		t.FailNow()
		return
	}

	if rsp.Result != "test" {
		t.FailNow()
		return
	}
	println("finished echo")

	clientStream, err := client.OpenPort(context.Background())
	if err != nil {
		log.Error(err)
		t.FailNow()
		return
	}

	err = clientStream.Send(&ypb.Input{
		Host: "127.0.0.1",
		Port: 8084,
	})
	if err != nil {
		log.Error(err)
		t.FailNow()
		return
	}
	clientStream.Send(&ypb.Input{Raw: []byte("asdfasdfasdf")})

	go func() {
		for {
			clientStream.Send(&ypb.Input{Raw: []byte("111test")})
			time.Sleep(time.Second)
		}
	}()
	for {
		output, err := clientStream.Recv()
		if err != nil {
			log.Error(err)
			return
		}
		print(string(output.GetRaw()))
	}
}
