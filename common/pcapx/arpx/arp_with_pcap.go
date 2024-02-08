package arpx

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"net"
	"time"
)

func ArpWithPcap(ctx context.Context, ifaceName string, ip string) (map[string]net.HardwareAddr, error) {
	ifaceIns, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx, _ = context.WithTimeout(ctx, 5*time.Second)
	}

	if ifaceIns.Flags&net.FlagLoopback != 0 {
		return nil, errors.New("loopback")
	}

	ctx, cancel := context.WithCancel(ctx)
	_ = cancel

	results := make(map[string]net.HardwareAddr)
	err = pcaputil.Start(
		pcaputil.WithDevice(ifaceName),
		pcaputil.WithEnableCache(true),
		pcaputil.WithBPFFilter("arp"),
		pcaputil.WithContext(ctx),
		pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
			arpLayer := packet.Layer(layers.LayerTypeARP)
			if arpLayer == nil {
				return
			}
			arpIns, ok := arpLayer.(*layers.ARP)
			if !ok {
				return
			}

			if arpIns.Operation != layers.ARPReply || bytes.Equal(ifaceIns.HardwareAddr, arpIns.SourceHwAddress) {
				return
			}

			ipAddr := net.IP(arpIns.SourceProtAddress).String()
			hwAddr := net.HardwareAddr(arpIns.SourceHwAddress)
			results[ipAddr] = hwAddr
		}),
	)
	if len(results) > 0 {
		return results, nil
	}
	return nil, err
}
