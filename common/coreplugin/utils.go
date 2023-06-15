package coreplugin

import (
	"context"
	"github.com/yaklang/yaklang/common/bindata"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

func TestMitmPlug(plug PlugInfo, vulServer VulServerInfo, vunInfo VulInfo, client ypb.YakClient, t *testing.T) bool {
	codeBytes, err := bindata.Asset(plug.BinDataPath)
	if err != nil {
		t.Error("无法从bindata获取" + plug.PlugName)
		return false
	}
	host, port, _ := utils.ParseStringToHostPort(vulServer.VulServerAddr)
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       string(codeBytes),
		PluginType: "mitm",
		Input:      utils.HostPort(host, port),
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path:    []string{vunInfo.Path},
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
