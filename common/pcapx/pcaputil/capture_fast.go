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
// using gopacket's complete decoder (VLAN, tunnels, IPv6 extensions, loopback).
type offlineDecoder struct {
	conf      *CaptureConfig
	link      layers.LinkType
	ethernet  layers.Ethernet
	ipv4      layers.IPv4
	ipv6      layers.IPv6
	tcp       layers.TCP
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
	var network gopacket.SerializableLayer
	if d.link == layers.LinkTypeEthernet {
		if err := d.ethernet.DecodeFromBytes(raw, d); err != nil {
			d.conf.trafficPool.malformedPacket(err.Error())
			return
		}
		switch d.ethernet.EthernetType {
		case layers.EthernetTypeIPv4:
			if err := d.ipv4.DecodeFromBytes(d.ethernet.Payload, d); err != nil {
				d.conf.trafficPool.malformedPacket(err.Error())
				return
			}
			if d.ipv4.Version != 4 || d.truncated {
				d.conf.trafficPool.malformedPacket("invalid or truncated IPv4 packet")
				return
			}
			if d.ipv4.Protocol == layers.IPProtocolTCP && d.ipv4.FragOffset == 0 && d.ipv4.Flags&layers.IPv4MoreFragments == 0 {
				network = &d.ipv4
			}
		case layers.EthernetTypeIPv6:
			if err := d.ipv6.DecodeFromBytes(d.ethernet.Payload, d); err != nil {
				d.conf.trafficPool.malformedPacket(err.Error())
				return
			}
			if d.ipv6.Version != 6 || d.truncated {
				d.conf.trafficPool.malformedPacket("invalid or truncated IPv6 packet")
				return
			}
			if d.ipv6.NextHeader == layers.IPProtocolTCP {
				network = &d.ipv6
			}
		}
	}
	if network != nil {
		if err := d.tcp.DecodeFromBytes(network.(gopacket.NetworkLayer).LayerPayload(), d); err != nil {
			d.conf.trafficPool.malformedPacket(err.Error())
			return
		}
		feeding = true
		d.conf.trafficPool.Feed(&d.ethernet, network, &d.tcp, ci.Timestamp)
		return true
	}
	packet := gopacket.NewPacket(raw, d.link, gopacket.DecodeOptions{Lazy: true, NoCopy: true, DecodeStreamsAsDatagrams: true})
	packet.Metadata().CaptureInfo = ci
	feeding = true
	d.conf.packetHandler(ctx, packet)
	_, decoded = packet.TransportLayer().(*layers.TCP)
	return decoded
}

func openOfflineFast(conf *CaptureConfig, ctx context.Context, handler *PcapHandleWrapper) error {
	d := offlineDecoder{conf: conf, link: handler.LinkType()}
	read := handler.handle.ZeroCopyReadPacketData
	// Classic pcap needs no native protocol processing. Amortize file reads in
	// Go and avoid a cgo transition per packet. Keep libpcap for pcapng, unusual
	// headers and callers that can configure the exposed native handle.
	if conf.onNetInterfaceCreated == nil {
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
		if conf.trafficPool.parallel != nil && conf.trafficPool.owner == nil && !conf.DisableAssembly {
			conf.trafficPool.parallel.checkTruncation(ci)
			if key, ok, err := rawFlowKey(raw, d.link); err != nil {
				conf.trafficPool.malformedPacket(err.Error())
			} else if ok {
				conf.trafficPool.parallel.submit(workerPacket{data: raw, ts: ci.Timestamp, link: d.link, raw: true, key: key})
			}
		} else {
			d.feed(ctx, raw, ci)
		}
	}
	return nil
}
