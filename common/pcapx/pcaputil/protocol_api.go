package pcaputil

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/yaklang/pcap"
)

// pcap_onProtocolMessage 注册协议消息回调，配合 StartSniff 或 OpenPcapFile 使用。
// pcapx 内置协议识别、TCP 消息分帧及字段解码，无需额外启用或加载规则。
// 一次回调对应一条协议消息或诊断事件，不对应一个原始数据包。
// 默认完整解码；message.Status 为 decoded 时可直接读取 message.Fields。
// 未知、不完整、非法或资源受限数据仍会投递，使用 Status 和 Error 区分。
// 同次捕获的回调串行执行；同一 TCP 会话内按消息顺序投递，跨会话不保证时间顺序。
// 回调返回后可以保留 Fields 和 Raw。慢回调会阻塞分析，持续流量下可能造成抓包丢包。
// 未注册消息或协议统计回调时跳过协议解析。协议订阅不能与禁用重组、抓包缓存或旧的全流 HTTP/TLS 回调混用。
//
// 参数:
//   - callback: 接收 ProtocolEvent 的函数；常用字段为 Protocol、Summary、Fields、Source、Destination、Status。最后一次配置生效，nil 取消订阅。
//
// 返回值:
//   - 抓包配置选项。
//
// Example:
// ```
//
//	pcapx.OpenPcapFile("session.pcap", pcapx.pcap_onProtocolMessage(func(message) {
//	    println(message.Protocol, message.Status, message.Summary)
//	    if message.Status == "decoded" { dump(message.Fields) }
//	}))~
//
// ```
func WithOnProtocolMessage(callback func(message *ProtocolEvent)) CaptureOption {
	return func(c *CaptureConfig) error {
		if callback == nil {
			return WithBinParser(nil)(c)
		}
		var mu sync.Mutex
		return WithBinParser(func(event *ProtocolEvent) {
			mu.Lock()
			defer mu.Unlock()
			callback(event)
		})(c)
	}
}

// pcap_onProtocolStats 注册协议分析的最终统计回调，配合 StartSniff 或 OpenPcapFile 使用。
// 分析启动后，在捕获结束并处理完已接收任务时回调一次，不提供周期统计；初始化失败时不保证回调。
// 仅注册此回调也会执行协议解析。Messages 是消息/诊断事件数，Decoded 是完整解码数，
// InputBytes 是送入协议分析的字节数，不是网卡捕获字节数；驱动丢包请使用 pcap_onTCPReassemblyStats。
//
// 参数:
//   - callback: 接收 ProtocolStats 的函数。最后一次配置生效，nil 取消统计订阅。
//
// 返回值:
//   - 抓包配置选项。
//
// Example:
// ```
//
//	pcapx.OpenPcapFile("session.pcap", pcapx.pcap_onProtocolStats(func(stats) {
//	    println(stats.Messages, stats.Decoded, stats.Unknown, stats.InputBytes)
//	}))~
//
// ```
func WithOnProtocolStats(callback func(stats ProtocolStats)) CaptureOption {
	return WithBinParserStats(callback)
}

// pcap_protocolDeferred 设置是否延迟协议字段解码，配合协议消息或统计订阅使用。
// 默认 false，消息回调直接获得完整 Fields。设为 true 时仍执行协议识别和消息分帧，
// 可解码消息的 Status 为 deferred，Fields 为空；调用 message.GetFields()~ 或历史查看器 Details 时再解码。
// 此选项本身不注册回调，也不启用无人接收的解析。
//
// 参数:
//   - deferred: true 延迟字段解码，false 在消息回调前解码。
//
// 返回值:
//   - 抓包配置选项。
//
// Example:
// ```
// history = pcapx.NewProtocolInspector()~
// pcapx.OpenPcapFile("session.pcap", pcapx.pcap_protocolDeferred(true),
//
//	pcapx.pcap_onProtocolMessage(history.OnEvent))~
//
// rows = history.Rows("http", 0)
//
//	if len(rows) > 0 {
//	    message = history.Details(rows[0].ID)~
//	    dump(message.Fields)
//	}
//
// ```
func WithProtocolDeferred(deferred bool) CaptureOption {
	return WithBinParserDeferred(deferred)
}

// NewProtocolInspector 创建有界协议消息历史，用 OnEvent 接收 pcap_onProtocolMessage 的结果。
// Rows 返回摘要，Details 按消息 ID 返回独立字段。超出条数或原始字节上限时淘汰最早消息，
// Evicted 返回淘汰数；历史淘汰不等于抓包丢包。
//
// 参数:
//   - limits: 不传参数时保留最多 4096 条、32 MiB 原始字节；自定义时必须同时传入消息条数和原始字节数，均为正数，条数最多 1000000。
//
// 返回值:
//   - 有界历史查看器。
//   - 参数错误，使用 ~ 检查。
//
// Example:
// ```
// history = pcapx.NewProtocolInspector()~
// pcapx.OpenPcapFile("session.pcap", pcapx.pcap_onProtocolMessage(history.OnEvent))~
// dump(history.Rows("http", 0)) // 0 表示所有会话
// ```
func NewProtocolInspector(limits ...int) (*ProtocolInspector, error) {
	if len(limits) == 0 {
		return NewBinParserInspector(4096, 32<<20)
	}
	if len(limits) != 2 {
		return nil, fmt.Errorf("protocol inspector accepts no limits or a message/byte pair")
	}
	return NewBinParserInspector(limits[0], limits[1])
}

// ListDevices 列出当前可抓包设备，需要本机 libpcap/Npcap 支持。
// 返回设备的 Name 可直接传给 StartSniff；Description 是设备说明，列表下标不是设备名称。
//
// 返回值:
//   - 设备列表，每项包含 Name、Description 和 Addresses 等信息。
//   - 枚举设备时的错误，使用 ~ 检查。
//
// Example:
// ```
// for _, device = range pcapx.ListDevices()~ { println(device.Name, device.Description) }
// ```
func ListDevices() ([]pcap.Interface, error) { return pcap.FindAllDevs() }

// CaptureContext 创建支持 Ctrl-C 和可选超时的捕获上下文，需通过 pcap_context 传给捕获入口。
// stop() 主动请求停止，同时释放定时器和信号订阅，应在成功创建后 defer stop()。
// StartSniff/OpenPcapFile 会处理完已接收任务再返回，因此截止时间不是强制返回时间。
//
// 参数:
//   - seconds: 持续秒数，可为小数；0 不设超时。必须非负、有限且不超过 time.Duration 范围。
//
// 返回值:
//   - 捕获上下文。
//   - 停止函数，可以重复调用。
//   - 参数错误，使用 ~ 检查。
//
// Example:
// ```
// ctx, stop = pcapx.CaptureContext(30)~
// defer stop()
// pcapx.StartSniff("en0", pcapx.pcap_context(ctx),
//
//	pcapx.pcap_onProtocolMessage(func(message) { println(message.Summary) }))~
//
// ```
func CaptureContext(seconds float64) (context.Context, context.CancelFunc, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds >= float64(math.MaxInt64)/float64(time.Second) {
		return nil, nil, fmt.Errorf("capture duration must be finite, nonnegative and fit a time.Duration")
	}
	ctx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt)
	if seconds == 0 {
		return ctx, stopSignal, nil
	}
	ctx, stopTimer := context.WithTimeout(ctx, time.Duration(seconds*float64(time.Second)))
	return ctx, func() { stopTimer(); stopSignal() }, nil
}
