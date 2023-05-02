package pcapx

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"math/rand"
	"net"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func injectWithError(raw []byte, c *Config) error {
	if c.Iface == "" && PublicInterface != nil {
		iface, _, _, err := GetPublicRoute()
		if err != nil {
			return utils.Errorf("get default public iface failed: %s", err)
		}
		c.Iface = iface.Name
	}

	if c.Iface == "" {
		return utils.Error("empty iface")
	}

	return injectRaw(c.Iface, raw)
}

func RegenerateTCPTraffic(raw []byte, localIPAddress string, opt ...ConfigOption) {
	c := &Config{}
	for _, o := range opt {
		o(c)
	}

	if !c.ToServerSet {
		c.ToServerSet = true
		c.ToServer = true
	}

	ip, tcp, payload, err := ParseTCPRaw(raw)
	if err != nil {
		log.Errorf("parse tcp/ip layer failed: %s", err)
		return
	}

	var bufFlow0 = gopacket.NewSerializeBuffer()
	var bufFlow1 = gopacket.NewSerializeBuffer()
	link, err := GetPublicToServerLinkLayerIPv4()

	if err != nil {
		log.Errorf("get link layer failed: %s", err)
		return
	}
	tcp.SetNetworkLayerForChecksum(ip)

	mySrcIp := net.ParseIP(localIPAddress)
	mySrcPort := layers.TCPPort(uint32(55000 + rand.Intn(65535-55000)))

	originalSrcIP := ip.SrcIP
	originalSrcPort := tcp.SrcPort

	// 出站流量
	tcp.SrcPort = mySrcPort
	ip.SrcIP = mySrcIp

	err = gopacket.SerializeLayers(bufFlow0, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, link, ip, tcp, payload)

	// 进站流量
	tcp.SrcPort = originalSrcPort
	ip.SrcIP = originalSrcIP
	tcp.DstPort = mySrcPort
	ip.DstIP = mySrcIp

	err = gopacket.SerializeLayers(bufFlow1, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, link, ip, tcp, payload)

	if err != nil {
		log.Errorf("serialize layers failed: %v", err)
	}

	rawData0 := bufFlow0.Bytes()
	rawData1 := bufFlow1.Bytes()

	if (rawData0 == nil || rawData1 == nil) || (len(rawData0) <= 0 || len(rawData1) <= 0) {
		log.Error("serialize packet failed")
		return
	}

	if c.ToServerSet {
		InjectRaw(rawData0, opt...)
	} else {
		InjectRaw(rawData1, opt...)
	}

	return
}

func InjectRaw(raw []byte, opt ...ConfigOption) {
	c := &Config{}
	for _, o := range opt {
		o(c)
	}
	err := injectWithError(raw, c)
	if err != nil {
		log.Warnf("inject packet to net.Iface failed: %s", err)
	}
}

func InjectTCPPayload(payload []byte, opt ...ConfigOption) {
	c := &Config{}
	for _, o := range opt {
		o(c)
	}
	if !c.ToServerSet {
		c.ToServerSet = true
		c.ToServer = true
	}
	if c.TCPLocalAddress == "" {
		_, _, ip, err := GetPublicRoute()
		if err != nil {
			log.Errorf("cannot fetch public route failed: %s", err)
		}
		c.TCPLocalAddress = ip.String() + ":" + fmt.Sprint(rand.Intn(65000-40000)+40000)
	}

	if c.TCPRemoteAddress == "" {
		c.TCPRemoteAddress = utils.GetRandomIPAddress() + ":" + fmt.Sprint(20+rand.Intn(1000))
	}

	syn, synack, ack, pushacks, finack, err := CreateTCPHandshakePackets(c.TCPLocalAddress, c.TCPRemoteAddress, payload)
	if err != nil {
		log.Errorf("create iptcp handshake failed: %s", err)
		return
	}
	toOpt := append(opt, WithToServer())
	fromOpt := append(opt, WithToClient())
	if !c.ToServer {
		for _, pushAck := range pushacks {
			InjectTCPIPInstance(pushAck, fromOpt...)
		}
		return
	}
	InjectTCPIPInstance(syn, toOpt...)
	InjectTCPIPInstance(synack, fromOpt...)
	InjectTCPIPInstance(ack, toOpt...)
	for _, pushack := range pushacks {
		InjectTCPIPInstance(pushack, toOpt...)
	}
	InjectTCPIPInstance(finack, toOpt...)
}

