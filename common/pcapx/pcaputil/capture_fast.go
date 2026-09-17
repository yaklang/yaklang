package pcaputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// offlineDecoder reuses layer structs only when no caller can observe a packet.
// TrafficPool copies every byte that outlives Feed. Unusual encapsulations keep
// using gopacket's complete decoder (VLAN, tunnels and IPv6 extensions).
type offlineDecoder struct {
	conf      *CaptureConfig
	link      layers.LinkType
	ethernet  layers.Ethernet
	loopback  layers.Loopback
	ipv4      layers.IPv4
	ipv6      layers.IPv6
	tcp       layers.TCP
	udp       layers.UDP
	truncated bool
}

func (d *offlineDecoder) SetTruncated() { d.truncated = true }

func (d *offlineDecoder) feed(ctx context.Context, raw []byte, ci gopacket.CaptureInfo) (decoded bool) {
	feeding := false
	d.truncated = false
	// Match packetHandler's callback isolation on the regular packet path.
	defer func() {
		if err := recover(); err != nil {
			if !feeding {
				d.conf.trafficPool.malformedPacket(fmt.Sprintf("malformed packet decoder panic: %v", err))
			} else {
				decoded = true // accounted separately as a callback panic
				d.conf.trafficPool.notePanic(err)
				log.Errorf("TCP assembly callback panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}
	}()
	if d.conf.DisableAssembly {
		return true
	}
	fallback := func() bool {
		packet := gopacket.NewPacket(raw, d.link, gopacket.DecodeOptions{Lazy: true, NoCopy: true, DecodeStreamsAsDatagrams: true})
		packet.Metadata().CaptureInfo = ci
		feeding = true
		d.conf.packetHandler(ctx, packet)
		_, tcp := packet.TransportLayer().(*layers.TCP)
		return tcp
	}
	var network gopacket.SerializableLayer
	var ethernet *layers.Ethernet
	ipBytes := raw
	var next gopacket.LayerType
	switch d.link {
	case layers.LinkTypeEthernet:
		if err := d.ethernet.DecodeFromBytes(raw, d); err != nil {
			d.conf.trafficPool.malformedPacket(err.Error())
			return
		}
		ethernet, ipBytes, next = &d.ethernet, d.ethernet.Payload, d.ethernet.NextLayerType()
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		if err := d.loopback.DecodeFromBytes(raw, d); err != nil {
			d.conf.trafficPool.malformedPacket(err.Error())
			return
		}
		ipBytes, next = d.loopback.Payload, d.loopback.NextLayerType()
	case layers.LinkTypeRaw:
		if len(raw) > 0 {
			if raw[0]>>4 == 4 {
				next = layers.LayerTypeIPv4
			} else if raw[0]>>4 == 6 {
				next = layers.LayerTypeIPv6
			}
		}
	}
	var transport layers.IPProtocol
	switch next {
	case layers.LayerTypeIPv4:
		if err := d.ipv4.DecodeFromBytes(ipBytes, d); err != nil {
			return fallback()
		}
		if d.ipv4.Version != 4 || d.truncated {
			return fallback()
		}
		if d.ipv4.FragOffset == 0 && d.ipv4.Flags&layers.IPv4MoreFragments == 0 {
			network = &d.ipv4
			transport = d.ipv4.Protocol
		}
	case layers.LayerTypeIPv6:
		if err := d.ipv6.DecodeFromBytes(ipBytes, d); err != nil {
			return fallback()
		}
		if d.ipv6.Version != 6 || d.truncated {
			return fallback()
		}
		network, transport = &d.ipv6, d.ipv6.NextHeader
	}
	if network != nil && transport == layers.IPProtocolTCP {
		if err := d.tcp.DecodeFromBytes(network.(gopacket.NetworkLayer).LayerPayload(), d); err != nil {
			d.conf.trafficPool.malformedPacket(err.Error())
			return
		}
		feeding = true
		d.conf.trafficPool.Feed(ethernet, network, &d.tcp, ci.Timestamp)
		return true
	}
	if network != nil && transport == layers.IPProtocolUDP && d.conf.binParser != nil {
		if err := d.udp.DecodeFromBytes(network.(gopacket.NetworkLayer).LayerPayload(), d); err == nil {
			feeding = true
			d.conf.binParser.datagramFields(network.(gopacket.NetworkLayer), &d.udp, ci, d.truncated)
			return false
		}
		// A partial UDP layer can still produce an incomplete event through
		// the original packet path. Preserve its behavior for decoder errors.
	}
	return fallback()
}

func openOfflineFast(conf *CaptureConfig, ctx context.Context, handler *PcapHandleWrapper) error {
	d := offlineDecoder{conf: conf, link: handler.LinkType()}
	read := handler.handle.ZeroCopyReadPacketData
	// Classic pcap needs no native protocol processing. Amortize file reads in
	// Go and avoid a cgo transition per packet. Keep libpcap for pcapng, unusual
	// headers and callers that can configure the exposed native handle.
	if conf.onNetInterfaceCreated == nil && conf.BPFFilter == "" {
		if file, err := os.Open(conf.Filename); err == nil {
			defer file.Close()
			if reader, err := newClassicPcapReader(file); err == nil {
				// Older native backends expose only microsecond precision. Keep
				// their timestamp semantics when reading a nanosecond file.
				if reader.scale != 1 || handler.handle.Resolution() == gopacket.TimestampResolutionNanosecond {
					d.link = reader.link
					read = reader.read
				}
			}
		}
	}
	for ctx.Err() == nil {
		// Consume the borrowed Go/native buffer before the next read invalidates it.
		raw, ci, err := read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		conf.trafficPool.observeCapture(len(raw))
		if conf.recorder != nil {
			if err := conf.recorder.write(raw, ci, d.link); err != nil {
				return err
			}
		}
		if conf.trafficPool.parallel != nil && conf.trafficPool.owner == nil && !conf.DisableAssembly {
			conf.trafficPool.parallel.checkTruncation(ci)
			if key, ok, err := rawFlowKey(raw, d.link); err != nil {
				conf.trafficPool.malformedPacket(err.Error())
			} else if ok {
				conf.trafficPool.parallel.submit(workerPacket{data: raw, ts: ci.Timestamp, link: d.link, raw: true, key: key})
			} else if conf.binParser != nil {
				d.feed(ctx, raw, ci)
			}
		} else {
			d.feed(ctx, raw, ci)
		}
	}
	return nil
}
