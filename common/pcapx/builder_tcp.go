package pcapx

import (
	"bytes"
	"github.com/asaskevich/govalidator"
	"github.com/davecgh/go-spew/spew"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

const (
	TCP_FLAG_FIN = 1 << iota
	TCP_FLAG_SYN
	TCP_FLAG_RST
	TCP_FLAG_PSH
	TCP_FLAG_ACK
	TCP_FLAG_URG
	TCP_FLAG_ECE
	TCP_FLAG_CWR
	TCP_FLAG_NS
)

var tcpOptions = map[string]any{
	"tcp_srcPort":             WithTCP_SrcPort,
	"tcp_dstPort":             WithTCP_DstPort,
	"tcp_seq":                 WithTCP_Seq,
	"tcp_ack":                 WithTCP_Ack,
	"tcp_dataOffset":          WithTCP_DataOffset,
	"tcp_flag":                WithTCP_Flags,
	"tcp_optionMSS":           WithTCP_OptionMSS,
	"tcp_optionWindowScale":   WithTCP_OptionWindowScale,
	"tcp_optionSACKPermitted": WithTCP_OptionSACKPermitted,
	"tcp_optionSACK":          WithTCP_OptionSACK,
	"tcp_optionTimestamp":     WithTCP_OptionTimestamp,
	"tcp_optionRaw":           WithTCP_Options,
	"tcp_window":              WithTCP_Window,
	"tcp_urgent":              WithTCP_Urgent,

	"TCP_FLAG_FIN": TCP_FLAG_FIN,
	"TCP_FLAG_SYN": TCP_FLAG_SYN,
	"TCP_FLAG_RST": TCP_FLAG_RST,
	"TCP_FLAG_PSH": TCP_FLAG_PSH,
	"TCP_FLAG_ACK": TCP_FLAG_ACK,
	"TCP_FLAG_URG": TCP_FLAG_URG,
	"TCP_FLAG_ECE": TCP_FLAG_ECE,
	"TCP_FLAG_CWR": TCP_FLAG_CWR,
	"TCP_FLAG_NS":  TCP_FLAG_NS,
}

func init() {
	for k, v := range tcpOptions {
		Exports[k] = v
	}
}

type TCPOption func(config *layers.TCP) error

// tcp_flag 设置 TCP 头部的标志位(可用字符串、"|"/"," 分隔的组合或整数)
// 在 yak 中通过 pcapx.tcp_flag 调用，可配合 pcapx.TCP_FLAG_SYN 等常量使用
// 参数:
//   - in: 标志位，如 "syn"、"syn|ack" 或整数组合
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：构造 SYN 包标志
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_flag("syn"))~
// println(len(raw))
// ```
func WithTCP_Flags(in any) TCPOption {
	return func(config *layers.TCP) error {
		var flagStr []string
		if ret := utils.InterfaceToString(in); govalidator.IsInt(ret) {
			var i = codec.Atoi(ret)
			config.FIN = i&TCP_FLAG_FIN > 0
			config.SYN = i&TCP_FLAG_SYN > 0
			config.RST = i&TCP_FLAG_RST > 0
			config.PSH = i&TCP_FLAG_PSH > 0
			config.ACK = i&TCP_FLAG_ACK > 0
			config.URG = i&TCP_FLAG_URG > 0
			config.ECE = i&TCP_FLAG_ECE > 0
			config.CWR = i&TCP_FLAG_CWR > 0
			config.NS = i&TCP_FLAG_NS > 0
			return nil
		} else if strings.Contains(ret, "|") {
			flagStr = utils.PrettifyListFromStringSplited(ret, "|")
		} else {
			flagStr = utils.PrettifyListFromStringSplited(ret, ",")
		}
		for _, flag := range flagStr {
			switch strings.ToLower(flag) {
			case "fin":
				config.FIN = true
			case "syn":
				config.SYN = true
			case "rst":
				config.RST = true
			case "psh":
				config.PSH = true
			case "ack":
				config.ACK = true
			case "urg":
				config.URG = true
			case "ece":
				config.ECE = true
			case "cwr":
				config.CWR = true
			case "ns":
				config.NS = true
			}
		}
		return nil
	}
}

// tcp_srcPort 设置 TCP 头部的源端口
// 在 yak 中通过 pcapx.tcp_srcPort 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - srcPort: 源端口号(0-65535)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 源端口
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80))~
// println(len(raw))
// ```
func WithTCP_SrcPort(srcPort any) TCPOption {
	return func(config *layers.TCP) error {
		config.SrcPort = layers.TCPPort(utils.InterfaceToInt(srcPort))
		return nil
	}
}

