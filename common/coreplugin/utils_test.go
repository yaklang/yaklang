package coreplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	. "github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

var (
	vulAddr string
	server  VulServerInfo
)

func init() {
	var err error
	vulAddr, err = vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic("VULINBOX START ERROR")
	}
	server = VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
}

func CoreMitmPlugTest(pluginName string, vulServer VulServerInfo, vulInfo VulInfo, client ypb.YakClient, t *testing.T) bool {
	initDB.Do(func() {
		yakit.InitialDatabase()

		// mock DNSLog server
		var mockMutex sync.Mutex
		mockDomainToTokenMap := make(map[string]string, 0)
		mockTokenToResultMap := make(map[string]*tpb.DNSLogEvent, 0)

		Mock(yakit.NewDNSLogDomain).To(func() (domain string, token string, _ error) {
			mockToken := utils.RandStringBytes(16)
			host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
				mockMutex.Lock()
				defer mockMutex.Unlock()
				mockTokenToResultMap[mockToken] = &tpb.DNSLogEvent{Domain: ""}
				return []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
			})
			mockDomain := utils.HostPort(host, port)

			mockMutex.Lock()
			defer mockMutex.Unlock()
			mockDomainToTokenMap[mockDomain] = mockToken
			log.Infof("mock domain: %v, token: %v", mockDomain, mockToken)
			return mockDomain, mockToken, nil
		}).Build()
		Mock(yakit.CheckDNSLogByToken).When(func(token string, yakitInfo yakit.YakitPluginInfo, timeout ...float64) bool {
			_, ok := mockTokenToResultMap[token]
			return ok
		}).To(func(token string, yakitInfo yakit.YakitPluginInfo, timeout ...float64) ([]*tpb.DNSLogEvent, error) {
			events, ok := mockTokenToResultMap[token]
			if !ok {
				return nil, nil

			} else {
				return []*tpb.DNSLogEvent{events}, nil
			}
		}).Build()
	})

	codeBytes := GetCorePluginData(pluginName)
	if codeBytes == nil {
		t.Errorf("无法从bindata获取: %v", pluginName)
		return false
	}

	// run
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

	expected := make(map[string]int)
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

func TestCorePluginAstCompileTime(t *testing.T) {
	for _, pluginName := range GetAllCorePluginName() {
		bytes := GetCorePluginData(pluginName)
		avgDur := time.Duration(0)
		times := 3
		for i := 0; i < times; i++ {
			lexer := yak.NewYaklangLexer(antlr.NewInputStream(string(bytes)))
			lexer.RemoveErrorListeners()
			tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
			parser := yak.NewYaklangParser(tokenStream)
			parser.RemoveErrorListeners()
			now := time.Now()
			raw := parser.Program()
			_ = raw
			// println(raw.ToStringTree(parser.RuleNames, parser))
			avgDur += time.Since(now)
			fmt.Printf("[%d] ast compile time: %s \n", i, time.Since(now))
		}
		avgDur /= time.Duration(times)
		fmt.Printf("---avg ast compile time: [%s]---\n", avgDur)
		require.LessOrEqual(t, avgDur.Milliseconds(), int64(600), fmt.Sprintf("core plugin [%s] ast compile timeout", pluginName))
	}
}
