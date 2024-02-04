package pcaputil

import (
	"bytes"
	"context"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
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
		value.handler.Close()
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
			return
		case <-time.After(3 * time.Second):
			getDaemonCache(key)
			log.Infof("keep daemon cache: %v", key)
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

func getInterfaceHandlerFromConfig(ifaceName string, conf *CaptureConfig) (string, *pcap.Handle, error) {
	if conf.EnableCache {
		var hashRaw bytes.Buffer
		hashRaw.WriteString(ifaceName)
		hashRaw.WriteString("|")
		if conf.OverrideCacheId != "" {
			hashRaw.WriteString(conf.BPFFilter)
		} else {
			hashRaw.WriteString("override|")
			hashRaw.WriteString(conf.OverrideCacheId)
		}
		cacheId := codec.Sha256(hashRaw.String())
		if handler, ok := getDaemonCache(cacheId); ok {
			return cacheId, handler.handler, nil
		}
		handler, err := OpenIfaceLive(ifaceName)
		if conf.BPFFilter != "" {
			if err := handler.SetBPFFilter(conf.BPFFilter); err != nil {
				return "", nil, err
			}
		}
		daemon := &daemonCache{
			handler:            handler,
			registeredHandlers: omap.NewOrderedMap(make(map[string]*pcapPacketHandlerContext)),
			startOnce:          new(sync.Once),
		}
		daemon.startOnce.Do(func() {
			if conf.BPFFilter != "" {
				log.Infof("background iface: %v with %s is start...", ifaceName, conf.BPFFilter)
			} else {
				log.Infof("background iface: %v is start...", ifaceName)
			}

			packetSource := gopacket.NewPacketSource(handler, handler.LinkType())
			go func() {
				defer func() {
					log.Infof("background iface: %v is stop...", ifaceName)
				}()
				for {
					select {
					case packet := <-packetSource.Packets():
						if packet == nil {
							return
						}
						var failedTrigger []string
						daemon.registeredHandlers.ForEach(func(i string, v *pcapPacketHandlerContext) bool {
							err := v.handler(v.ctx, packet)
							if err != nil {
								log.Errorf("%v handler error: %s", i, err)
								failedTrigger = append(failedTrigger, i)
							}
							return true
						})
						for _, i := range failedTrigger {
							daemon.registeredHandlers.Delete(i)
						}
					case <-time.After(3 * time.Second):
						if handler.Error() != nil {
							return
						}
					}
				}
			}()
		})
		registerDaemonCache(cacheId, daemon)
		return cacheId, handler, err
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
