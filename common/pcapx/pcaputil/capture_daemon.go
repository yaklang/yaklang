package pcaputil

import (
	"bytes"
	"context"
	"github.com/google/gopacket"
	"github.com/jellydator/ttlcache/v3"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"sync"
	"time"
)

var captureDaemonCache *ttlcache.Cache[string, *daemonCache]

func init() {
	captureDaemonCache = ttlcache.New[string, *daemonCache](
		ttlcache.WithTTL[string, *daemonCache](10 * time.Second),
	)

	captureDaemonCache.OnInsertion(func(ctx context.Context, item *ttlcache.Item[string, *daemonCache]) {
		log.Warnf("daemon cache: %v is insert %s", item.Key(), item.ExpiresAt())
	})
	captureDaemonCache.OnEviction(func(ctx context.Context, reason ttlcache.EvictionReason, item *ttlcache.Item[string, *daemonCache]) {
		if reason == ttlcache.EvictionReasonExpired {
			log.Debugf("daemon cache: %v is expired", item.Key())
			item.Value().handler.Close()
		}

		if reason == ttlcache.EvictionReasonDeleted {
			log.Debugf("daemon cache: %v is deleted", item.Key())
			item.Value().handler.Close()
		}
	})
	go func() {
		for {
			time.Sleep(10 * time.Second)
			captureDaemonCache.DeleteExpired()
		}
	}()
}

func registerDaemonCache(key string, handler *daemonCache) {
	if !captureDaemonCache.Has(key) {
		captureDaemonCache.Set(key, handler, ttlcache.DefaultTTL)
	}
}

func getDaemonCache(key string) (*daemonCache, bool) {
	if captureDaemonCache.Has(key) {
		item := captureDaemonCache.Get(key)
		if item == nil {
			log.Debugf("daemon cache: %v is nil", key)
			return nil, false
		}
		return item.Value(), true
	}
	return nil, false
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
		case <-time.After(5 * time.Second):
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

func getInterfaceHandlerFromConfig(ifaceName string, conf *CaptureConfig) (string, PcapHandleOperation, error) {
	if conf.EnableCache {
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
				if oldHandle, ok := daemon.handler.(*pcap.Handle); ok {
					conf.onNetInterfaceCreated(oldHandle)
				}
			}
			return cacheId, daemon.handler, nil
		}

		var handler *pcap.Handle
		var operation PcapHandleOperation
		var err error

		if conf.mock != nil {
			operation = conf.mock
		} else {
			handler, err = OpenIfaceLive(ifaceName)
			if err != nil {
				return "", nil, err
			}
			operation = handler
		}
		if conf.BPFFilter != "" {
			if err := operation.SetBPFFilter(conf.BPFFilter); err != nil {
				return "", nil, err
			}
		}
		daemon := &daemonCache{
			handler:            handler,
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

				packetSource := gopacket.NewPacketSource(handler, handler.LinkType()).Packets()

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
						case packet := <-packetSource:
							if packet == nil {
								return
							}
							var failedTrigger []string
							daemon.registeredHandlers.ForEach(func(i string, v *pcapPacketHandlerContext) bool {
								err := v.handler(v.ctx, packet)
								if err != nil {
									//defer daemon.handler.Close()
									log.Errorf("%v handler error: %s", i, err)
									failedTrigger = append(failedTrigger, i)
								}
								return true
							})
							for _, i := range failedTrigger {
								daemon.registeredHandlers.Delete(i)
							}
						case <-time.After(3 * time.Second):
							if handler.Error() != nil && handler.Error().Error() != "" {
								log.Errorf("background iface: %v error: %s", ifaceName, handler.Error())
								captureDaemonCache.Delete(cacheId)
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
	handler, err := OpenIfaceLive(ifaceName)
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
