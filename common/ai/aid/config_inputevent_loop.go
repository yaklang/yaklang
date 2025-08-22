package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func (c *Config) initSyncGuardian() {
	if c.syncGuardian == nil {
		c.syncGuardian = aicommon.NewSyncGuardian(c.Emitter)
	}
	c.syncGuardian.RegisterSyncFunc(SYNC_TYPE_CONSUMPTION, schema.EVENT_TYPE_CONSUMPTION)
	c.syncGuardian.RegisterSyncFunc(SYNC_TYPE_PLAN, schema.EVENT_TYPE_PLAN)
	c.syncGuardian.RegisterSyncFunc(SYNC_TYPE_CONFIG, schema.EVENT_TYPE_AI_CONFIG)
	c.syncGuardian.RegisterSyncFunc(SYNC_TYPE_CURRENT_TASK, schema.EVENT_TYPE_CURRENT_TASK)

	c.syncGuardian.SetSyncData(SYNC_TYPE_CONFIG, func() any {
		return c.SimpleInfoMap()
	})
}

func (c *Config) startEventLoop(ctx context.Context) {
	c.initSyncGuardian()
	c.startInputEventOnce.Do(func() {
		consumptionUUID := ksuid.New().String()
		validator := make(chan struct{})
		go func() {
			log.Infof("config %s started, start to handle receiving loop", c.id)
			logOnce := new(sync.Once)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			consumptionDataGetter := func() any {
				return map[string]any{
					"input_consumption":  c.GetInputConsumption(),
					"output_consumption": c.GetOutputConsumption(),
					"consumption_uuid":   consumptionUUID,
				}
			}
			c.syncGuardian.SetSyncData(SYNC_TYPE_CONSUMPTION, consumptionDataGetter)
			consumptionNotification := func() {
				if c.GetInputConsumption() > 0 || c.GetOutputConsumption() > 0 {
					c.EmitJSON(
						schema.EVENT_TYPE_CONSUMPTION,
						"system",
						consumptionDataGetter(),
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
						c.epm.Feed(event.Id, event.Params)
						continue
					}

					if event.IsSyncInfo {
						err := c.syncGuardian.Process(event.SyncType, event.Params)
						if err != nil {
							log.Error(err)
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
