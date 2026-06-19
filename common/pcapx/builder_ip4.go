package pcapx

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"strings"
)

var ipv4LayerExports = map[string]any{
	"ipv4_tos":               WithIPv4_TOS,
	"ipv4_id":                WithIPv4_ID,
	"ipv4_nextLayerProtocol": WithIPv4_NextProtocol,
	"ipv4_srcIp":             WithIPv4_SrcIP,
	"ipv4_dstOp":             WithIPv4_DstIP,
	"ipv4_ttl":               WithIPv4_TTL,
	"ipv4_flags":             WithIPv4_Flags,
	"ipv4_fragment":          WithIPv4_FragmentOffset,
	"ipv4_option":            WithIPv4_Option,

	// consts
	"IPV4_FLAG_EVIL_BIT":      int(layers.IPv4EvilBit),
	"IPV4_FLAG_DONT_FRAGMENT": int(layers.IPv4DontFragment),
	"IPV4_FLAG_MORE_FRAGMENT": int(layers.IPv4MoreFragments),

	"IPV4_PROTOCOL_TCP":      int(layers.IPProtocolTCP),
	"IPV4_PROTOCOL_UDP":      int(layers.IPProtocolUDP),
	"IPV4_PROTOCOL_ICMP":     int(layers.IPProtocolICMPv4),
	"IPV4_PROTOCOL_IGMP":     int(layers.IPProtocolIGMP),
	"IPV4_PROTOCOL_GRE":      int(layers.IPProtocolGRE),
	"IPV4_PROTOCOL_SCTP":     int(layers.IPProtocolSCTP),
	"IPV4_PROTOCOL_OSPF":     int(layers.IPProtocolOSPF),
	"IPV4_PROTOCOL_IPIP":     int(layers.IPProtocolIPIP),
	"IPV4_PROTOCOL_VRRP":     int(layers.IPProtocolVRRP),
	"IPV4_PROTOCOL_UDPLITE":  int(layers.IPProtocolUDPLite),
	"IPV4_PROTOCOL_MPLSINIP": int(layers.IPProtocolMPLSInIP),
	"IPV4_PROTOCOL_ETHERIP":  int(layers.IPProtocolEtherIP),
	"IPV4_PROTOCOL_AH":       int(layers.IPProtocolAH),
	"IPV4_PROTOCOL_ESP":      int(layers.IPProtocolESP),
}

func init() {
	for k, v := range ipv4LayerExports {
		Exports[k] = v
	}
}

type IPv4Option func(pv4 *layers.IPv4) error

func NewDefaultIPv4Layer() *layers.IPv4 {
	return &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		Options: []layers.IPv4Option{
			{
				OptionType:   layers.TCPOptionKindMSS,
				OptionLength: 4,
				OptionData:   []byte{0x05, 0xb4},
			},
			{
				OptionType:   layers.TCPOptionKindWindowScale,
				OptionLength: 4,
				OptionData:   []byte{0x07},
			},
		},
	}
}

func NewDefaultTCPLayer() *layers.TCP {
	return &layers.TCP{
		SrcPort: 80,
		DstPort: 80,
		Options: []layers.TCPOption{
			{
				OptionType:   layers.TCPOptionKindMSS,
				OptionLength: 4,
				OptionData:   []byte{0x05, 0xb4},
			},
			{
				OptionType:   layers.TCPOptionKindWindowScale,
				OptionLength: 4,
				OptionData:   []byte{0x07},
			},
		},
	}
}

/*
// IPv4 is the header of an IP packet.
type IPv4 struct {
	BaseLayer
	Version    uint8
	IHL        uint8
	TOS        uint8
	Length     uint16
	Id         uint16
	Flags      IPv4Flag
	FragOffset uint16
	TTL        uint8
	Protocol   IPProtocol
	Checksum   uint16
	SrcIP      net.IP
	DstIP      net.IP
	Options    []IPv4Option
	Padding    []byte
}

一般来说，不需要操作的字段有：IHL / Length
*/

