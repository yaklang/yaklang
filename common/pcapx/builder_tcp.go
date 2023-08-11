package pcapx

import (
	"bytes"
	"github.com/asaskevich/govalidator"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

func WithTCP_Flags(in any) TCPOption {
	return func(config *layers.TCP) error {
		var flagStr []string
		if ret := utils.InterfaceToString(in); govalidator.IsInt(ret) {
			var i = utils.Atoi(ret)
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

func WithTCP_SrcPort(srcPort any) TCPOption {
	return func(config *layers.TCP) error {
		config.SrcPort = layers.TCPPort(utils.InterfaceToInt(srcPort))
		return nil
	}
}

func WithTCP_DstPort(dstPort any) TCPOption {
	return func(config *layers.TCP) error {
		config.DstPort = layers.TCPPort(utils.InterfaceToInt(dstPort))
		return nil
	}
}

func WithTCP_Seq(seq any) TCPOption {
	return func(config *layers.TCP) error {
		config.Seq = uint32(utils.InterfaceToInt(seq))
		return nil
	}
}

func WithTCP_Ack(ack any) TCPOption {
	return func(config *layers.TCP) error {
		config.Ack = uint32(utils.InterfaceToInt(ack))
		return nil
	}
}

func WithTCP_DataOffset(dataOffset any) TCPOption {
	return func(config *layers.TCP) error {
		config.DataOffset = uint8(utils.InterfaceToInt(dataOffset))
		return nil
	}
}

func WithTCP_Window(window any) TCPOption {
	return func(config *layers.TCP) error {
		config.Window = uint16(utils.InterfaceToInt(window))
		return nil
	}
}

func WithTCP_Urgent(urgent any) TCPOption {
	return func(config *layers.TCP) error {
		config.Urgent = uint16(utils.InterfaceToInt(urgent))
		return nil
	}
}

func WithTCP_Options(optionType any, data []byte) TCPOption {
	return func(config *layers.TCP) error {
		if len(data)+2 > 255 {
			log.Warnf("tcp option data length is too long, max length is 255, got %d, data: %v", len(data), spew.Sdump(data))
			return nil
		}
		config.Options = append(config.Options, layers.TCPOption{
			OptionType:   layers.TCPOptionKind(utils.InterfaceToInt(optionType)),
			OptionLength: uint8(len(data)) + 2,
			OptionData:   data,
		})
		return nil
	}
}

// WithTCP_OptionMSS is a IPv4Option default 1460
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

func WithTCP_OptionSACKPermitted() TCPOption {
	return func(pv4 *layers.TCP) error {
		for _, p := range pv4.Options {
			if p.OptionType == layers.TCPOptionKindSACKPermitted {
				return nil
			}
		}
		pv4.Options = append(pv4.Options, layers.TCPOption{
			OptionType:   layers.TCPOptionKindSACKPermitted,
			OptionLength: 2,
		})
		return nil
	}
}

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
