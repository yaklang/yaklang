package coreplugin

import (
	"context"
	"embed"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

//go:embed base-yak-plugin/*
var basePlugin embed.FS

type PlugInfo struct {
	PlugName    string
	BinDataPath string
}

type VulServerInfo struct {
	VulServerAddr string
	IsHttps       bool
}

type VulInfo struct {
	Path           string
	Headers        []*ypb.KVPair
	ExpectedResult map[string]int
}

func NewLocalClient() (ypb.YakClient, error) {
	consts.InitilizeDatabase("", "")
	yakit.InitializeDefaultDatabase()

	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)
	grpcTrans := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	s, err := yakgrpc.NewServer()
	if err != nil {
		log.Errorf("build yakit server failed: %s", err)
		return nil, err
	}
	ypb.RegisterYakServer(grpcTrans, s)
	var lis net.Listener
	lis, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		err = grpcTrans.Serve(lis)
		if err != nil {
			log.Error(err)
		}
	}()

	time.Sleep(1 * time.Second)

	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(100*1024*1045),
		grpc.MaxCallRecvMsgSize(100*1024*1045),
	))
	if err != nil {
		return nil, err
	}
	return ypb.NewYakClient(conn), nil
}

func TestCoreMitmPlug(pluginName string, vulServer VulServerInfo, vunInfo VulInfo, client ypb.YakClient, t *testing.T) bool {
	codeBytes := GetCorePluginData(pluginName)
	if codeBytes == nil {
		t.Errorf("无法从bindata获取%v", pluginName)
		return false
	}
	host, port, _ := utils.ParseStringToHostPort(vulServer.VulServerAddr)
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       string(codeBytes),
		PluginType: "mitm",
		Input:      utils.HostPort(host, port),
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path:    []string{vunInfo.Path},
			Headers: vunInfo.Headers,
			IsHttps: vulServer.IsHttps,
		},
	})

	if err != nil {
		panic(err)
	}

	for {
		exec, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Warn(err)
		}
		if string(exec.Message) != "" {
			for expected := range vunInfo.ExpectedResult {
				if strings.Contains(string(exec.Message), expected) {
					vunInfo.ExpectedResult[expected]--
					break
				}
			}
		}
	}
	for expected, cnt := range vunInfo.ExpectedResult {
		if cnt != 0 {
			t.Errorf("`%v` 的预期检测出次数缺少了 %v 次", expected, cnt)
			return false
		}
	}
	return true
}

func Must(condition bool, errMsg string) {
	if !condition {
		panic(errMsg)
	}
}

func GetCorePluginData(name string) []byte {
	codeBytes, err := basePlugin.ReadFile(fmt.Sprintf("base-yak-plugin/%v.yak", name))
	if err != nil {
		log.Errorf("%v不是core plugin", name)
		return nil
	}
	return codeBytes
}

func ConnectVulinboxAgentEx(addr string, handler func(request []byte), onPing func(), onClose func()) (func(), error) {
	return ConnectVulinboxAgentRaw(addr, func(bytes []byte) {
		spew.Dump(bytes)
		t := strings.ToLower(utils.ExtractMapValueString(bytes, "type"))
		log.Infof(`vulinbox ws agent fetch message: %v`, t)
		switch t {
		case "ping":
			if onPing != nil {
				onPing()
			}
		case "request":
			handler([]byte(utils.ExtractMapValueString(bytes, "request")))
		}
	}, func() {
		if onClose != nil {
			onClose()
		}
		log.Infof("vulinbox agent: %v is closed", addr)
	})
}

func ConnectVulinboxAgent(addr string, handler func(request []byte), onPing ...func()) (func(), error) {
	return ConnectVulinboxAgentEx(addr, handler, func() {
		for _, i := range onPing {
			i()
		}
	}, nil)
}

func ConnectVulinboxAgentRaw(addr string, handler func([]byte), onClose func()) (func(), error) {
	var cancel = func() {}

	if addr == "" {
		addr = "127.0.0.1:8787"
	}

	host, port, _ := utils.ParseStringToHostPort(addr)
	if port <= 0 {
		host = "127.0.0.1"
		port = 8787
	} else {
		addr = utils.HostPort(host, port)
		addr = strings.ReplaceAll(addr, "0.0.0.0", "127.0.0.1")
		addr = strings.ReplaceAll(addr, "[::]", "127.0.0.1")
	}

	log.Info("start to create ws client to connect vulinbox/_/ws/agent")
	wsPacket := lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /_/ws/agent HTTP/1.1
Host: vuliobox:8787
Connection: Upgrade
Sec-WebSocket-Key: kpFli2X1YeW53YainWGFzA==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: FeedbackStreamer/1.0

`), "Host", addr)
	fmt.Println(string(wsPacket))
	var start = false
	client, err := lowhttp.NewWebsocketClient(wsPacket, lowhttp.WithWebsocketFromServerHandler(func(bytes []byte) {
		if !start {
			if utils.ExtractMapValueString(bytes, "type") == "ping" {
				start = true
			}
		}
		handler(bytes)
	}))
	if err != nil {
		cancel()
		return cancel, err
	}
	client.StartFromServer()
	cancel = func() {
		client.Stop()
	}
	log.Info("start to wait for vulinbox ws agent connected")
	if utils.Spinlock(5, func() bool {
		return start
	}) != nil {
		cancel()
		return nil, utils.Errorf("vulinbox ws agent connect timeout")
	}
	go func() {
		client.Wait()
		if onClose != nil {
			onClose()
		}
	}()
	log.Info("vulinbox ws agent connected")
	return cancel, nil
}
