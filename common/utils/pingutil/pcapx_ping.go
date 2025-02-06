package pingutil

import (
	"context"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/pcapx/arpx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
	"sync"
	"time"
)

var icmpPayload = []byte("f\xc8\x14A\x00\n\xebs\b\t\n\v\f\r\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f !\"#$%&'()*+,-./01234567")

func PingAuto2(target string, config *PingConfig) *PingResult {
	result, err := PcapxPing(target, config)
	if result != nil {
		return result
	}
	_ = err
	// tcp
	wg := new(sync.WaitGroup)
	isAlive := utils.NewBool(false)
	ports := utils.ParseStringToPorts(config.defaultTcpPort)
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()
	for _, p := range ports {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := netx.DialContext(ctx, utils.HostPort(target, p), config.proxies...)
			if err != nil {
				return
			}
			isAlive.Set()
			cancel()
			_ = conn.Close()
		}()
	}
	wg.Wait()
	if isAlive.IsSet() {
		return &PingResult{
			IP:  target,
			Ok:  true,
			RTT: 0,
		}
	}
	return &PingResult{
		IP:     target,
		Ok:     false,
		RTT:    0,
		Reason: "tcp timeout",
	}
}

func PcapxPing(target string, config *PingConfig) (*PingResult, error) {
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}
	parentCtx := config.Ctx
	ip := target
	if !utils.IsIPv4(target) && !utils.IsIPv6(target) {
		host, _, _ := utils.ParseStringToHostPort(target)
		if host == "" {
			host = target
		}
		ip = netx.LookupFirst(host, netx.WithTimeout(config.timeout), netx.WithDNSServers(config.proxies...))
		if ip == "" {
			return nil, utils.Errorf("lookup %s failed", host)
		}
	}

	v4 := net.ParseIP(ip).To4() != nil
	isLoopBack := utils.IsLoopback(ip)
	var iface *net.Interface
	var err error
	var firstIP string
	if isLoopBack {
		iface, err = pcaputil.GetLoopBackNetInterface()
		if err != nil {
			return nil, err
		}
		firstIP = ip
	} else {
		var srcIp net.IP
		iface, _, srcIp, err = netutil.GetPublicRoute()
		if err != nil {
			return nil, err
		}
		if srcIp == nil {
			return nil, utils.Errorf("no address found")
		}
		if v4 {
			firstIP = srcIp.To4().String()
		} else {
			firstIP = srcIp.To16().String()
		}
	}

	ctx, cancel := context.WithTimeout(parentCtx, config.timeout)
	defer cancel()
	macAddress, _ := arpx.ArpWithContext(ctx, iface.Name, ip)

	isAlive := utils.NewBool(false)
	ttl := 0

	go func() {
		defer func() {
			if err := recover(); err != nil {
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		err := pcaputil.Start(
			pcaputil.WithDevice(iface.Name),
			pcaputil.WithBPFFilter("icmp"),
			pcaputil.WithEnableCache(true),
			pcaputil.WithContext(ctx),
			pcaputil.WithNetInterfaceCreated(func(handle *pcaputil.PcapHandleWrapper) {
				go func() {
					defer func() {
						if err := recover(); err != nil {
							utils.PrintCurrentGoroutineRuntimeStack()
						}
					}()
					retryCount := config.RetryCount
					if retryCount == 0 {
						retryCount = 2
					}
					retryInterval := config.RetryInterval
					if retryInterval == 0 {
						retryInterval = 60 * time.Millisecond
					}

					// time.Ticker 控制发包间隔
					ticker := time.NewTicker(retryInterval)
					defer ticker.Stop()

					for i := 0; i < retryCount; i++ {
						if isAlive.IsSet() {
							return
						}

						// 4. 发包错误处理优化
						if err := sendICMPPacket(handle, ip, firstIP, isLoopBack, macAddress, iface, i); err != nil {
							log.Debugf("send icmp packet failed: %s", err)
							continue
						}

						<-ticker.C
					}
				}()
			}),
			pcaputil.WithEveryPacket(handleICMPPacket(ip, isAlive, &ttl, cancel)),
		)
		if err != nil {
			log.Warnf("sniff failed: %s", err)
		}
	}()

	resultChan := make(chan *PingResult, 1)
	go func() {
		<-ctx.Done()
		if isAlive.IsSet() {
			resultChan <- &PingResult{IP: target, Ok: true, RTT: int64(ttl)}
		} else {
			resultChan <- &PingResult{IP: target, Ok: false, Reason: "timeout"}
		}
	}()

	return <-resultChan, nil
}

// 发包
func sendICMPPacket(handle *pcaputil.PcapHandleWrapper, ip, firstIP string, isLoopBack bool, macAddress net.HardwareAddr, iface *net.Interface, seq int) error {
	packetOpts := []any{
		pcapx.WithLoopback(isLoopBack),
		pcapx.WithIPv4_DstIP(ip),
		pcapx.WithIPv4_SrcIP(firstIP),
		pcapx.WithIPv4_NoOptions(),
		pcapx.WithICMP_Type(layers.ICMPv4TypeEchoRequest, nil),
		pcapx.WithICMP_Id(1),
		pcapx.WithICMP_Sequence(seq),
		pcapx.WithPayload(icmpPayload),
	}

	if macAddress != nil {
		packetOpts = append(packetOpts,
			pcapx.WithEthernet_DstMac(macAddress),
			pcapx.WithEthernet_SrcMac(iface.HardwareAddr))
	}

	packet, err := pcapx.PacketBuilder(packetOpts...)
	if err != nil {
		return fmt.Errorf("build packet: %w", err)
	}

	return handle.WritePacketData(packet)
}

// 包处理
func handleICMPPacket(targetIP string, isAlive *utils.AtomicBool, ttl *int, cancel context.CancelFunc) func(packet gopacket.Packet) {
	return func(packet gopacket.Packet) {
		if isAlive.IsSet() || !pcaputil.IsICMP(packet) {
			return
		}

		ip4Layer := packet.Layer(layers.LayerTypeIPv4)
		icmpLayer := packet.Layer(layers.LayerTypeICMPv4)

		if ip4, ok := ip4Layer.(*layers.IPv4); ok {
			if l, ok := icmpLayer.(*layers.ICMPv4); ok &&
				l.TypeCode.Type() == layers.ICMPv4TypeEchoReply &&
				ip4.SrcIP.String() == targetIP {
				isAlive.SetTo(true)
				*ttl = int(ip4.TTL)
				cancel()
				log.Debugf("%v is alive", ip4.SrcIP.String())
			}
		}
	}
}