// ipv4_tos 设置 IPv4 头部的 TOS(服务类型)字段
// 在 yak 中通过 pcapx.ipv4_tos 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: TOS 值(0-255)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 IPv4 TOS
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"), pcapx.ipv4_tos(0))~
// println(len(raw))
// ```
func WithIPv4_TOS(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		pv4.TOS = uint8(utils.InterfaceToInt(i))
		return nil
	}
}

// ipv4_id 设置 IPv4 头部的标识(Identification)字段
// 在 yak 中通过 pcapx.ipv4_id 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 标识值(0-65535)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 IPv4 标识
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"), pcapx.ipv4_id(12345))~
// println(len(raw))
// ```
func WithIPv4_ID(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		pv4.Id = uint16(utils.InterfaceToInt(i))
		return nil
	}
}

func WithIPv4_Version(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		pv4.Version = uint8(utils.InterfaceToInt(i))
		return nil
	}
}

// ipv4_flags 设置 IPv4 头部的标志位(如 DF、MF 等)
// 在 yak 中通过 pcapx.ipv4_flags 调用，可配合 pcapx.IPV4_FLAG_DONT_FRAGMENT 等常量使用
// 参数:
//   - i: 标志位值，取 layers.IPv4Flag 或整数
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 IPv4 不分片标志
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"), pcapx.ipv4_flags(pcapx.IPV4_FLAG_DONT_FRAGMENT))~
// println(len(raw))
// ```
func WithIPv4_Flags(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		switch f := i.(type) {
		case layers.IPv4Flag:
			pv4.Flags = f
		case int:
			pv4.Flags = layers.IPv4Flag(f)
		}
		return nil
	}
}

// ipv4_fragment 设置 IPv4 头部的分片偏移(Fragment Offset)字段
// 在 yak 中通过 pcapx.ipv4_fragment 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 分片偏移值
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 IPv4 分片偏移
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"), pcapx.ipv4_fragment(0))~
// println(len(raw))
// ```
func WithIPv4_FragmentOffset(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		pv4.FragOffset = uint16(utils.InterfaceToInt(i))
		return nil
	}
}

// ipv4_ttl 设置 IPv4 头部的 TTL(生存时间)字段
// 在 yak 中通过 pcapx.ipv4_ttl 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: TTL 值(0-255)
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 IPv4 TTL
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"), pcapx.ipv4_ttl(64))~
// println(len(raw))
// ```
func WithIPv4_TTL(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		pv4.TTL = uint8(utils.InterfaceToInt(i))
		return nil
	}
}

// ipv4_srcIp 设置 IPv4 头部的源 IP 地址
// 在 yak 中通过 pcapx.ipv4_srcIp 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 源 IP 地址字符串，如 "1.1.1.1"
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 IPv4 源地址
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"))~
// println(len(raw))
// ```
func WithIPv4_SrcIP(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		pv4.SrcIP = net.ParseIP(utils.FixForParseIP(utils.InterfaceToString(i)))
		if pv4.SrcIP == nil {
			return utils.Errorf("WithIPv4_SrcIP error: %v", i)
		}
		return nil
	}
}

// ipv4_dstOp 设置 IPv4 头部的目的 IP 地址
// 在 yak 中通过 pcapx.ipv4_dstOp 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 目的 IP 地址字符串，如 "2.2.2.2"
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 IPv4 目的地址
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"))~
// println(len(raw))
// ```
func WithIPv4_DstIP(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		pv4.DstIP = net.ParseIP(utils.FixForParseIP(utils.InterfaceToString(i)))
		if pv4.DstIP == nil {
			return utils.Errorf("WithIPv4_DstIP error: %v", i)
		}
		return nil
	}
}

