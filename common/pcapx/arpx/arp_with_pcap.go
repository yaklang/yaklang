package arpx

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"net"
	"sync/atomic"
	"time"
)

func ArpWithPcapFirst(ctx context.Context, ifaceName string, target string) (net.HardwareAddr, error) {
	result, err := ArpWithPcap(ctx, ifaceName, target)
	if err != nil {
		return nil, err
	}
	for _, hw := range result {
		return hw, nil
	}
	return nil, errors.New("no result")
}

func ArpWithPcap(ctx context.Context, ifaceName string, targets string) (map[string]net.HardwareAddr, error) {
	ifaceIns, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx, _ = context.WithTimeout(ctx, 5*time.Second)
	} else {
		_, haveDDL := ctx.Deadline()
		if !haveDDL {
			ctx, _ = context.WithTimeout(ctx, 5*time.Second)
		}
	}

	if ifaceIns.Flags&net.FlagLoopback != 0 {
		return nil, errors.New("loopback")
	}

	ctx, cancel := context.WithCancel(ctx)
	_ = cancel

	targetList := hostsparser.NewHostsParser(ctx, targets)
	results := make(map[string]net.HardwareAddr)
	maxSize := targetList.Size()
	var resultSize int64 = 0
	err = pcaputil.Start(
		pcaputil.WithDevice(ifaceName),
		pcaputil.WithEnableCache(true),
		pcaputil.WithBPFFilter("arp"),
		pcaputil.WithContext(ctx),
		pcaputil.WithNetInterfaceCreated(func(handle *pcap.Handle) {
			for p := range targetList.Hosts() {
				buf, err := newArpARPPacket(ifaceIns, p)
				if err != nil {
					log.Errorf("new arp packet failed: %s", err)
					continue
				}
				err = handle.WritePacketData(buf.Bytes())
				time.Sleep(5 * time.Millisecond) // 20ms delay for write
				if err != nil {
					log.Errorf("write packet failed: %s", err)
					continue
				}
			}
			return
		}),
		pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
			select {
			case <-ctx.Done():
				return
			default:
			}

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
			if !targetList.Contains(ipAddr) {
				return
			}
			hwAddr := net.HardwareAddr(arpIns.SourceHwAddress)
			results[ipAddr] = hwAddr
			if atomic.AddInt64(&resultSize, 1) >= int64(maxSize) {
				cancel()
			}
		}),
	)
	if len(results) > 0 {
		return results, nil
	}
	return nil, err
}
