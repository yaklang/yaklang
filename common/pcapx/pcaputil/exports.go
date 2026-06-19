package pcaputil

import (
	"github.com/yaklang/yaklang/common/utils"
)

var Exports = map[string]any{
	"StartSniff":   Sniff,
	"OpenPcapFile": OpenPcapFile,

	"pcap_bpfFilter":                    WithBPFFilter,
	"pcap_onFlowCreated":                WithOnTrafficFlowCreated,
	"pcap_onFlowClosed":                 WithOnTrafficFlowClosed,
	"pcap_onFlowDataFrameNoReassembled": WithOnTrafficFlowOnDataFrameArrived,
	"pcap_onFlowDataFrame":              WithOnTrafficFlowOnDataFrameReassembled,
	"pcap_onTLSClientHello":             WithTLSClientHello,
	"pcap_onHTTPRequest":                WithHTTPRequest,
	"pcap_onHTTPFlow":                   WithHTTPFlow,
	"pcap_everyPacket":                  WithEveryPacket,
	"pcap_debug":                        WithDebug,
	"pcap_disableAssembly":              WithDisableAssembly,
}

// StartSniff 在指定网卡上开始抓包(嗅探),通过回调选项处理捕获到的流量
// 在 yak 中通过 pcapx.StartSniff 调用，需要相应的抓包权限
// 参数:
//   - iface: 网卡名称，多个网卡用逗号分隔
//   - opts: 抓包配置项，如 pcapx.pcap_bpfFilter、pcapx.pcap_onHTTPFlow 等
//
// 返回值:
//   - 抓包过程中的错误
//
// Example:
// ```
// // 该示例为示意性用法：在 eth0 上抓取 80 端口流量(需要抓包权限)
// pcapx.StartSniff("eth0",
//
//	pcapx.pcap_bpfFilter("tcp port 80"),
//	pcapx.pcap_onHTTPFlow(func(flow, req, rsp) { println("got a http flow") }),
//
// )~
// ```
func Sniff(iface string, opts ...CaptureOption) error {
	opts = append(opts, WithDevice(utils.PrettifyListFromStringSplited(iface, ",")...), WithFile(""))
	return Start(opts...)
}

// OpenPcapFile 打开并解析一个 pcap 抓包文件，通过回调选项处理其中的流量
// 在 yak 中通过 pcapx.OpenPcapFile 调用
// 参数:
//   - filename: pcap 文件路径
//   - opts: 处理配置项，如 pcapx.pcap_onHTTPFlow 等
//
// 返回值:
//   - 解析过程中的错误
//
// Example:
// ```
// // 该示例为示意性用法：读取并解析一个 pcap 文件
// pcapx.OpenPcapFile("/tmp/capture.pcap",
//
//	pcapx.pcap_onHTTPRequest(func(flow, req) { println("got a http request") }),
//
// )~
// ```
func OpenPcapFile(filename string, opts ...CaptureOption) error {
	opts = append(opts, WithDevice(), WithFile(filename))
	return Start(opts...)
}
