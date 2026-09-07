package yakgrpc

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 端到端复现: 配置了替换规则(未命中), 响应 Content-Type 不规范(如 text/html;charset=utf-8),
// 流量被引擎打上 [规则修改] 标记(橙色), 尽管没有任何规则命中。
// 复现路径: MITMV2(自动转发) -> replacer.Hook 对响应 FixHTTPResponse 规范化 ->
// packetModified 字节对比判定被修改 -> 保存时打 [规则修改] + 橙色。
func TestGRPCMUSTPASS_MITMV2_UnmatchedRuleMarksFlowAsModified(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(60))
	defer cancel()

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	}))

	// mock 服务器返回不规范 Content-Type(缺少规范空格), 与真实场景(martian 502 错误页)一致
	token := utils.RandSecret(16)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html;charset=utf-8")
		writer.Write([]byte(token))
	})

	started := false
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if !strings.Contains(string(rsp.GetMessage().GetMessage()), `starting mitm server`) {
			continue
		}
		started = true
		// 设置一条"替换"规则(NoReplace=false), 正则绝不可能命中响应内容
		require.NoError(t, stream.Send(&ypb.MITMV2Request{
			Host:                "127.0.0.1",
			Port:                uint32(mitmPort),
			SetContentReplacers: true,
			Replacers: []*ypb.MITMContentReplacer{
				{
					Rule:              `this-rule-never-matches-[0-9a-f]{64}`,
					Result:            `replaced`,
					EnableForResponse: true,
					EnableForHeader:   true,
					EnableForBody:     true,
					Index:             1,
					VerboseName:       "never-match-rule",
				},
			},
		}))
		// 通过 MITM 代理访问 mock 服务器
		_, err = lowhttp.HTTP(
			lowhttp.WithPacketBytes([]byte(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
Connection: close

`, utils.HostPort(host, port)))),
			lowhttp.WithProxy("http://"+utils.HostPort("127.0.0.1", mitmPort)),
		)
		require.NoError(t, err)
		cancel()
		break
	}
	require.True(t, started, "mitm server should start")

	// 查询该流量并断言标记
	flows, err := QueryHTTPFlows(utils.TimeoutContextSeconds(3), client, &ypb.QueryHTTPFlowRequest{
		Keyword: token,
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 1,
		},
	}, 1)
	require.NoError(t, err)
	require.NotEmpty(t, flows.Data, "httpflow not found")
	flow := flows.Data[0]
	require.NotContains(t, flow.Tags, "[规则修改]",
		"规则未命中时流量不应被打上 [规则修改] 标记, 实际 Tags: %v", flow.Tags)
}

// 规则未命中: 无论响应是否被 FixHTTPResponse 规范化(如 Content-Type 重组), 流量都不应被打上
// [规则修改] 标记。修复前引擎用"原始报文 vs 规则结果"做字节对比, 规范化造成的差异被误判为规则修改。
func TestGRPCMUSTPASS_MITM_UnmatchedRuleNoModifiedTag(t *testing.T) {
	runMITMRuleModifiedMatrix(t, false)
}

// 规则命中: 无论响应是否被规范化, 流量都应被打上 [规则修改] 标记, 且带上规则配置的颜色。
func TestGRPCMUSTPASS_MITM_MatchedRuleHasModifiedTagAndColor(t *testing.T) {
	runMITMRuleModifiedMatrix(t, true)
}

// 规则命中/未命中 × 响应是否被规范化 的 2×2 矩阵, 每个组合一个请求, 共 4 个请求。
// 命中规则: 规则在 FixHTTPResponse 规范化后的报文上匹配/替换, 替换结果与规范化基准不同,
// 引擎据此打 [规则修改] 标记; 颜色由 HookColor 在保存 flow 时按命中规则配置打上。
// 未命中规则: 规则 Hook 返回的规范化报文与规范化基准一致, 不应打标记。
func runMITMRuleModifiedMatrix(t *testing.T, ruleMatched bool) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(60))
	defer cancel()
	startTime := time.Now().Unix()

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream, err := client.MITM(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(mitmPort),
		SetAutoForward:   true,
		AutoForwardValue: true,
	}))

	// mock 服务器: 按 path 返回规范化/未规范化的响应, body 为固定字面量(命中规则用)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path := request.URL.Path
		body := "matrix-token" + path
		if strings.Contains(path, "normalized") {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		} else {
			// 不规范 Content-Type(缺少规范空格), 与真实场景(如 martian 502 错误页)一致
			writer.Header().Set("Content-Type", "text/html;charset=utf-8")
		}
		writer.Write([]byte(body))
	})

	// 命中规则: 替换 body 中必然存在的字面量; 未命中规则: 正则绝不可能命中
	rule := `this-rule-never-matches-[0-9a-f]{64}`
	if ruleMatched {
		rule = `matrix-token`
	}
	started := false
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if !strings.Contains(string(rsp.GetMessage().GetMessage()), `starting mitm server`) {
			continue
		}
		started = true
		require.NoError(t, stream.Send(&ypb.MITMRequest{
			Host:                "127.0.0.1",
			Port:                uint32(mitmPort),
			SetContentReplacers: true,
			Replacers: []*ypb.MITMContentReplacer{
				{
					Rule:              rule,
					Result:            `replaced`,
					Color:             "orange",
					EnableForResponse: true,
					EnableForHeader:   true,
					EnableForBody:     true,
					Index:             1,
					VerboseName:       "matrix-rule",
				},
			},
		}))
		// 通过 MITM 代理访问 mock 服务器, 规范化/未规范化各一次。
		// 客户端自身不保存 flow(否则每个请求会额外保存一条无 MITM 标记的 flow)。
		for _, path := range []string{"/normalized", "/raw"} {
			_, err = lowhttp.HTTP(
				lowhttp.WithPacketBytes([]byte(fmt.Sprintf(`GET %s HTTP/1.1
Host: %s
Connection: close

`, path, utils.HostPort(host, port)))),
				lowhttp.WithProxy("http://"+utils.HostPort("127.0.0.1", mitmPort)),
				lowhttp.WithSaveHTTPFlow(false),
			)
			require.NoError(t, err)
		}
		cancel()
		break
	}
	require.True(t, started, "mitm server should start")

	// 查询该流量并断言标记与颜色。命中规则时响应 body 已被替换, 不能用 body 内容过滤,
	// 按 mock 服务器地址过滤(端口随机唯一), 并用 AfterUpdatedAt 隔离本次测试的 flow。
	flows, err := QueryHTTPFlows(utils.TimeoutContextSeconds(3), client, &ypb.QueryHTTPFlowRequest{
		IncludeInUrl:    []string{utils.HostPort(host, port)},
		AfterUpdatedAt:  startTime,
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 10,
		},
	}, 2)
	require.NoError(t, err)
	require.Len(t, flows.Data, 2, "应保存 2 条流量(规范化/未规范化各一条)")
	for _, flow := range flows.Data {
		if ruleMatched {
			require.Contains(t, flow.Tags, yakit.HTTPFlowTagRuleEdit,
				"规则命中时流量应被打上 [规则修改] 标记, 实际 Tags: %v", flow.Tags)
			require.Contains(t, flow.Tags, "ORANGE",
				"规则命中时流量应带上规则配置的颜色, 实际 Tags: %v", flow.Tags)
		} else {
			require.NotContains(t, flow.Tags, yakit.HTTPFlowTagRuleEdit,
				"规则未命中时流量不应被打上 [规则修改] 标记, 实际 Tags: %v", flow.Tags)
			require.NotContains(t, flow.Tags, "ORANGE",
				"规则未命中时流量不应带上规则颜色, 实际 Tags: %v", flow.Tags)
		}
	}
}
