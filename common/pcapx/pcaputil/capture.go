package pcaputil

import (
	"context"
	"github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func _open(conf *CaptureConfig, ctx context.Context, handlerWrapper *PcapHandleWrapper) error {
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	packetSource := gopacket.NewPacketSource(handlerWrapper.handle, handlerWrapper.handle.LinkType())
	packetSource.Lazy = true
	packetSource.NoCopy = true
	packetSource.DecodeStreamsAsDatagrams = true
	if conf.onNetInterfaceCreated != nil {
		conf.onNetInterfaceCreated(handlerWrapper)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		//case packet := <-packetSource.PacketsCtx(innerCtx):
		case packet := <-packetSource.Packets():
			if packet == nil {
				return nil
			}
			conf.packetHandler(innerCtx, packet)
			//fmt.Println(packet.String())
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
	handlers := omap.NewOrderedMap(map[string]PcapHandleOperation{})
	if conf.Filename != "" {
		pcapHandler, err := OpenFile(conf.Filename)
		if err != nil {
			log.Errorf("open file (%v) failed: %s", conf.Filename, err)
		} else {
			handlers.Set(conf.Filename, WrapPcapHandle(pcapHandler))
		}
	} else if len(conf.DeviceAdapter) > 0 {
		for _, adapter := range conf.DeviceAdapter {
			pcapIface, err := IfaceNameToPcapIfaceName(adapter.DeviceName)
			if err != nil {
				log.Warnf("convert iface name (%v) failed: %s, use default", adapter.DeviceName, err)
				pcapIface = adapter.DeviceName
			}
			conf.deviceAdapter = adapter
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
	} else if len(conf.Device) > 0 {
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
	} else {
		if conf.EmptyDeviceStop {
			return utils.Errorf("no device found")
		}

		ifs, err := pcap.FindAllDevs()
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
		log.Debug("pcapx.utils.capture context done")
		cancel()
		conf.trafficPool.flowCache.ForEach(func(key string, flow *TrafficFlow) {
			flow.ForceShutdownConnection()
			if flow.requestQueue.Len() > 0 || flow.responseQueue.Len() > 0 {
				if conf.trafficPool._onHTTPFlow == nil {
					log.Warnf("unbalanced flow request/response flow: req[%v] rsp[%v]", flow.requestQueue.Len(), flow.responseQueue.Len())
				} else {
					for flow.CanShiftHTTPFlow() {
						req, rsp := flow.ShiftFlow()
						conf.trafficPool._onHTTPFlow(flow, req, rsp)
					}
				}
			}
		})
	}()

	conf.trafficPool = NewTrafficPool(ctx)
	conf.trafficPool.captureConf = conf
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
				cancels = append(cancels, keepDaemonCache(ctx, i))
			}
			return true
		})
		defer func() {
			for _, c := range cancels {
				c()
			}
		}()

		runtimeId := uuid.New().String()
		for _, key := range handlers.Keys() {
			registerCallback(ctx, key, runtimeId, func(ctx context.Context, packet gopacket.Packet) error {
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
			handler, ok := origin.(*PcapHandleWrapper)
			if !ok {
				log.Errorf("invalid handler: %v", origin)
				return
			}
			defer func() {
				handler.close()
			}()
			if err := _open(conf, ctx, handler); err != nil {
				log.Errorf("open device failed: %s", err)
			}
		})
	}

	return nil
}
