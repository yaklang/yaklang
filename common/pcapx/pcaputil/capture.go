package pcaputil

import (
	"context"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func _open(conf *CaptureConfig, ctx context.Context, handler *pcap.Handle, packetEntry func(context.Context, gopacket.Packet)) error {
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	packetSource := gopacket.NewPacketSource(handler, handler.LinkType())

	if conf.onNetInterfaceCreated != nil {
		conf.onNetInterfaceCreated(handler)
	}

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

	var handlers = omap.NewOrderedMap(map[string]PcapHandleOperation{})
	if conf.Filename != "" {
		handler, err := OpenFile(conf.Filename)
		if err != nil {
			log.Errorf("open file (%v) failed: %s", conf.Filename, err)
		} else {
			handlers.Set(conf.Filename, handler)
		}
	} else if len(conf.Device) == 0 {
		if conf.EmptyDeviceStop {
			return utils.Errorf("no device found")
		}

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
			cacheId, handler, err := getInterfaceHandlerFromConfig(iface.Name, conf)
			if err != nil {
				log.Errorf("open device (%v) failed: %s", iface.Name, err)
				continue
			}
			handlers.Set(cacheId, handler.(PcapHandleOperation))
		}
	} else {
		for _, i := range conf.Device {
			pcapIface, err := IfaceNameToPcapIfaceName(i)
			if err != nil {
				log.Warnf("convert iface name (%v) failed: %s, use default", i, err)
				pcapIface = i
			}
			cacheId, handler, err := getInterfaceHandlerFromConfig(pcapIface, conf)
			if err != nil {
				log.Errorf("open device (%v) failed: %s", pcapIface, err)
				continue
			}
			if cacheId == "" {
				cacheId = uuid.New().String()
			}
			handlers.Set(cacheId, handler)
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

	if conf.EnableCache {
		// keep cache
		// add cancel func to defer
		// hack: use runtimeId to registerCallback
		var cancels []func()
		handlers.ForEach(func(i string, _ PcapHandleOperation) bool {
			if conf.EnableCache {
				cancels = append(cancels, keepDaemonCache(i, ctx))
			}
			return true
		})
		defer func() {
			for _, c := range cancels {
				c()
			}
		}()

		runtimeId := uuid.New().String()
		for _, i := range handlers.Keys() {
			registerCallback(i, runtimeId, ctx, func(ctx context.Context, packet gopacket.Packet) error {
				conf.packetHandler(ctx, packet)
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					return nil
				}
			})
		}
		select {
		case <-ctx.Done():
		}
	} else {
		utils.WaitRoutinesFromSlice(handlers.Values(), func(origin PcapHandleOperation) {
			handler, ok := origin.(*pcap.Handle)
			if !ok {
				log.Errorf("invalid handler: %v", origin)
				return
			}
			defer func() {
				handler.Close()
			}()
			if err := _open(conf, ctx, handler, conf.packetHandler); err != nil {
				log.Errorf("open device failed: %s", err)
			}
		})
	}

	return nil
}
