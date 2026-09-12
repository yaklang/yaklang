package pcaputil

import (
	"github.com/yaklang/yaklang/common/utils"
)

var Exports = map[string]any{
	"StartSniff":             Sniff,
	"OpenPcapFile":           OpenPcapFile,
	"ReplayPcapFile":         ReplayPcapFile,
	"ListDevices":            ListDevices,
	"CaptureContext":         CaptureContext,
	"NewProtocolInspector":   NewProtocolInspector,
	"pcap_onProtocolMessage": WithOnProtocolMessage,
	"pcap_onProtocolStats":   WithOnProtocolStats,
	"pcap_protocolDeferred":  WithProtocolDeferred,
	"NewBinParserInspector":  NewBinParserInspector,
	"pcap_binParser":         WithBinParser,
	"pcap_binParserDeferred": WithBinParserDeferred,
	"pcap_binParserStats":    WithBinParserStats,
	"pcap_captureWriter":     WithCaptureWriter,
	"pcap_outputFile":        WithOutputFile,
	"pcap_captureBufferSize": WithCaptureBufferSize,
	"pcap_context":           WithContext,

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
	"pcap_tcpReassemblyStream":          WithTCPReassemblyStream,
	"pcap_tcpReassemblyWorkers":         WithTCPReassemblyWorkers,
	"pcap_tcpReassemblyStats":           WithTCPReassemblyStats,
	"pcap_onTCPReassemblyStats":         WithOnTCPReassemblyStats,
}

// StartSniff 在指定网卡上抓包，通过回调选项处理原始包、TCP 流或协议消息。
// 需要本机 libpcap/Npcap 和抓包权限；设备名称可从 ListDevices 获取。
// 使用 pcap_onProtocolMessage 即可订阅内置协议解析，无需额外启用开关。
// 默认持续运行；传入 pcap_context 可以取消捕获，结束前会处理完已接收任务。
// 参数:
//   - iface: 网卡名称，多个网卡用逗号分隔
//   - opts: 抓包配置项，如 pcap_bpfFilter、pcap_onProtocolMessage、pcap_onProtocolStats、pcap_outputFile
//
// 返回值:
//   - 抓包过程中的错误
//
// Example:
// ```
// ctx, stop = pcapx.CaptureContext(30)~
// defer stop()
// pcapx.StartSniff("eth0",
//
//	pcapx.pcap_context(ctx),
//	pcapx.pcap_bpfFilter("tcp port 80"),
//	pcapx.pcap_onProtocolMessage(func(message) { println(message.Protocol, message.Summary) }),
//
// )~
// ```
func Sniff(iface string, opts ...CaptureOption) error {
	opts = append(opts, WithDevice(utils.PrettifyListFromStringSplited(iface, ",")...), WithFile(""))
	return Start(opts...)
}

// OpenPcapFile 回放 pcap/pcapng 文件，与 StartSniff 共用协议消息、统计和输出选项。
// 默认使用纯 Go 文件读取器，不依赖抓包驱动；BPF 或原生句柄选项需要 libpcap/Npcap。
// 到达文件末尾或上下文被取消后，处理完已接收任务再返回；不按包时间戳等待。
// 参数:
//   - filename: pcap 或 pcapng 文件路径
//   - opts: 处理配置项，如 pcap_onProtocolMessage、pcap_onProtocolStats、pcap_context
//
// 返回值:
//   - 解析过程中的错误
//
// Example:
// ```
// pcapx.OpenPcapFile("/tmp/capture.pcap",
//
//	pcapx.pcap_onProtocolMessage(func(message) { println(message.Protocol, message.Summary) }),
//
// )~
// ```
func OpenPcapFile(filename string, opts ...CaptureOption) error {
	opts = append(opts, WithDevice(), WithFile(filename))
	return Start(opts...)
}
