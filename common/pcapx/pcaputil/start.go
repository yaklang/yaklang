package pcaputil

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type Config struct {
	Device    []string
	BPFFilter string
	Context   context.Context
}

func (c *Config) packetHandler(ctx context.Context, packet gopacket.Packet) {
	fmt.Println(packet.String())
}

func NewDefaultConfig() *Config {
	return &Config{}
}

type Option func(*Config) error

func _open(ctx context.Context, dev string, bpf string, packetEntry func(context.Context, gopacket.Packet)) error {
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	handler, err := pcap.OpenLive(dev, 65535, true, pcap.BlockForever)
	if err != nil {
		return utils.Errorf("pcap.OpenLive in pcaputils error: %s", err)
	}

	if bpf != "" {
		if err := handler.SetBPFFilter(bpf); err != nil {
			return utils.Errorf("set bpf filter failed: %s", err)
		}
	}

	packetSource := gopacket.NewPacketSource(handler, handler.LinkType())
	for {
		select {
		case <-ctx.Done():
			return nil
		case packet := <-packetSource.Packets():
			if packetEntry != nil {
				packetEntry(innerCtx, packet)
			} else {
				fmt.Println(packet.String())
			}
		}
	}
}

func Start(opt ...Option) error {
	conf := NewDefaultConfig()
	for _, i := range opt {
		if err := i(conf); err != nil {
			return utils.Errorf("set option failed: %s", err)
		}
	}

	var devs []string
	if len(conf.Device) == 0 {
		var ifs, err = pcap.FindAllDevs()
		if err != nil {
			return utils.Errorf("(pcap) find all devs failed: %s", err)
		}
		for _, iface := range ifs {
			devs = append(devs, iface.Name)
		}
	} else {
		for _, i := range conf.Device {
			pcapIface, err := IfaceNameToPcapIfaceName(i)
			if err != nil {
				log.Warnf("convert iface name (%v) failed: %s, use default", i, err)
				pcapIface = i
			}
			devs = append(devs, pcapIface)
		}
	}

	// TODO: check devs length, 128 is enough...
	if len(devs) > 128 {
		return utils.Errorf("too many devices: %d", len(devs))
	} else if len(devs) == 0 {
		return utils.Errorf("no pcap devices found")
	}

	if conf.Context == nil {
		conf.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(conf.Context)
	defer cancel()

	var wg = new(sync.WaitGroup)
	for _, i := range devs {
		wg.Add(1)
		go func(dev string) {
			defer wg.Done()
			if err := _open(ctx, dev, "", func(ctx context.Context, packet gopacket.Packet) {
				defer func() {
					if err := recover(); err != nil {
						spew.Dump(err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				conf.packetHandler(ctx, packet)
			}); err != nil {
				log.Errorf("open device (%v) failed: %s", dev, err)
			}
		}(i)
	}
	wg.Wait()

	return nil
}
