package cybertunnel

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/pcap"
)

type remoteICMPIPDesc struct {
	IP net.IP

	ConnectionDesc  *utils.Cache[uint16]
	connectionCache map[string]uint16

	cacheMutex *sync.Mutex
}

type triggeredSizeDesc struct {
	RemoteCache       *utils.Cache[int64]
	CurrentRemoteAddr string
	LastTimestamp     int64

	addrCache  map[string]int64
	cacheMutex *sync.Mutex
}

type ICMPTrigger struct {
	size         *sync.Map
	remoteICMPIP *utils.Cache[*remoteICMPIPDesc]
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewICMPTrigger() (*ICMPTrigger, error) {
	trigger := &ICMPTrigger{
		size:         new(sync.Map),
		remoteICMPIP: utils.NewTTLCache[*remoteICMPIPDesc](),
		ctx:          context.Background(),
	}
	trigger.ctx, trigger.cancel = context.WithCancel(trigger.ctx)
	trigger.remoteICMPIP.SetTTL(1 * time.Minute)

	return trigger, nil
}

func (p *ICMPTrigger) Run() error {
	return p.run(p.ctx)
}

func (p *ICMPTrigger) run(ctx context.Context) error {
	ifm, ip, interfaceIP, err := netutil.Route(5*time.Second, "8.8.8.8")
	if err != nil {
		return utils.Errorf("fetch public net interface failed: %s", err)
	}
	_ = ip

	log.Infof("use iface: %v(%v) - %v", ifm.Name, ip, interfaceIP)
	ifaceName, err := pcaputil.IfaceNameToPcapIfaceName(ifm.Name)
	if err != nil {
		return utils.Errorf("convert iface name failed: %s", err)
	}
	handler, err := pcap.OpenLive(ifaceName, 65535, false, pcap.BlockForever)
	if err != nil {
		return utils.Errorf("open [%v] failed: %s", ifaceName, err)
	}

	go func() {
		<-ctx.Done()
		handler.Close()
	}()

	err = handler.SetBPFFilter("icmp[icmptype] == icmp-echo")
	if err != nil {
		return utils.Errorf("compile bpf failed: %s", err)
	}

	source := gopacket.NewPacketSource(handler, handler.LinkType())
	for {
		packet, ok := <-source.Packets()
		if !ok {
			return nil
		}
		p.handlePacket(interfaceIP, packet)
	}
}

func (p *ICMPTrigger) handlePacket(interfaceIP net.IP, packet gopacket.Packet) {
	icmpLayer, ok := packet.NetworkLayer().(*layers.IPv4)
	if !ok {
		return
	}
	if interfaceIP.Equal(icmpLayer.DstIP) {
		remoteAddr := icmpLayer.SrcIP
		icmpLength := icmpLayer.Length
		log.Infof("fetch ICMP from %v => %v (SIZE: %v)",
			remoteAddr.String(),
			icmpLayer.DstIP.String(),
			icmpLength,
		)
		desc, ok := p.remoteICMPIP.Get(remoteAddr.String())
		// 该远程IP没有记录
		if !ok {
			newDesc := &remoteICMPIPDesc{
				IP:              remoteAddr,
				ConnectionDesc:  utils.NewTTLCache[uint16](time.Minute),
				cacheMutex:      new(sync.Mutex),
				connectionCache: make(map[string]uint16),
			}
			// ttl到期删除
			newDesc.ConnectionDesc.SetExpirationCallback(func(key string, value uint16) {
				newDesc.cacheMutex.Lock()
				defer newDesc.cacheMutex.Unlock()
				delete(newDesc.connectionCache, key)
			})
			// 添加
			newDesc.ConnectionDesc.SetNewItemCallback(func(key string, value uint16) {
				newDesc.cacheMutex.Lock()
				defer newDesc.cacheMutex.Unlock()
				newDesc.connectionCache[key] = icmpLength
			})
			p.remoteICMPIP.Set(remoteAddr.String(), newDesc)
			desc = newDesc
		}
		desc.ConnectionDesc.Set(remoteAddr.String(), icmpLength)

		var sizeDesc *triggeredSizeDesc
		sizeDescRaw, ok := p.size.Load(icmpLength)
		if !ok {
			sDesc := &triggeredSizeDesc{
				RemoteCache: utils.NewTTLCache[int64](time.Minute),
				cacheMutex:  new(sync.Mutex),
				addrCache:   make(map[string]int64),
			}
			sDesc.RemoteCache.SetNewItemCallback(func(key string, value int64) {
				sDesc.CurrentRemoteAddr = key
				sDesc.LastTimestamp = time.Now().Unix()
				sDesc.cacheMutex.Lock()
				defer sDesc.cacheMutex.Unlock()
				sDesc.addrCache[sDesc.CurrentRemoteAddr] = sDesc.LastTimestamp
			})
			sDesc.RemoteCache.SetExpirationCallback(func(key string, value int64) {
				sDesc.cacheMutex.Lock()
				defer sDesc.cacheMutex.Unlock()
				delete(sDesc.addrCache, sDesc.CurrentRemoteAddr)
				if len(sDesc.addrCache) <= 0 {
					p.size.Delete(icmpLength)
				}
			})
			p.size.Store(icmpLength, sDesc)
			sizeDesc = sDesc
		} else {
			sizeDesc = sizeDescRaw.(*triggeredSizeDesc)
		}
		sizeDesc.RemoteCache.Set(remoteAddr.String(), time.Now().Unix())
	}
}

func (p *ICMPTrigger) getTriggeredSizeDesc(i int) (*triggeredSizeDesc, bool) {
	raw, ok := p.size.Load(uint16(i))
	if ok {
		return raw.(*triggeredSizeDesc), true
	} else {
		return nil, false
	}
}

func (p *ICMPTrigger) getRemoteAddrDesc(i int) (*remoteICMPIPDesc, bool) {
	sDesc, ok := p.getTriggeredSizeDesc(i)
	if !ok {
		return nil, false
	}

	remoteIP := sDesc.CurrentRemoteAddr

	rDesc, ok := p.remoteICMPIP.Get(remoteIP)
	if !ok {
		return nil, false
	}

	return rDesc, true
}

type ICMPTriggerNotification struct {
	Size                               int
	CurrentRemoteAddr                  string
	Histories                          []string
	CurrentRemoteCachedConnectionCount int
	SizeCachedHistoryConnectionCount   int
	TriggerTimestamp                   int64
	Timestamp                          int64
}

func (t *ICMPTriggerNotification) Show() {
	fmt.Printf("Size:[%v] FROM: [%v] REMOTE_CONNS_COUNT:[%v] "+
		"HISTORY:[%v] FROM_NOW:[%v]\n",
		t.Size,
		t.CurrentRemoteAddr,
		t.CurrentRemoteCachedConnectionCount,
		t.SizeCachedHistoryConnectionCount,
		(time.Duration(t.Timestamp-t.TriggerTimestamp) * time.Second).String(),
	)
}

func (p *ICMPTrigger) GetICMPTriggerNotification(i int) (*ICMPTriggerNotification, error) {
	i = i + 28
	notif := &ICMPTriggerNotification{
		Size:      i,
		Timestamp: time.Now().Unix(),
	}
	sDesc, _ := p.getTriggeredSizeDesc(i)
	if sDesc == nil {
		return nil, utils.Error("empty size connections")
	}
	if sDesc != nil {
		notif.SizeCachedHistoryConnectionCount = len(sDesc.addrCache)
		notif.CurrentRemoteAddr = sDesc.CurrentRemoteAddr
		notif.TriggerTimestamp = sDesc.LastTimestamp
	}

	rDesc, _ := p.getRemoteAddrDesc(i)
	if rDesc != nil {
		notif.CurrentRemoteCachedConnectionCount = len(rDesc.connectionCache)
	}

	return notif, nil
}
