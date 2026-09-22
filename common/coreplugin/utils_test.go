package coreplugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/coreplugin"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"

	"github.com/yaklang/yaklang/common/consts"
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
	MaxRetries     int // 最大重试次数
}

// msg is used for parsing JSON messages from plugin execution
type msg struct {
	Type    string `json:"type"`
	Content struct {
		Level   string  `json:"level"`
		Data    string  `json:"data"`
		ID      string  `json:"id"`
		Process float64 `json:"progress"`
	}
}

var (
	vulAddr string
	server  VulServerInfo
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithCancel(context.Background())
	var err error
	vulAddr, err = vulinbox.NewVulinServer(ctx)
	if err != nil {
		panic(err)
	}
	server = VulServerInfo{VulServerAddr: vulAddr, IsHttps: true}
	dns := startLocalDNSLog()
	vulinbox.HandleDnsRequest = dns.ObserveDomain
	code := m.Run()
	dns.Close()
	cancel()
	os.Exit(code)
}

func CoreMitmPlugTest(pluginName string, vulServer VulServerInfo, vulInfo VulInfo, client ypb.YakClient, t *testing.T) bool {
	t.Helper()
	coreplugin.InitDBForTest()

	codeBytes := coreplugin.GetCorePluginDataWithHook(pluginName)
	if codeBytes == nil {
		t.Errorf("无法从bindata获取: %v", pluginName)
		return false
	}

	// run
	host, port, _ := utils.ParseStringToHostPort(vulServer.VulServerAddr)

	// 重试次数
	maxRetries := vulInfo.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	for i := 0; i < maxRetries; i++ {
		// Bound every single DebugPlugin run with a deadline. Without it a stalled
		// plugin execution can only be killed by the global test timeout (6m),
		// which defeats the retry logic. On timeout the stream returns an error,
		// we treat the attempt as a failure and let the retry loop handle it.
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
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
			cancel()
			if i < maxRetries-1 {
				continue
			}
			panic(err)
		}

		var runtimeId string
		for {
			exec, err := stream.Recv()
			if err != nil {
				// EOF means finished; any other error (incl. context deadline)
				// means this attempt is aborted. Break out either way; never
				// dereference exec when err != nil.
				if err != io.EOF {
					cancel()
					t.Errorf("DebugPlugin %s stream failed: %v", pluginName, err)
					return false
				}
				break
			}
			if runtimeId == "" {
				runtimeId = exec.RuntimeID
			}
		}
		cancel()

		if runtimeId == "" {
			if i < maxRetries-1 {
				continue
			}
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

		// 检查是否所有预期的漏洞都被发现
		allFound := true
		for k, expectedCount := range vulInfo.ExpectedResult {
			if expected[k] != expectedCount {
				allFound = false
				break
			}
		}

		if allFound {
			return true
		}

		// 如果不是最后一次重试，继续
		if i < maxRetries-1 {
			continue
		}

		// 最后一次重试失败，输出错误信息
		for k, expectedCount := range vulInfo.ExpectedResult {
			if expected[k] != expectedCount {
				vulinfo := fmt.Sprintf(",VulInfo Id: %v", vulInfo.Id)
				t.Fatalf("Risk Keyword:[%v] Should Found Vul: %v but got: %v%v", k, expectedCount, expected[k], vulinfo)
				t.FailNow()
			}
		}
	}

	return false
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
	t.Skip("")
	for _, pluginName := range coreplugin.GetAllCorePluginName() {
		bytes := coreplugin.GetCorePluginData(pluginName)
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