func InjectHTTPRequest(raw []byte, opt ...ConfigOption) {
	c := &Config{}
	for _, o := range opt {
		o(c)
	}

	urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(raw, c.IsHttps)
	if err != nil {
		log.Errorf("extract port failed: %v", err)
		return
	}
	host, port, _ := utils.ParseStringToHostPort(urlIns.String())
	InjectTCPPayload(lowhttp.FixHTTPRequestOut(raw), WithRemoteAddress(utils.HostPort(host, port)))
}

func InjectTCPIPInstance(raw *TCPIPFrame, opt ...ConfigOption) {
	if raw == nil {
		return
	}

	if raw.ToServer {
		opt = append(opt, WithToServer())
	} else {
		opt = append(opt, WithToClient())
	}

	var buf = gopacket.NewSerializeBuffer()
	if raw.TCP.Payload == nil {
		err := gopacket.SerializeLayers(buf, defaultGopacketSerializeOpt, raw.IP, raw.TCP)
		if err != nil {
			log.Error(err)
			return
		}
	} else {
		err := gopacket.SerializeLayers(buf, defaultGopacketSerializeOpt, raw.IP, raw.TCP, gopacket.Payload(raw.TCP.Payload))
		if err != nil {
			log.Error(err)
			return
		}
	}
	InjectTCPIP(buf.Bytes(), opt...)
}

func InjectTCPIP(raw []byte, opt ...ConfigOption) {
	ip, tcp, payload, err := ParseTCPIPv4(raw)
	if err != nil {
		log.Errorf("parse tcp/ip layer failed: %s", err)
		return
	}

	globalStatistics.AddTransportationLayerStatistics(utils.HostPort(ip.SrcIP.String(), fmt.Sprint(int(tcp.SrcPort))))
	globalStatistics.AddTransportationLayerStatistics(utils.HostPort(ip.DstIP.String(), fmt.Sprint(int(tcp.DstPort))))
	globalStatistics.AddNetworkLayerStatistics(ip.SrcIP.String())
	globalStatistics.AddNetworkLayerStatistics(ip.DstIP.String())

	c := &Config{}
	for _, o := range opt {
		o(c)
	}

	if !c.ToServerSet {
		log.Error("tcp/ip layer should specific to server or client")
		return
	}
	var buf = gopacket.NewSerializeBuffer()
	link, err := GetPublicToServerLinkLayerIPv4()
	if c.ToServer {
		if err != nil {
			log.Errorf("get link layer failed: %s", err)
			return
		}
		tcp.SetNetworkLayerForChecksum(ip)
		err = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		}, link, ip, tcp, payload)
		if err != nil {
			log.Errorf("serialize layers failed: %v", err)
		}
	} else {
		if err != nil {
			log.Errorf("get link layer failed: %s", err)
			return
		}
		tcp.SetNetworkLayerForChecksum(ip)
		err = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		}, link, ip, tcp, payload)
		if err != nil {
			log.Errorf("serialize layers failed: %v", err)
		}
	}
	rawData := buf.Bytes()
	if rawData == nil || len(rawData) <= 0 {
		log.Error("serialize packet failed")
		return
	}
	// 增加统计信息
	if link.DstMAC != nil {
		globalStatistics.AddLinkLayerStatistics(link.DstMAC.String())
	}
	if link.SrcMAC != nil {
		globalStatistics.AddLinkLayerStatistics(link.SrcMAC.String())
	}
	InjectRaw(rawData, opt...)
}

