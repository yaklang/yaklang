package pcaputil

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func _open(conf *CaptureConfig, ctx context.Context, handler *PcapHandleWrapper) error {
	innerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if conf.recorder != nil {
		if err := conf.recorder.init(handler.LinkType()); err != nil {
			return err
		}
	}
	packetSource := gopacket.NewPacketSource(handler, handler.LinkType())
	packetSource.Lazy = true
	packetSource.NoCopy = true
	packetSource.DecodeStreamsAsDatagrams = true
	if conf.onNetInterfaceCreated != nil {
		conf.onNetInterfaceCreated(handler)
	}
	if conf.Filename != "" && len(conf.onEveryPacket) == 0 && conf.Output == nil && !conf.Debug {
		return openOfflineFast(conf, innerCtx, handler)
	}
	if conf.Filename == "" && conf.requiresExclusiveHandle() {
		return openLiveWorkers(conf, innerCtx, handler)
	}

	// Offline handles cannot block indefinitely. Avoid a packet channel and its
	// producer goroutine for files; preserve the cancellable path for live input.
	if conf.Filename != "" {
		for ctx.Err() == nil {
			packet, err := packetSource.NextPacket()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			conf.trafficPool.observeCapture(len(packet.Data()))
			if conf.recorder != nil {
				if err := conf.recorder.write(packet.Data(), packet.Metadata().CaptureInfo, handler.LinkType()); err != nil {
					return err
				}
			}
			conf.packetHandler(innerCtx, packet)
		}
		return nil
	}
	packets := packetSource.PacketsCtx(innerCtx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case packet := <-packets:
			if packet == nil {
				return nil
			}
			conf.trafficPool.observeCapture(len(packet.Data()))
			conf.packetHandler(innerCtx, packet)
		}
	}
}

func Start(opt ...CaptureOption) (resultErr error) {
	conf := NewDefaultConfig()
	for _, i := range opt {
		if err := i(conf); err != nil {
			return utils.Errorf("set option failed: %s", err)
		}
	}
	if err := conf.prepareBinParser(); err != nil {
		return err
	}
	if (conf.recorder != nil || conf.captureBuffer > 0) && conf.EnableCache {
		return errors.New("capture writer/buffer requires an exclusive capture handle")
	}
	if conf.captureBuffer > 0 && conf.Filename != "" {
		return errors.New("capture buffer applies only to live devices")
	}
	if conf.reassemblyOptions.Stream && conf.requiresFullStream {
		return utils.Errorf("TCP streaming cannot be combined with built-in HTTP/TLS parsers")
	}
	if conf.reassemblyOptions.Workers > 1 && conf.EnableCache {
		return utils.Errorf("TCP workers require an exclusive capture handle; disable capture cache")
	}
	handlers := omap.NewOrderedMap(map[string]PcapHandleOperation{})
	if conf.requiresExclusiveHandle() {
		defer handlers.ForEach(func(_ string, op PcapHandleOperation) bool {
			if h, ok := op.(*PcapHandleWrapper); ok {
				h.close()
			}
			return true
		})
	}
	if conf.Filename != "" {
		pcapHandler, err := OpenFile(conf.Filename)
		if err != nil {
			return err
		} else {
			if conf.BPFFilter != "" {
				if err := pcapHandler.SetBPFFilter(conf.BPFFilter); err != nil {
					pcapHandler.Close()
					return err
				}
			}
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
				if conf.requiresExclusiveHandle() {
					return err
				}
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
				if conf.requiresExclusiveHandle() {
					return err
				}
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
				if conf.requiresExclusiveHandle() {
					return err
				}
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
	conf.trafficPool = newTrafficPool(ctx, conf.reassemblyOptions)
	conf.trafficPool.captureAccountingAvailable = !conf.EnableCache
	defer func() {
		conf.trafficPool.Close()
		cancel()
		if err := conf.trafficPool.Err(); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
		if conf.onReassemblyStats != nil {
			conf.onReassemblyStats(conf.trafficPool.Stats())
		}
		resultErr = errors.Join(resultErr, conf.finishBinParser())
	}()

	conf.trafficPool.captureConf = conf
	for _, p := range conf.onPoolCreated {
		p(conf.trafficPool)
	}
	conf.trafficPool.startWorkers()

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
		var captureErr error
		var captureErrMu sync.Mutex
		utils.WaitRoutinesFromSlice(handlers.Values(), func(origin PcapHandleOperation) {
			handler, ok := origin.(*PcapHandleWrapper)
			if !ok {
				log.Errorf("invalid handler: %v", origin)
				return
			}
			defer func() {
				if conf.Filename == "" && conf.requiresExclusiveHandle() {
					conf.trafficPool.deviceStats(handler)
				}
				handler.close()
			}()
			if err := _open(conf, ctx, handler); err != nil {
				cancel()
				captureErrMu.Lock()
				captureErr = errors.Join(captureErr, err)
				captureErrMu.Unlock()
			}
		})
		if captureErr != nil {
			return captureErr
		}
	}

	return nil
}
