package pcapx

import (
	"fmt"
	"github.com/asaskevich/govalidator"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

var tcpExports = map[string]interface{}{
	"tcp_srcPort":    TCPBuilderConfigWithSrcPort,
	"tcp_dstPort":    TCPBuilderConfigWithDstPort,
	"tcp_seq":        TCPBuilderConfigWithSeq,
	"tcp_ack":        TCPBuilderConfigWithAck,
	"tcp_dataOffset": TCPBuilderConfigWithDataOffset,
	"tcp_FIN":        TCPBuilderConfigWithFIN,
	"tcp_SYN":        TCPBuilderConfigWithSYN,
	"tcp_RST":        TCPBuilderConfigWithRST,
	"tcp_PSH":        TCPBuilderConfigWithPSH,
	"tcp_ACK":        TCPBuilderConfigWithACK,
	"tcp_URG":        TCPBuilderConfigWithURG,
	"tcp_ECE":        TCPBuilderConfigWithECE,
	"tcp_CWR":        TCPBuilderConfigWithCWR,
	"tcp_NS":         TCPBuilderConfigWithNS,
	"tcp_window":     TCPBuilderConfigWithWindow,
	"tcp_urgent":     TCPBuilderConfigWithUrgent,
	"tcp_options":    TCPBuilderConfigWithOptions,
	"tcp_payload":    TCPBuilderConfigWithPadding,
}

type TCPBuilderConfig struct {
	SrcPort    int
	DstPort    int
	Seq        int
	Ack        int
	DataOffset int
	FIN        bool
	SYN        bool
	RST        bool
	PSH        bool
	ACK        bool
	URG        bool
	ECE        bool
	CWR        bool
	NS         bool
	Window     int
	Checksum   int
	Urgent     int
	Options    []layers.TCPOption
	Padding    []byte
}

type TCPBuilderConfigOption func(config *TCPBuilderConfig)

func TCPBuilderConfigWithSrcPort(srcPort int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.SrcPort = srcPort
	}
}

func TCPBuilderConfigWithDstPort(dstPort int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.DstPort = dstPort
	}
}

func TCPBuilderConfigWithSeq(seq int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.Seq = seq
	}
}

func TCPBuilderConfigWithAck(ack int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.Ack = ack
	}
}

func TCPBuilderConfigWithDataOffset(dataOffset int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.DataOffset = dataOffset
	}
}

func TCPBuilderConfigWithFIN(fin bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.FIN = fin
	}
}

func TCPBuilderConfigWithSYN(syn bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.SYN = syn
	}
}

func TCPBuilderConfigWithRST(rst bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.RST = rst
	}
}

func TCPBuilderConfigWithPSH(psh bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.PSH = psh
	}
}

func TCPBuilderConfigWithACK(ack bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.ACK = ack
	}
}

func TCPBuilderConfigWithURG(urg bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.URG = urg
	}
}

func TCPBuilderConfigWithECE(ece bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.ECE = ece
	}
}

func TCPBuilderConfigWithCWR(cwr bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.CWR = cwr
	}
}

func TCPBuilderConfigWithNS(ns bool) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.NS = ns
	}
}

func TCPBuilderConfigWithWindow(window int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.Window = window
	}
}

func TCPBuilderConfigWithChecksum(checksum int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.Checksum = checksum
	}
}

func TCPBuilderConfigWithUrgent(urgent int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.Urgent = urgent
	}
}

func TCPBuilderConfigWithOptions(options ...layers.TCPOption) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.Options = append(config.Options, options...)
	}
}

func TCPBuilderConfigWithPadding(padding []byte) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.Padding = padding
	}
}

func TCPBuilderConfigWithMSS(mss any) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		if !govalidator.IsInt(fmt.Sprint(mss)) {
			log.Warnf("MSS is not int, got %v", mss)
			return
		}
		// 65535 -> 0xffff
		config.Options = append(config.Options, layers.TCPOption{
			OptionType: layers.TCPOptionKindMSS,
			OptionData: utils.NetworkByteOrderUint16ToBytes(mss),
		})
	}
}

func TCPBuilderConfigWithWindowScale(i int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		if i > 14 {
			log.Warnf("WindowScale is too large, got %v", i)
			return
		}
		config.Options = append(config.Options, layers.TCPOption{
			OptionType: layers.TCPOptionKindWindowScale,
			OptionData: []byte{byte(i)},
		})
	}
}

func TCPBuilderConfigWithSACKPermitted(i int) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		config.Options = append(config.Options, layers.TCPOption{
			OptionType: layers.TCPOptionKindSACKPermitted,
		})
	}
}

func TCPBuilderConfigWithTimestamps(now, before any) TCPBuilderConfigOption {
	return func(config *TCPBuilderConfig) {
		origin := utils.NetworkByteOrderUint32ToBytes(now)
		replay := utils.NetworkByteOrderUint32ToBytes(before)
		buf := make([]byte, 8)
		copy(buf[:4], origin)
		copy(buf[4:], replay)
		config.Options = append(config.Options, layers.TCPOption{
			OptionType: layers.TCPOptionKindTimestamps,
			OptionData: buf,
		})
	}
}

func TCPBuilderConfigWithTimestampNow(before any) TCPBuilderConfigOption {
	return TCPBuilderConfigWithTimestamps(time.Now().Unix(), before)
}

func (t *TCPBuilderConfig) Create() *layers.TCP {
	packet := &layers.TCP{
		SrcPort:    layers.TCPPort(t.SrcPort),
		DstPort:    layers.TCPPort(t.DstPort),
		Seq:        uint32(t.Seq),
		Ack:        uint32(t.Ack),
		DataOffset: uint8(t.DataOffset),
		FIN:        t.FIN,
		SYN:        t.SYN,
		RST:        t.RST,
		PSH:        t.PSH,
		ACK:        t.ACK,
		URG:        t.URG,
		ECE:        t.ECE,
		CWR:        t.CWR,
		NS:         t.NS,
		Window:     uint16(t.Window),
		Checksum:   uint16(t.Checksum),
		Urgent:     uint16(t.Urgent),
		Options:    t.Options,
		Padding:    t.Padding,
	}
	return packet
}