func InjectUDPIP(raw []byte, opt ...ConfigOption) {
	ip, udp, payload, err := ParseUDPIPv4(raw)
	if err != nil {
		log.Errorf("parse udp/ip layer failed: %s", err)
		return
	}

	globalStatistics.AddTransportationLayerStatistics(utils.HostPort(ip.SrcIP.String(), fmt.Sprint(int(udp.SrcPort))))
	globalStatistics.AddTransportationLayerStatistics(utils.HostPort(ip.DstIP.String(), fmt.Sprint(int(udp.DstPort))))
	globalStatistics.AddNetworkLayerStatistics(ip.SrcIP.String())
	globalStatistics.AddNetworkLayerStatistics(ip.DstIP.String())

	c := &Config{}
	for _, o := range opt {
		o(c)
	}

	if !c.ToServerSet {
		log.Error("udp/ip layer should specific to server or client")
		return
	}
	var buf = gopacket.NewSerializeBuffer()
	link, err := GetPublicToServerLinkLayerIPv4()

	if err != nil {
		log.Errorf("get link layer failed: %s", err)
		return
	}
	udp.SetNetworkLayerForChecksum(ip)
	err = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, link, ip, udp, payload)
	if err != nil {
		log.Errorf("serialize layers failed: %v", err)
	}

	rawData := buf.Bytes()
	if rawData == nil || len(rawData) <= 0 {
		log.Error("serialize packet failed")
		return
	}
	// 增加统计信息
	if link.DstMAC != nil {
		globalStatistics.AddLinkLayerStatistics(link.DstMAC.String())
	}
	if link.SrcMAC != nil {
		globalStatistics.AddLinkLayerStatistics(link.SrcMAC.String())
	}
	InjectRaw(rawData, opt...)
}

func InjectICMPIP(raw []byte, opt ...ConfigOption) {
	ip, icmp, payload, err := ParseICMPIPv4(raw)
	if err != nil {
		log.Errorf("parse icmp/ip layer failed: %s", err)
		return
	}

	globalStatistics.AddNetworkLayerStatistics(ip.SrcIP.String())
	globalStatistics.AddNetworkLayerStatistics(ip.DstIP.String())

	c := &Config{}
	for _, o := range opt {
		o(c)
	}

	var buf = gopacket.NewSerializeBuffer()
	link, err := GetPublicToServerLinkLayerIPv4()

	if err != nil {
		log.Errorf("get link layer failed: %s", err)
		return
	}

	err = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, link, ip, icmp, payload)
	if err != nil {
		log.Errorf("serialize layers failed: %v", err)
	}

	rawData := buf.Bytes()
	if rawData == nil || len(rawData) <= 0 {
		log.Error("serialize packet failed")
		return
	}
	// 增加统计信息
	if link.DstMAC != nil {
		globalStatistics.AddLinkLayerStatistics(link.DstMAC.String())
	}
	if link.SrcMAC != nil {
		globalStatistics.AddLinkLayerStatistics(link.SrcMAC.String())
	}
	InjectRaw(rawData, opt...)
}

func InjectChaosTraffic(t *chaosmaker.ChaosTraffic, opts ...ConfigOption) {
	if t.HttpRequest != nil {
		InjectHTTPRequest(t.HttpRequest, opts...)
	}

	if t.RawTCP && t.TCPIPPayload != nil {
		RegenerateTCPTraffic(t.TCPIPPayload, t.LocalIP, append(opts, WithToClient())...)
		RegenerateTCPTraffic(t.TCPIPPayload, t.LocalIP, append(opts, WithToServer())...)
		return
	}

	if t.HttpResponse != nil {
		_, _, _, _ = GetPublicRoute()
		InjectTCPPayload(t.HttpResponse, append(opts, WithToClient(), WithLocalAddress(PublicPreferredAddress.String()+":80"))...)
	}

	if t.TCPIPPayload != nil {
		InjectTCPIP(t.TCPIPPayload, append(opts, WithToClient())...)
		InjectTCPIP(t.TCPIPPayload, append(opts, WithToServer())...)
	}

	if t.UDPIPOutboundPayload != nil {
		InjectUDPIP(t.UDPIPOutboundPayload, append(opts, WithToServer())...)
	}

	if t.UDPIPInboundPayload != nil {
		InjectUDPIP(t.UDPIPInboundPayload, append(opts, WithToClient())...)
	}

	if t.ICMPIPOutboundPayload != nil {
		InjectICMPIP(t.ICMPIPOutboundPayload, append(opts, WithToServer())...)
	}

	if t.ICMPIPInboundPayload != nil {
		InjectICMPIP(t.ICMPIPInboundPayload, append(opts, WithToClient())...)
	}

	if t.LinkLayerPayload != nil {
		InjectRaw(t.LinkLayerPayload, opts...)
	}
}

func getStatistics() *Statistics {
	return globalStatistics
}

var (
	Exports = map[string]interface{}{
		"GetStatistics":      getStatistics,
		"InjectRaw":          InjectRaw,
		"InjectIP":           InjectTCPIP,
		"InjectTCP":          InjectTCPIP,
		"InjectHTTPRequest":  InjectHTTPRequest,
		"InjectChaosTraffic": InjectChaosTraffic,
	}
)
