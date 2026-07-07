package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/crep/trafficguard"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// countFlowByTokenAndPlugin 统计请求/URL 含 token 且 from_plugin=plugin 的流量条数。
// 用于精确判断"被过滤但命中"的流量是否以内置敏感信息检测的插件流量形式落库,
// 不依赖总条数(用户本地启用的被动扫描插件会额外生成 source_type=scan 的镜像副本)。
func countFlowByTokenAndPlugin(token, plugin string) int {
	db := consts.GetGormProjectDatabase()
	var count int64
	db.Model(&schema.HTTPFlow{}).
		Where("(request LIKE ?) OR (url LIKE ?)", "%"+token+"%", "%"+token+"%").
		Where("from_plugin = ?", plugin).
		Count(&count)
	return int(count)
}

// TestGRPCMUSTPASS_MITM_TrafficGuard_FilteredVsNotFiltered 验证内置敏感信息检测(TrafficGuard)
// 与 MITM 过滤的交互(这是核心行为约束):
//
//   - 命中敏感数据、且"未被过滤"的流量: 仍然作为普通 MITM 流量保存(source_type=mitm), 留在 MITM History TAB;
//   - 命中敏感数据、但"本应被过滤"的流量(如 .js 静态资源): 不进 MITM History, 改以"插件流量"
//     (source_type=scan + FromPlugin)形式保存, 既留存证据又不污染 MITM TAB。
//
// 断言用 QuickSearchMITMHTTPFlowCount(只数 source_type=mitm)判断是否在 MITM History,
// 用 countFlowByTokenAndPlugin(按 FromPlugin 精确匹配)判断是否以内置检测插件流量形式落库;
// 不依赖落库总数, 因为用户本地启用的被动扫描插件会对每条流量额外生成 source_type=scan 的镜像副本。
func TestGRPCMUSTPASS_MITM_TrafficGuard_FilteredVsNotFiltered(t *testing.T) {
	// 响应体内藏一个 AWS AKID(规则2, 响应方向、非 Google 域 -> 一定命中), 触发 TrafficGuard。
	const secret = "AKIAIOSFODNN7EXAMPLE"
	_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		body := "page content leak " + secret + " end"
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n" + body))
		return rsp
	})

	notFilteredToken := utils.RandStringBytes(12) // 未被过滤的请求标识(写入 X-TOKEN 头)
	filteredToken := utils.RandStringBytes(12)    // 被过滤(.js)的请求标识

	client, err := NewLocalClient()
	require.NoError(t, err)

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 结果在 onLoad 内采集到外层变量, 断言在 RunMITMTestServer 返回后(主 goroutine)进行。
	var (
		notFilteredMITM   int // 未过滤流量在 MITM History 中的数量(期望 1)
		notFilteredPlugin int // 未过滤流量被存成"内置检测插件流量"的数量(期望 0)
		filteredMITM      int // 被过滤流量在 MITM History 中的数量(期望 0)
		filteredPlugin    int // 被过滤流量被存成"内置检测插件流量"的数量(期望 >=1)
	)

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	}, func(stream ypb.Yak_MITMClient) {
		// 关键: 无论后续是否出错, 都要 cancel 以解除 RunMITMTestServer 的 Recv 阻塞(该 in-process
		// 流不会因 ctx 超时而中断, 只能靠 cancel 关闭)。
		defer cancel()
		defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()

		// 配置过滤: 排除 .js 后缀(静态资源)。注意: 裸 UpdateFilter 不会回推消息, 不能 Recv, 否则死锁。
		stream.Send(&ypb.MITMRequest{
			UpdateFilter: true,
			FilterData: &ypb.MITMFilterData{
				ExcludeSuffix: []*ypb.FilterDataItem{
					{MatcherType: "suffix", Group: []string{".js"}},
				},
			},
		})
		time.Sleep(500 * time.Millisecond)

		mockHostPort := utils.HostPort("127.0.0.1", mockPort)

		// 1) 未被过滤的敏感流量: GET /api/data (不匹配 .js)。
		notFilteredPacket := []byte("GET /api/data HTTP/1.1\r\nHost: " + mockHostPort + "\r\n\r\n")
		notFilteredPacket = lowhttp.ReplaceHTTPPacketHeader(notFilteredPacket, "X-TOKEN", notFilteredToken)
		lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(notFilteredPacket), lowhttp.WithProxy(proxy), lowhttp.WithTimeout(10*time.Second))

		// 2) 被过滤的敏感流量: GET /static/app.js (匹配 .js 后缀被过滤)。
		filteredPacket := []byte("GET /static/app.js HTTP/1.1\r\nHost: " + mockHostPort + "\r\n\r\n")
		filteredPacket = lowhttp.ReplaceHTTPPacketHeader(filteredPacket, "X-TOKEN", filteredToken)
		lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(filteredPacket), lowhttp.WithProxy(proxy), lowhttp.WithTimeout(10*time.Second))

		// 等待异步保存(TrafficGuard 扫描 + 落库)。
		for i := 0; i < 40; i++ {
			time.Sleep(200 * time.Millisecond)
			notFilteredMITM = yakit.QuickSearchMITMHTTPFlowCount(notFilteredToken)
			notFilteredPlugin = countFlowByTokenAndPlugin(notFilteredToken, trafficguard.PluginName)
			filteredMITM = yakit.QuickSearchMITMHTTPFlowCount(filteredToken)
			filteredPlugin = countFlowByTokenAndPlugin(filteredToken, trafficguard.PluginName)
			if notFilteredMITM >= 1 && filteredPlugin >= 1 {
				break
			}
		}
	})

	// 核心断言一: 命中敏感数据 + 未被过滤 -> 仍走 MITM 流量(留在 MITM History), 不被转存为插件流量。
	require.Equal(t, 1, notFilteredMITM, "命中敏感数据且未被过滤的流量必须仍作为 MITM 流量(source_type=mitm)保存, 留在 MITM History")
	require.Equal(t, 0, notFilteredPlugin, "未被过滤的命中流量不应被转存为内置检测插件流量(应留在 MITM History)")

	// 核心断言二: 命中敏感数据 + 本应被过滤 -> 不进 MITM History, 改以内置检测插件流量(source_type=scan + FromPlugin)保存。
	require.Equal(t, 0, filteredMITM, "被过滤的命中流量不应出现在 MITM History(source_type=mitm)")
	require.GreaterOrEqual(t, filteredPlugin, 1, "被过滤的命中流量应以内置检测插件流量(FromPlugin)形式保存, 而非被丢弃")
}