// tcp_dstPort 设置 TCP 头部的目的端口
// 在 yak 中通过 pcapx.tcp_dstPort 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - dstPort: 目的端口号(0-65535)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 目的端口
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80))~
// println(len(raw))
// ```
func WithTCP_DstPort(dstPort any) TCPOption {
	return func(config *layers.TCP) error {
		config.DstPort = layers.TCPPort(utils.InterfaceToInt(dstPort))
		return nil
	}
}

// tcp_seq 设置 TCP 头部的序列号(Sequence Number)
// 在 yak 中通过 pcapx.tcp_seq 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - seq: 序列号
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 序列号
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_seq(1000))~
// println(len(raw))
// ```
func WithTCP_Seq(seq any) TCPOption {
	return func(config *layers.TCP) error {
		config.Seq = uint32(utils.InterfaceToInt(seq))
		return nil
	}
}

// tcp_ack 设置 TCP 头部的确认号(Acknowledgment Number)
// 在 yak 中通过 pcapx.tcp_ack 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - ack: 确认号
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 确认号
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_ack(1001))~
// println(len(raw))
// ```
func WithTCP_Ack(ack any) TCPOption {
	return func(config *layers.TCP) error {
		config.Ack = uint32(utils.InterfaceToInt(ack))
		return nil
	}
}

// tcp_dataOffset 设置 TCP 头部的数据偏移(Data Offset，即首部长度)
// 在 yak 中通过 pcapx.tcp_dataOffset 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - dataOffset: 数据偏移值(以 4 字节为单位)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 数据偏移
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_dataOffset(5))~
// println(len(raw))
// ```
func WithTCP_DataOffset(dataOffset any) TCPOption {
	return func(config *layers.TCP) error {
		config.DataOffset = uint8(utils.InterfaceToInt(dataOffset))
		return nil
	}
}

// tcp_window 设置 TCP 头部的窗口大小(Window Size)
// 在 yak 中通过 pcapx.tcp_window 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - window: 窗口大小(0-65535)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 窗口大小
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_window(8192))~
// println(len(raw))
// ```
func WithTCP_Window(window any) TCPOption {
	return func(config *layers.TCP) error {
		config.Window = uint16(utils.InterfaceToInt(window))
		return nil
	}
}

// tcp_urgent 设置 TCP 头部的紧急指针(Urgent Pointer)
// 在 yak 中通过 pcapx.tcp_urgent 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - urgent: 紧急指针值
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 紧急指针
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_urgent(0))~
// println(len(raw))
// ```
func WithTCP_Urgent(urgent any) TCPOption {
	return func(config *layers.TCP) error {
		config.Urgent = uint16(utils.InterfaceToInt(urgent))
		return nil
	}
}

// tcp_optionRaw 向 TCP 头部追加一个自定义选项，optionType 为 nil 时清空全部选项
// 在 yak 中通过 pcapx.tcp_optionRaw 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - optionType: 选项类型，传 nil 表示清空所有选项
//   - data: 选项数据字节(长度 +2 不能超过 255)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：追加一个原始 TCP 选项
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_optionRaw(8, []byte{0x00, 0x00}))~
// println(len(raw))
// ```
func WithTCP_Options(optionType any, data []byte) TCPOption {
	return func(config *layers.TCP) error {
		if len(data)+2 > 255 {
			log.Warnf("tcp option data length is too long, max length is 255, got %d, data: %v", len(data), spew.Sdump(data))
			return nil
		}
		if optionType == nil {
			config.Options = nil
		} else {
			config.Options = append(config.Options, layers.TCPOption{
				OptionType:   layers.TCPOptionKind(utils.InterfaceToInt(optionType)),
				OptionLength: uint8(len(data)) + 2,
				OptionData:   data,
			})
		}
		return nil
	}
}

