package pcaputil

import (
	"context"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func _open(ctx context.Context, handler *pcap.Handle, bpf string, packetEntry func(context.Context, gopacket.Packet)) error {
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

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
			if packet == nil {
				return nil
			}
			if packetEntry != nil {
				packetEntry(innerCtx, packet)
			} else {
				fmt.Println(packet.String())
			}
		}
	}
}

func Start(opt ...CaptureOption) error {
	conf := NewDefaultConfig()
	for _, i := range opt {
		if err := i(conf); err != nil {
			return utils.Errorf("set option failed: %s", err)
		}
	}

	var handlers []*pcap.Handle
	if conf.Filename != "" {
		handler, err := OpenFile(conf.Filename)
		if err != nil {
			log.Errorf("open file (%v) failed: %s", conf.Filename, err)
		} else {
			handlers = append(handlers, handler)
		}
	} else if len(conf.Device) == 0 {
		var ifs, err = pcap.FindAllDevs()
		if err != nil {
			return utils.Errorf("(pcap) find all devs failed: %s", err)
		}

		if len(ifs) > 128 {
			return utils.Errorf("too many devices: %d", len(ifs))
		}

		if len(ifs) == 0 {
			return utils.Errorf("no pcap devices found")
		}

		for _, iface := range ifs {
			handler, err := OpenIfaceLive(iface.Name)
			if err != nil {
				log.Errorf("open device (%v) failed: %s", iface.Name, err)
				continue
			}
			handlers = append(handlers, handler)
		}
	} else {
		for _, i := range conf.Device {
			pcapIface, err := IfaceNameToPcapIfaceName(i)
			if err != nil {
				log.Warnf("convert iface name (%v) failed: %s, use default", i, err)
				pcapIface = i
			}
			handler, err := OpenIfaceLive(pcapIface)
			if err != nil {
				log.Errorf("open device (%v) failed: %s", pcapIface, err)
				continue
			}
			handlers = append(handlers, handler)
		}
	}

	if conf.Context == nil {
		conf.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(conf.Context)
	defer func() {
		log.Info("pcapx.utils.capture context done")
		cancel()
	}()

	conf.trafficPool = NewTrafficPool(ctx)
	for _, p := range conf.onPoolCreated {
		p(conf.trafficPool)
	}
	utils.WaitRoutinesFromSlice(handlers, func(handler *pcap.Handle) {
		if err := _open(ctx, handler, "", conf.packetHandler); err != nil {
			log.Errorf("open device failed: %s", err)
		}
	})

	return nil
}
