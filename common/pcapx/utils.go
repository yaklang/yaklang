package pcapx

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

func AutoSerializeLayers(layers ...gopacket.SerializableLayer) ([]byte, error) {
	buffer := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, layers...)
	if err != nil {
		return nil, errors.Wrap(err, "serialize gopacket error")
	}
	return buffer.Bytes(), nil
}

func CopyEthernetLayer(t *layers.Ethernet) *layers.Ethernet {
	return &layers.Ethernet{
		BaseLayer: layers.BaseLayer{
			Contents: utils.CopyBytes(t.Contents),
			Payload:  utils.CopyBytes(t.Payload),
		},
		SrcMAC:       utils.CopyBytes(t.SrcMAC),
		DstMAC:       utils.CopyBytes(t.DstMAC),
		EthernetType: t.EthernetType,
	}
}

func CopyIPv4Layer(t *layers.IPv4) *layers.IPv4 {
	return &layers.IPv4{
		BaseLayer: layers.BaseLayer{
			Contents: utils.CopyBytes(t.Contents),
			Payload:  utils.CopyBytes(t.Payload),
		},
		Version:    t.Version,
		IHL:        t.IHL,
		TOS:        t.TOS,
		Length:     t.Length,
		Id:         t.Id,
		Flags:      t.Flags,
		FragOffset: t.FragOffset,
		TTL:        t.TTL,
		Protocol:   t.Protocol,
		Checksum:   t.Checksum,
		SrcIP:      t.SrcIP,
		DstIP:      t.DstIP,
		Options: lo.Map(t.Options, func(k layers.IPv4Option, v int) layers.IPv4Option {
			return layers.IPv4Option{
				OptionType:   k.OptionType,
				OptionLength: k.OptionLength,
				OptionData:   utils.CopyBytes(k.OptionData),
			}
		}),
		Padding: utils.CopyBytes(t.Padding),
	}
}

func CopyIPv6Layer(t *layers.IPv6) *layers.IPv6 {
	v6 := &layers.IPv6{
		BaseLayer: layers.BaseLayer{
			Contents: utils.CopyBytes(t.Contents),
			Payload:  utils.CopyBytes(t.Payload),
		},
		Version:      t.Version,
		TrafficClass: t.TrafficClass,
		FlowLabel:    t.FlowLabel,
		Length:       t.Length,
		NextHeader:   t.NextHeader,
		HopLimit:     t.HopLimit,
		SrcIP:        t.SrcIP,
		DstIP:        t.DstIP,
	}

	if t.HopByHop != nil {
		v6.HopByHop = &layers.IPv6HopByHop{
			Options: lo.Map(t.HopByHop.Options, func(k *layers.IPv6HopByHopOption, v int) *layers.IPv6HopByHopOption {
				opt := &layers.IPv6HopByHopOption{
					OptionType:   k.OptionType,
					OptionLength: k.OptionLength,
					ActualLength: k.ActualLength,
					OptionData:   utils.CopyBytes(k.OptionData),
					OptionAlignment: [2]uint8{
						k.OptionAlignment[0],
						k.OptionAlignment[1],
					},
				}
				return opt
			}),
		}
	}
	return v6
}

func CopyTCP(t *layers.TCP) *layers.TCP {
	return &layers.TCP{
		BaseLayer: layers.BaseLayer{
			Contents: utils.CopyBytes(t.Contents),
			Payload:  utils.CopyBytes(t.Payload),
		},
		SrcPort:    t.SrcPort,
		DstPort:    t.DstPort,
		Seq:        t.Seq,
		Ack:        t.Ack,
		DataOffset: t.DataOffset,
		FIN:        t.FIN,
		SYN:        t.SYN,
		RST:        t.RST,
		PSH:        t.PSH,
		ACK:        t.ACK,
		URG:        t.URG,
		ECE:        t.ECE,
		CWR:        t.CWR,
		NS:         t.NS,
		Window:     t.Window,
		Checksum:   t.Checksum,
		Urgent:     t.Urgent,
		Options: lo.Map(t.Options, func(k layers.TCPOption, v int) layers.TCPOption {
			return layers.TCPOption{
				OptionType:   k.OptionType,
				OptionLength: k.OptionLength,
				OptionData:   utils.CopyBytes(k.OptionData),
			}
		}),
		Padding: utils.CopyBytes(t.Padding),
	}
}
