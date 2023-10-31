// Package recv
// @Author bcy2007  2023/9/18 10:54
package recv

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/bas/core"
	basUtils "github.com/yaklang/yaklang/common/bas/utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const (
	info      = "result"
	heartbeat = "heartbeat"
)

type Receiver struct {
	sync.Mutex
	iface             string
	source            string
	localAddress      string
	send              func()
	ctx               context.Context
	cancel            func()
	message           []string
	httpMessageSender *basUtils.HttpMessageSend
	heartBeat         func()
}

func CreateReceiver(iface, source, localAddress string) *Receiver {
	receiver := &Receiver{
		iface:        iface,
		source:       source,
		localAddress: localAddress,
	}
	receiver.init()
	return receiver
}

func (receiver *Receiver) init() {
	if receiver.localAddress == "" {
		receiver.localAddress = core.GetIfaceByName(receiver.iface)
	}
	if receiver.localAddress == "" {
		log.Error("no local address found")
		return
	}
	log.Infof("%v: %v", receiver.iface, receiver.localAddress)

	receiver.send = receiver.createSender()
	ctx, cancel := context.WithCancel(context.Background())
	receiver.ctx = ctx
	receiver.cancel = cancel
	receiver.message = make([]string, 0)
	if receiver.source != "" {
		receiver.httpMessageSender = basUtils.NewMessageSender(receiver.source)
		receiver.heartBeat = func() {
			//err := receiver.httpMessageSender.SendMessages(PacketMessage{IPAddress: receiver.localAddress}, heartbeat)
			err := receiver.httpMessageSender.PretendSendMessages(PacketMessage{IPAddress: receiver.localAddress}, heartbeat)
			if err != nil {
				log.Errorf("http send message error: %v", err)
			}
		}
	} else {
		receiver.heartBeat = func() {
			fmt.Printf("%v working...\n", receiver.localAddress)
		}
	}
}

func (receiver *Receiver) Cancel() {
	receiver.cancel()
}

func (receiver *Receiver) createSender() func() {
	if receiver.source == "" {
		return func() {
			receiver.Lock()
			defer receiver.Unlock()
			if len(receiver.message) == 0 {
				return
			}
			for _, msg := range receiver.message {
				fmt.Printf("%v\n", msg)
			}
			receiver.message = receiver.message[:0]
		}
	} else {
		return func() {
			receiver.Lock()
			defer receiver.Unlock()
			if len(receiver.message) > 0 {
				msg := PacketMessage{IPAddress: receiver.localAddress, MD5: receiver.message}
				//err := receiver.httpMessageSender.SendMessages(msg, info)
				err := receiver.httpMessageSender.PretendSendMessages(msg, info)
				if err != nil {
					log.Errorf("http send message error: %v", err)
				}
			}
			receiver.message = receiver.message[:0]
		}
	}
}

func (receiver *Receiver) InsertMessage(message string) {
	receiver.Lock()
	receiver.message = append(receiver.message, message)
	receiver.Unlock()
}

func (receiver *Receiver) SendMessage() {
	for {
		receiver.send()
		time.Sleep(10 * time.Second)
	}
}

func (receiver *Receiver) ReceivePacket() error {
	snapLen := int32(65535)
	promise := false
	timeout := pcap.BlockForever

	handle, err := pcap.OpenLive(receiver.iface, snapLen, promise, timeout)
	if err != nil {
		return utils.Errorf("failed to open device: %v", err)
	}

	defer handle.Close()

	// heartbeat
	go func() {
		for {
			receiver.heartBeat()
			time.Sleep(10 * time.Second)
		}
	}()

	var eth layers.Ethernet
	var ipv4 layers.IPv4
	var ipv6 layers.IPv6
	var tcp layers.TCP
	var udp layers.UDP
	var tls layers.TLS
	var payload gopacket.Payload
	var dns layers.DNS
	var dhcpv6 layers.DHCPv6

	layersTypes := []gopacket.DecodingLayer{
		&eth,
		&ipv4,
		&ipv6,
		&tcp,
		&udp,
		&tls,
		&payload,
		&dns,
		&dhcpv6,
	}

	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, layersTypes...)
	decoded := make([]gopacket.LayerType, 0)

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	log.Infof("listening from %v", core.TestIP)
	for packet := range packetSource.Packets() {
		if err := parser.DecodeLayers(packet.Data(), &decoded); err != nil {
			continue
		}
		var src, dst string
		for _, layerType := range decoded {
			switch layerType {
			case layers.LayerTypeIPv4:
				src = ipv4.SrcIP.String()
				dst = ipv4.DstIP.String()
			case layers.LayerTypeIPv6:
				src = ipv6.SrcIP.String()
				dst = ipv6.DstIP.String()
			default:
			}
		}
		if dst != receiver.localAddress || src != core.TestIP {
			continue
		}
		traffic := packet.Data()
		result, err := core.PacketDataAnalysis(traffic)
		if err != nil {
			log.Errorf("analysis packet data error: %v", err)
			continue
		}
		if len(result) != 0 {
			log.Infof("%v -> %v", src, dst)
			receiver.InsertMessage(codec.Md5(result))
		}
	}
	return nil
}