// ipv4_nextLayerProtocol 设置 IPv4 头部的上层协议(Protocol)字段
// 在 yak 中通过 pcapx.ipv4_nextLayerProtocol 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - i: 上层协议，如 "tcp"、"udp"、"icmp" 或对应数字
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定 IPv4 上层协议为 udp
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"), pcapx.ipv4_nextLayerProtocol("udp"))~
// println(len(raw))
// ```
func WithIPv4_NextProtocol(i any) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		strI := utils.InterfaceToString(i)
		switch strings.ToLower(strI) {
		case "ipv6_hop_by_hop":
			pv4.Protocol = layers.IPProtocolIPv6HopByHop
		case "icmp", "icmp4", "icmpv4", "icmp_v4":
			pv4.Protocol = layers.IPProtocolICMPv4
		case "igmp":
			pv4.Protocol = layers.IPProtocolIGMP
		case "ipv4", "ip4": // ?
			pv4.Protocol = layers.IPProtocolIPv4
		case "tcp":
			pv4.Protocol = layers.IPProtocolTCP
		case "udp":
			pv4.Protocol = layers.IPProtocolUDP
		case "rudp":
			pv4.Protocol = layers.IPProtocolRUDP
		case "ipv6", "ip6": // ?
			pv4.Protocol = layers.IPProtocolIPv6
		case "ipv6_routing":
			pv4.Protocol = layers.IPProtocolIPv6Routing
		case "ipv6_fragment":
			pv4.Protocol = layers.IPProtocolIPv6Fragment
		case "ipv6_icmp", "icmp6", "icmp_v6":
			pv4.Protocol = layers.IPProtocolICMPv6
		case "no_next_header":
			pv4.Protocol = layers.IPProtocolNoNextHeader
		case "ipv6_destination":
			pv4.Protocol = layers.IPProtocolIPv6Destination
		case "gre":
			pv4.Protocol = layers.IPProtocolGRE
		case "esp":
			pv4.Protocol = layers.IPProtocolESP
		case "ah":
			pv4.Protocol = layers.IPProtocolAH
		case "ospf":
			pv4.Protocol = layers.IPProtocolOSPF
		case "ipip":
			pv4.Protocol = layers.IPProtocolIPIP
		case "etherip":
			pv4.Protocol = layers.IPProtocolEtherIP
		case "vrrp":
			pv4.Protocol = layers.IPProtocolVRRP
		case "sctp":
			pv4.Protocol = layers.IPProtocolSCTP
		case "udplite":
			pv4.Protocol = layers.IPProtocolUDPLite
		case "mplsinip":
			pv4.Protocol = layers.IPProtocolMPLSInIP
		default:
			if utils.MatchAllOfRegexp(i, `\d+`) {
				pv4.Protocol = layers.IPProtocol(utils.InterfaceToInt(i))
				return nil
			}
			return utils.Errorf("unknown parse ip_protocol: %v", i)
		}
		return nil
	}
}

// ipv4_option 向 IPv4 头部追加一个选项(Option)，optType 为 nil 时清空全部选项
// 在 yak 中通过 pcapx.ipv4_option 调用，配合 pcapx.PacketBuilder 使用
// 参数:
//   - optType: 选项类型，传 nil 表示清空所有选项
//   - data: 选项数据字节
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的 IPv4 层配置选项
//
// Example:
// ```
// // 该示例为示意性用法：追加一个 IPv4 选项
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"), pcapx.ipv4_option(7, []byte{0x01}))~
// println(len(raw))
// ```
func WithIPv4_Option(optType any, data []byte) IPv4Option {
	return func(pv4 *layers.IPv4) error {
		if len(data)+2 > 255 {
			log.Warnf("ipv4 option data length is too long, max length is 255, got %d, data: %v", len(data), spew.Sdump(data))
			return nil
		}
		if optType == nil {
			pv4.Options = nil
		} else {
			pv4.Options = append(pv4.Options, layers.IPv4Option{
				OptionType:   uint8(utils.InterfaceToInt(optType)),
				OptionLength: uint8(len(data)) + 2,
				OptionData:   data,
			})
		}
		return nil
	}
}

func WithIPv4_NoOptions() IPv4Option {
	return func(pv4 *layers.IPv4) error {
		pv4.Options = nil
		return nil
	}
}
