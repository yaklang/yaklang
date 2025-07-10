package aid

import (
	"context"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"sync"
	"time"
)

func (c *Config) startEventLoop(ctx context.Context) {
	c.startInputEventOnce.Do(func() {
		consumptionUUID := ksuid.New().String()
		validator := make(chan struct{})
		go func() {
			log.Infof("config %s started, start to handle receiving loop", c.id)
			logOnce := new(sync.Once)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			consumptionNotification := func() {
				if c.GetInputConsumption() > 0 || c.GetOutputConsumption() > 0 {
					c.emitJson(
						EVENT_TYPE_CONSUMPTION,
						"system",
						map[string]any{
							"input_consumption":  c.GetInputConsumption(),
							"output_consumption": c.GetOutputConsumption(),
							"consumption_uuid":   consumptionUUID,
						},
					)
				}
			}

			tickerCallback := func() {
				consumptionNotification()
			}
			for {
				if c.eventInputChan == nil {
					logOnce.Do(func() {
						log.Infof("event input chan is nil, will retry in 1 second")
					})
					select {
					case <-validator:
						log.Infof("coordinator validator working, (%v) start", c.id)
						continue
					case <-ticker.C:
						tickerCallback()
						continue
					case <-ctx.Done():
						return
					}
				}

				select {
				case <-validator:
					log.Infof("coordinator validator working, (%v) start", c.id)
					continue
				case event, ok := <-c.eventInputChan:
					if !ok {
						log.Errorf("event input channel closed, (%v) start", c.id)
						return
					}
					if event == nil {
						continue
					}

					log.Infof("event received, (%v) start: %v", c.id, event)

					if event.IsInteractive || event.Id != "" {
						c.epm.feed(event.Id, event.Params)
						continue
					}

					if event.IsSyncInfo {
						switch event.SyncType {
						case SYNC_TYPE_CONSUMPTION:
							consumptionNotification()
						case SYNC_TYPE_PING:
							c.emitJson(EVENT_TYPE_PONG, "system", map[string]any{
								"now":         time.Now().Format(time.RFC3339),
								"now_unix":    time.Now().Unix(),
								"now_unix_ms": time.Now().UnixMilli(),
							})
						case SYNC_TYPE_PLAN:
							c.syncMutex.RLock()
							callback, _ := c.syncMap[string(SYNC_TYPE_PLAN)]
							c.syncMutex.RUnlock()
							if callback != nil {
								c.emitJson(EVENT_TYPE_PLAN, "system", map[string]any{
									"root_task": callback(),
								})
							} else {
								c.EmitWarning("sync method: %v is not supported yet", SYNC_TYPE_PLAN)
							}

						}
					}
				case <-ticker.C:
					tickerCallback()
					continue
				case <-ctx.Done():
					return
				}
			}
		}()
		validator <- struct{}{}
	})
}
