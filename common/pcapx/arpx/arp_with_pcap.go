package arpx

import (
	"bytes"
	"context"
	"errors"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/utils/omap"
	"net"
	"sync"
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

	var timeout time.Duration
	if ctx == nil {
		ctx = context.Background()
	} else {
		ddl, haveDDL := ctx.Deadline()
		if haveDDL {
			timeout = time.Until(ddl)
		}
	}
	if ifaceIns.Flags&net.FlagLoopback != 0 {
		return nil, errors.New("arp on loopback interface is not supported")
	}

	ctx, cancel := context.WithCancel(ctx)
	_ = cancel

	targetList := hostsparser.NewHostsParser(ctx, targets)
	results := omap.NewOrderedMap(map[string]net.HardwareAddr{}) // make(map[string]net.HardwareAddr)
	maxSize := targetList.Size()
	var resultSize int64 = 0

	senderSwg := utils.NewSizedWaitGroup(20)
	senderMutex := new(sync.Mutex)
	_ = timeout

	err = pcaputil.Start(
		pcaputil.WithDevice(ifaceName),
		pcaputil.WithEnableCache(true),
		pcaputil.WithBPFFilter("arp"),
		pcaputil.WithContext(ctx),
		pcaputil.WithNetInterfaceCreated(func(handle *pcaputil.PcapHandleWrapper) {
			go func() {
				defer func() {
					if err := recover(); err != nil {
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				for p := range targetList.Hosts() {
					p := p
					senderSwg.Add(1)
					go func() {
						defer func() {
							senderSwg.Done()
						}()

						buf, err := newArpARPPacket(ifaceIns, p)
						if err != nil {
							log.Errorf("new arp packet failed: %s", err)
							return
						}
						count := 2
						for i := 0; i < count; i++ {
							select {
							case <-ctx.Done():
								return
							default:
							}

							if results.Have(p) {
								return
							}

							senderMutex.Lock()
							err = handle.WritePacketData(buf)
							time.Sleep(5 * time.Millisecond) // some ms delay for write
							senderMutex.Unlock()
							if err != nil {
								log.Errorf("new arp packet failed: %s", err)
								return
							}

							if i != count-1 {
								time.Sleep(100 * time.Millisecond)
							}
						}

					}()
				}
				senderSwg.Wait()
				time.Sleep(timeout)
				cancel()
			}()
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
			log.Debugf("IP[%v] 's mac addr: %v", ipAddr, arpIns.SourceHwAddress)
			hwAddr := net.HardwareAddr(arpIns.SourceHwAddress)
			results.Set(ipAddr, hwAddr)
			if atomic.AddInt64(&resultSize, 1) >= int64(maxSize) {
				cancel()
			}
		}),
	)
	if results.Len() > 0 {
		var ret = make(map[string]net.HardwareAddr)
		results.ForEach(func(i string, v net.HardwareAddr) bool {
			ret[i] = v
			return true
		})
		return ret, nil
	}
	return nil, err
}
