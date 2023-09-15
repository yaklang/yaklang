package coreplugin

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
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
	Method         string
	Path           []string
	Body           []byte
	Headers        []*ypb.KVPair
	ExpectedResult map[string]int
	StrictMode     bool
	RawHTTPRequest []byte
	Id             string
}

func NewLocalClient() (ypb.YakClient, error) {
	consts.InitilizeDatabase("", "")
	yakit.InitializeDefaultDatabaseSchema()

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

var initDB = sync.Once{}

func TestCoreMitmPlug(pluginName string, vulServer VulServerInfo, vulInfo VulInfo, client ypb.YakClient, t *testing.T) bool {
	initDB.Do(func() {
		yakit.InitialDatabase()
	})
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
			Path:             vulInfo.Path,
			Headers:          vulInfo.Headers,
			IsHttps:          vulServer.IsHttps,
			Body:             vulInfo.Body,
			IsRawHTTPRequest: len(vulInfo.RawHTTPRequest) != 0,
			RawHTTPRequest:   vulInfo.RawHTTPRequest,
			Method:           vulInfo.Method,
		},
	})

	if err != nil {
		panic(err)
	}

	var runtimeId string
	for {
		exec, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Warn(err)
		}
		if runtimeId == "" {
			runtimeId = exec.RuntimeID
		}
	}

	if runtimeId == "" {
		panic("NO RUNTIME ID SET")
	}

	var expected = make(map[string]int)
	for k := range vulInfo.ExpectedResult {
		expected[k] = 0
	}
	risks := yakit.YieldRisksByRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId)

	for risk := range risks {
		match := false
		riskInfo, _ := json.Marshal(risk)
		for k := range vulInfo.ExpectedResult {
			if vulInfo.StrictMode && (risk.TitleVerbose == k || risk.Title == k) {
				expected[k] = expected[k] + 1
				match = true
			} else if !vulInfo.StrictMode {
				if strings.Contains(string(riskInfo), k) || strings.Contains(risk.TitleVerbose, k) {
					expected[k] = expected[k] + 1
					match = true
				}
			}
		}
		if !match {
			println(riskInfo)
		}
	}

	for k, expectedCount := range vulInfo.ExpectedResult {
		if expected[k] != expectedCount {
			vulinfo := fmt.Sprintf(",VulInfo Id: %v", vulInfo.Id)
			t.Fatalf("Risk Keyword:[%v] Should Found Vul: %v but got: %v%v", k, expectedCount, expected[k], vulinfo)
			t.FailNow()
		}
	}
	return true
}

func Must(condition bool, errMsg ...string) {
	if !condition {
		if len(errMsg) > 0 {
			panic(strings.Join(errMsg, ", "))
		} else {
			panic("TESTCASE FAILED")
		}
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
