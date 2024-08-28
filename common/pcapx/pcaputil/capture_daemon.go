package pcaputil

import (
	"bytes"
	"context"
	"github.com/google/gopacket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"sync"
	"time"
)

var captureDaemonCache = utils.NewTTLCache[*daemonCache](10 * time.Second)

func init() {
	captureDaemonCache.SetExpirationCallback(func(key string, value *daemonCache) {
		value.handler.close()
	})
}

func registerDaemonCache(key string, handler *daemonCache) {
	captureDaemonCache.Set(key, handler)
}

func getDaemonCache(key string) (*daemonCache, bool) {
	return captureDaemonCache.Get(key)
}

func syncKeepDaemonCache(key string, ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			log.Debugf("keep daemon cache: %v is stop", key)
			return
		case <-time.After(3 * time.Second):
			log.Debugf("keep daemon cache: %v", key)
			getDaemonCache(key)
		}
	}
}

func keepDaemonCache(key string, ctx context.Context) context.CancelFunc {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	go syncKeepDaemonCache(key, ctx)
	return func() {
		cancel()
	}
}

var getInterfaceHandlerMutex = new(sync.Mutex)

func getInterfaceHandlerFromConfig(ifaceName string, conf *CaptureConfig) (string, PcapHandleOperation, error) {
	if conf.EnableCache {
		getInterfaceHandlerMutex.Lock()
		defer getInterfaceHandlerMutex.Unlock()
		var hashRaw bytes.Buffer
		hashRaw.WriteString(ifaceName)
		hashRaw.WriteString("|")
		if conf.OverrideCacheId == "" {
			hashRaw.WriteString(conf.BPFFilter)
		} else {
			hashRaw.WriteString("override|")
			hashRaw.WriteString(conf.OverrideCacheId)
		}
		cacheId := codec.Sha256(hashRaw.String())
		if daemon, ok := getDaemonCache(cacheId); ok {
			if conf.onNetInterfaceCreated != nil { // 取缓存时 检测是否有新的 onNetInterfaceCreated 回调
				if oldHandle, ok := daemon.handler.(*PcapHandleWrapper); ok {
					conf.onNetInterfaceCreated(oldHandle)
				}
			}
			return cacheId, daemon.handler, nil
		}

		var handler *PcapHandleWrapper
		var operation PcapHandleOperation
		var err error

		if conf.mock != nil {
			operation = conf.mock
		} else {
			pcapHandler, err := OpenIfaceLive(ifaceName)
			if err != nil {
				return "", nil, err
			}
			handler = WrapPcapHandle(pcapHandler)
			operation = handler
		}
		if conf.BPFFilter != "" {
			if err := operation.SetBPFFilter(conf.BPFFilter); err != nil {
				return "", nil, err
			}
		}
		daemon := &daemonCache{
			handler:            operation,
			registeredHandlers: omap.NewOrderedMap(make(map[string]*pcapPacketHandlerContext)),
			startOnce:          new(sync.Once),
		}

		if conf.mock == nil {
			daemon.startOnce.Do(func() {
				if conf.BPFFilter != "" {
					log.Infof("background iface: %v with %s is start...", ifaceName, conf.BPFFilter)
				} else {
					log.Infof("background iface: %v is start...", ifaceName)
				}

				packetSource := gopacket.NewPacketSource(handler, handler.LinkType())
				packetSource.Lazy = true
				packetSource.NoCopy = true
				packetSource.DecodeStreamsAsDatagrams = true
				source := packetSource.Packets()
				onceFirstPacket := new(sync.Once)

				go func() {
					defer func() {
						log.Infof("background iface: %v is stop...", ifaceName)
					}()

					onceFirstPacket.Do(func() {
						// first packet
						if conf.onNetInterfaceCreated != nil {
							conf.onNetInterfaceCreated(handler)
						}
					})

					for {
						select {
						case packet := <-source:
							if packet == nil {
								return
							}
							var failedTrigger []string
							daemon.registeredHandlers.ForEach(func(i string, v *pcapPacketHandlerContext) bool {
								err := v.handler(v.ctx, packet)
								if err != nil {
									//defer daemon.handler.Close()
									log.Debugf("%v handler error: %s", i, err)
									failedTrigger = append(failedTrigger, i)
								}
								return true
							})
							for _, i := range failedTrigger {
								daemon.registeredHandlers.Delete(i)
							}
						case <-time.After(3 * time.Second):
							if handler == nil {
								log.Errorf("background iface: %v handler is nil", ifaceName)
								return
							} else if handler.Error() != nil && handler.Error().Error() != "" {
								log.Errorf("background iface: %v error: %s", ifaceName, handler.Error())
								captureDaemonCache.Remove(cacheId)
								return
							}
						}
					}
				}()
			})
		} else {
			daemon.startOnce.Do(func() {
				log.Infof("mock background iface: %v is start...", ifaceName)
				go func() {
					defer func() {
						log.Infof("mock background iface: %v is stop...", ifaceName)
					}()
					for {

						time.Sleep(time.Millisecond * 100)
						var failedTrigger []string
						daemon.registeredHandlers.ForEach(func(i string, v *pcapPacketHandlerContext) bool {
							err := v.handler(v.ctx, nil)
							if err != nil {
								log.Errorf("%v handler error: %s", i, err)
								failedTrigger = append(failedTrigger, i)
							}
							return true
						})
						for _, i := range failedTrigger {
							daemon.registeredHandlers.Delete(i)
						}
					}
				}()
			})
		}
		registerDaemonCache(cacheId, daemon)
		return cacheId, daemon.handler, err
	}
	pcapHandler, err := OpenIfaceLive(ifaceName)
	if err != nil {
		return "", nil, err
	}
	handler := WrapPcapHandle(pcapHandler)
	err = handler.SetBPFFilter(conf.BPFFilter)
	if err != nil {
		return "", handler, err
	}
	return "", handler, err
}

func registerCallback(key string, callbackId string, originCtx context.Context, handler pcapPacketHandler) {
	cache, ok := getDaemonCache(key)
	if !ok {
		return
	}
	cache.registeredHandlers.Set(callbackId, &pcapPacketHandlerContext{
		ctx:     originCtx,
		handler: handler,
	})
}
