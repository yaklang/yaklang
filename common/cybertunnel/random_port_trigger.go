package cybertunnel

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/netutil"

	"github.com/ReneKroon/ttlcache"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type remoteIPDesc struct {
	IP net.IP

	// map[string]string
	// key: remoteAddr value: localPort
	ConnectionDesc  *ttlcache.Cache
	connectionCache map[string]int

	cacheMutex *sync.Mutex
}

type addrConnEvent struct {
	Addr      string
	Timestamp int64
}

type localPortDesc struct {
	// map[string]interface{}
	// key: remoteAddr value: timestamp
	RemoteCache       *ttlcache.Cache
	CurrentRemoteAddr string
	LastTimestamp     int64

	// 缓存一下结果
	addrCache  map[string]int64 // key: addr value: timestamp
	cacheMutex *sync.Mutex
}

type RandomPortTrigger struct {
	// map[port]
	localPort *sync.Map
	remoteIP  *ttlcache.Cache
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewRandomPortTrigger() (*RandomPortTrigger, error) {
	trigger := &RandomPortTrigger{
		localPort: new(sync.Map),
		remoteIP:  ttlcache.NewCache(),
		ctx:       context.Background(),
	}
	trigger.ctx, trigger.cancel = context.WithCancel(trigger.ctx)
	trigger.remoteIP.SetTTL(1 * time.Minute)

	return trigger, nil
}

func (p *RandomPortTrigger) Run() error {
	return p.run(p.ctx)
}

func (p *RandomPortTrigger) handlePacket(interfaceIP net.IP, packet gopacket.Packet) {
	//defer func() {
	//	if err := recover(); err != nil {
	//		log.Error(err)
	//	}
	//}()

	if packet.TransportLayer() == nil {
		return
	}

	tcpLayer := packet.TransportLayer()
	l, ok := tcpLayer.(*layers.TCP)
	if !ok {
		return
	}

	if l.SYN && !l.ACK {
		ipv4, ok := packet.NetworkLayer().(*layers.IPv4)
		if !ok {
			return
		}

		if interfaceIP.Equal(ipv4.DstIP) {
			log.Infof("fetch SYN from %v => %v (ORIGIN: %v)",
				utils.HostPort(ipv4.SrcIP.String(), l.SrcPort),
				utils.HostPort(ipv4.DstIP.String(), l.DstPort),
				interfaceIP.String(),
			)

			remoteAddr := utils.HostPort(ipv4.SrcIP.String(), int(l.SrcPort))
			localPortInt := int(l.DstPort)

			// 记录远程 IP 对应的端口
			var desc *remoteIPDesc
			descRaw, ok := p.remoteIP.Get(ipv4.SrcIP.String())
			if !ok {
				newDesc := &remoteIPDesc{
					IP:              ipv4.SrcIP,
					ConnectionDesc:  ttlcache.NewCache(),
					cacheMutex:      new(sync.Mutex),
					connectionCache: make(map[string]int),
				}
				newDesc.ConnectionDesc.SetTTL(time.Minute)
				newDesc.ConnectionDesc.SetExpirationCallback(func(key string, value interface{}) {
					newDesc.cacheMutex.Lock()
					defer newDesc.cacheMutex.Unlock()
					delete(newDesc.connectionCache, key)
				})
				newDesc.ConnectionDesc.SetNewItemCallback(func(key string, value interface{}) {
					newDesc.cacheMutex.Lock()
					defer newDesc.cacheMutex.Unlock()
					newDesc.connectionCache[key] = localPortInt
				})
				p.remoteIP.Set(ipv4.SrcIP.String(), newDesc)
				desc = newDesc
			} else {
				desc = descRaw.(*remoteIPDesc)
			}
			desc.ConnectionDesc.Set(remoteAddr, localPortInt)

			// 记录本地端口
			var localDesc *localPortDesc
			localDescRaw, ok := p.localPort.Load(int(l.DstPort))
			if !ok {
				lDesc := &localPortDesc{
					RemoteCache: ttlcache.NewCache(),
					cacheMutex:  new(sync.Mutex),
					addrCache:   make(map[string]int64),
				}
				lDesc.RemoteCache.SetTTL(time.Minute)
				lDesc.RemoteCache.SetNewItemCallback(func(key string, value interface{}) {
					lDesc.CurrentRemoteAddr = key
					lDesc.LastTimestamp = time.Now().Unix()
					lDesc.cacheMutex.Lock()
					defer lDesc.cacheMutex.Unlock()
					lDesc.addrCache[lDesc.CurrentRemoteAddr] = lDesc.LastTimestamp
				})
				lDesc.RemoteCache.SetExpirationCallback(func(key string, value interface{}) {
					lDesc.cacheMutex.Lock()
					defer lDesc.cacheMutex.Unlock()
					delete(lDesc.addrCache, lDesc.CurrentRemoteAddr)
					if len(lDesc.addrCache) <= 0 {
						p.localPort.Delete(int(l.DstPort))
					}
				})
				p.localPort.Store(int(l.DstPort), lDesc)
				localDesc = lDesc
			} else {
				localDesc = localDescRaw.(*localPortDesc)
			}
			localDesc.RemoteCache.Set(remoteAddr, time.Now().Unix())
		}
	}
}

func (p *RandomPortTrigger) run(ctx context.Context) error {
	ifm, ip, interfaceIP, err := netutil.Route(5*time.Second, "8.8.8.8")
	if err != nil {
		return utils.Errorf("fetch public net interface failed: %s", err)
	}
	_ = ip
	_ = interfaceIP

	ifaceName, err := utils.IfaceNameToPcapIfaceName(ifm.Name)
	if err != nil {
		return utils.Errorf("convert iface name failed: %s", err)
	}

	handler, err := pcap.OpenLive(ifaceName, 65535, false, pcap.BlockForever)
	if err != nil {
		return utils.Errorf("open [%v] failed: %s", ifaceName, err)
	}

	go func() {
		select {
		case <-ctx.Done():
		}
		handler.Close()
	}()

	err = handler.SetBPFFilter("(tcp[tcpflags] & (tcp-syn)) != 0")
	if err != nil {
		return utils.Errorf("compile bpf failed: %s", err)
	}

	source := gopacket.NewPacketSource(handler, handler.LinkType())
	for {
		select {
		case packet, ok := <-source.Packets():
			if !ok {
				return nil
			}
			p.handlePacket(interfaceIP, packet)
		}
	}
}

func (p *RandomPortTrigger) getLocalPortDesc(i int) (*localPortDesc, bool) {
	raw, ok := p.localPort.Load(i)
	if ok {
		return raw.(*localPortDesc), true
	} else {
		return nil, false
	}
}

func (p *RandomPortTrigger) getRemoteAddrDesc(i int) (*remoteIPDesc, bool) {
	lDesc, ok := p.getLocalPortDesc(i)
	if !ok {
		return nil, false
	}

	remoteIP, _, err := utils.ParseStringToHostPort(lDesc.CurrentRemoteAddr)
	if err != nil {
		return nil, false
	}

	rDesc, ok := p.remoteIP.Get(remoteIP)
	if !ok {
		return nil, false
	}

	return rDesc.(*remoteIPDesc), true
}

func (p *RandomPortTrigger) GetTriggerNotification(port int) (*TriggerNotification, error) {
	var notif = &TriggerNotification{
		LocalPort: port,
		Timestamp: time.Now().Unix(),
	}
	lDesc, _ := p.getLocalPortDesc(port)
	if lDesc == nil {
		return nil, utils.Error("empty local-port connections")
	}
	if lDesc != nil {
		if lDesc.addrCache != nil {
			for addr := range lDesc.addrCache {
				notif.Histories = append(notif.Histories, addr)
			}
		}
		notif.LocalPortCachedHistoryConnectionCount = len(lDesc.addrCache)
		notif.CurrentRemoteAddr = lDesc.CurrentRemoteAddr
		notif.TriggerTimestamp = lDesc.LastTimestamp
	}

	rDesc, _ := p.getRemoteAddrDesc(port)
	if rDesc != nil {
		notif.CurrentRemoteCachedConnectionCount = len(rDesc.connectionCache)
	}

	return notif, nil
}

type TriggerNotification struct {
	LocalPort                             int
	CurrentRemoteAddr                     string
	Histories                             []string
	CurrentRemoteCachedConnectionCount    int
	LocalPortCachedHistoryConnectionCount int
	TriggerTimestamp                      int64
	Timestamp                             int64
}

func (t *TriggerNotification) Show() {
	fmt.Printf("LOCAL_PORT:[%5d] FROM: [%21s] REMOTE_CONNS_COUNT:[%v] "+
		"HISTORY:[%v] FROM_NOW:[%v]\n",
		t.LocalPort, t.CurrentRemoteAddr,
		t.CurrentRemoteCachedConnectionCount,
		t.LocalPortCachedHistoryConnectionCount,
		(time.Duration(t.Timestamp-t.TriggerTimestamp) * time.Second).String(),
	)
}