// tcp_optionMSS 设置 TCP 的 MSS(最大报文段长度)选项，默认 1460
// 在 yak 中通过 pcapx.tcp_optionMSS 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: MSS 值
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP MSS 选项
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_optionMSS(1460))~
// println(len(raw))
// ```
func WithTCP_OptionMSS(i any) TCPOption {
	return func(pv4 *layers.TCP) error {
		newOpt := layers.TCPOption{
			OptionType:   layers.TCPOptionKindMSS,
			OptionLength: 4,
			OptionData:   utils.NetworkByteOrderUint16ToBytes(i),
		}
		var targetIndex = -1
		for index, p := range pv4.Options {
			if p.OptionType == layers.TCPOptionKindMSS {
				targetIndex = index
			}
		}
		if targetIndex > -1 {
			pv4.Options[targetIndex] = newOpt
		} else {
			pv4.Options = append(pv4.Options, newOpt)
		}
		return nil
	}
}

// tcp_optionWindowScale 设置 TCP 的窗口缩放(Window Scale)选项
// 在 yak 中通过 pcapx.tcp_optionWindowScale 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 窗口缩放因子
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 窗口缩放选项
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_optionWindowScale(7))~
// println(len(raw))
// ```
func WithTCP_OptionWindowScale(i any) TCPOption {
	return func(pv4 *layers.TCP) error {
		newOpt := layers.TCPOption{
			OptionType:   layers.TCPOptionKindWindowScale,
			OptionLength: 3,
			OptionData:   utils.NetworkByteOrderUint8ToBytes(i),
		}

		var targetIndex = -1
		for index, p := range pv4.Options {
			if p.OptionType == layers.TCPOptionKindWindowScale {
				targetIndex = index
			}
		}
		if targetIndex > -1 {
			pv4.Options[targetIndex] = newOpt
		} else {
			pv4.Options = append(pv4.Options, newOpt)
		}
		return nil
	}
}

// tcp_optionSACKPermitted 设置 TCP 的 SACK Permitted(允许选择性确认)选项
// 在 yak 中通过 pcapx.tcp_optionSACKPermitted 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - 无
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：开启 TCP SACK Permitted 选项
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_optionSACKPermitted())~
// println(len(raw))
// ```
func WithTCP_OptionSACKPermitted() TCPOption {
	var kind layers.TCPOptionKind = layers.TCPOptionKindSACKPermitted
	return func(pv4 *layers.TCP) error {
		for _, p := range pv4.Options {
			if p.OptionType == kind {
				return nil
			}
		}
		pv4.Options = append(pv4.Options, layers.TCPOption{
			OptionType:   kind,
			OptionLength: 2,
		})
		return nil
	}
}

// tcp_optionSACK 设置 TCP 的 SACK(选择性确认)选项，可传入多个边界值
// 在 yak 中通过 pcapx.tcp_optionSACK 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 一组 SACK 边界值(每个占 4 字节)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP SACK 选项
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_optionSACK(1000, 2000))~
// println(len(raw))
// ```
func WithTCP_OptionSACK(i ...any) TCPOption {
	return func(pv4 *layers.TCP) error {
		if len(i) <= 0 {
			return nil
		}

		var buf bytes.Buffer
		for _, v := range i {
			buf.Write(utils.NetworkByteOrderUint32ToBytes(v))
		}
		newOpt := layers.TCPOption{
			OptionType:   layers.TCPOptionKindSACK,
			OptionLength: 2 + uint8(len(i))*8,
			OptionData:   buf.Bytes(),
		}

		var targetIndex = -1
		for index, p := range pv4.Options {
			if p.OptionType == layers.TCPOptionKindSACK {
				targetIndex = index
			}
		}

		if targetIndex > -1 {
			pv4.Options[targetIndex] = newOpt
		} else {
			pv4.Options = append(pv4.Options, newOpt)
		}
		return nil
	}
}

// tcp_optionTimestamp 设置 TCP 的 Timestamp(时间戳)选项
// 在 yak 中通过 pcapx.tcp_optionTimestamp 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 时间戳值
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 TCP 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 时间戳选项
// raw = pcapx.PacketBuilder(pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_optionTimestamp(123456))~
// println(len(raw))
// ```
func WithTCP_OptionTimestamp(i any) TCPOption {
	return func(pv4 *layers.TCP) error {
		var targetIndex = -1
		for index, p := range pv4.Options {
			if p.OptionType == layers.TCPOptionKindTimestamps {
				targetIndex = index
			}
		}
		opt := layers.TCPOption{
			OptionType:   layers.TCPOptionKindTimestamps,
			OptionLength: 10,
			OptionData:   utils.NetworkByteOrderUint32ToBytes(i),
		}
		if targetIndex > -1 {
			pv4.Options[targetIndex] = opt
		} else {
			pv4.Options = append(pv4.Options, opt)
		}
		return nil
	}
}
