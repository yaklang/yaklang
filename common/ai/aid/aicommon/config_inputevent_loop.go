package aicommon

import (
	"context"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
	"time"
)

func (c *Config) StartEventLoop(ctx context.Context) {
	c.StartEventLoopEx(ctx, nil, nil)
}

func (c *Config) StartEventLoopEx(ctx context.Context, startCall func(), doneCall func()) {
	c.RegisterBasicSyncHandlers()
	c.StartInputEventOnce.Do(func() {
		if c.consumptionUUID == "" {
			c.consumptionUUID = ksuid.New().String()
		}
		validator := make(chan struct{})
		go func() {
			//log.Infof("config %s started, start to handle receiving loop", c.id)
			logOnce := new(sync.Once)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			defer func() {
				if doneCall != nil {
					log.Infof("event loop done call for config %s", c.id)
					doneCall()
				}
			}()

			if startCall != nil {
				log.Infof("event loop start call for config %s", c.id)
				startCall()
			}

			consumptionNotification := func() {
				if c.GetInputConsumption() > 0 || c.GetOutputConsumption() > 0 {
					c.EmitJSON(
						schema.EVENT_TYPE_CONSUMPTION,
						"system",
						map[string]any{
							"input_consumption":  c.GetInputConsumption(),
							"output_consumption": c.GetOutputConsumption(),
							"consumption_uuid":   c.consumptionUUID,
						},
					)
				}
			}

			tickerCallback := func() {
				consumptionNotification()
			}
			for {
				if c.EventInputChan == nil {
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
				case event, ok := <-c.EventInputChan.OutputChannel():
					if !ok {
						log.Errorf("event input channel closed, (%v) start", c.id)
						return
					}
					if event == nil {
						continue
					}

					if c.DebugEvent {
						log.Infof("event loop processing event: IsFreeInput=%v, IsInteractive=%v",
							event.IsFreeInput, event.IsInteractiveMessage)
					}

					if event.IsConfigHotpatch {
						hotPatchOptions := ProcessHotPatchMessage(event)
						for _, option := range hotPatchOptions {
							c.HotPatchOptionChan.SafeFeed(option)
						}
						continue
					}

					go func(event *ypb.AIInputEvent) {
						if err := c.processInputEvent(event); err != nil {
							log.Errorf("ReAct event processing failed: %v", err)
						}
					}(event)
				case <-ticker.C:
					tickerCallback()
					continue
				case <-ctx.Done():
					return
				}
			}
		}()
		select {
		case validator <- struct{}{}:
		case <-ctx.Done():
		}
	})
	c.StartHotPatchLoop(ctx)
}

// processInputEvent processes a single input event and triggers ReAct loop
func (c *Config) processInputEvent(event *ypb.AIInputEvent) error {
	if c.DebugEvent {
		log.Infof("Processing input event: IsFreeInput=%v, IsInteractive=%v", event.IsFreeInput, event.IsInteractiveMessage)
	}

	if c.InputEventManager != nil {
		c.InputEventManager.CallMirrorOfAIInputEvent(event)
	}

	if event.IsInteractiveMessage { // interactive message is fixed
		if event.InteractiveId != "" {
			hasSend := false
			err := jsonextractor.ExtractStructuredJSON(
				event.InteractiveJSONInput,
				jsonextractor.WithObjectCallback(func(data map[string]any) {
					_, ok := data["suggestion"]
					if !ok {
						return
					}
					params := aitool.InvokeParams(data)
					c.Epm.Feed(event.InteractiveId, params)
					hasSend = true
				}),
			)
			if err != nil {
				return err
			}
			if !hasSend { // default continue
				c.Epm.Feed(event.InteractiveId, aitool.InvokeParams{
					"suggestion": "continue",
				})
			}

		}
	} else if c.InputEventManager != nil {
		return c.InputEventManager.processEvent(event) // process other input events, can register different callbacks
	}

	return nil
}

type AIInputEventProcessor struct {
	syncCallback      map[string]func(event *ypb.AIInputEvent) error
	freeInputCallback func(event *ypb.AIInputEvent) error
	mirrorCallback    map[string]func(event *ypb.AIInputEvent)
	mu                sync.Mutex
}

func NewAIInputEventProcessor() *AIInputEventProcessor {
	return &AIInputEventProcessor{
		syncCallback:   make(map[string]func(event *ypb.AIInputEvent) error),
		mirrorCallback: make(map[string]func(event *ypb.AIInputEvent)),
		mu:             sync.Mutex{},
	}
}

func (p *AIInputEventProcessor) RegisterSyncCallback(syncType string, callback func(event *ypb.AIInputEvent) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.syncCallback == nil {
		p.syncCallback = make(map[string]func(event *ypb.AIInputEvent) error)
	}
	p.syncCallback[syncType] = callback
}

func (p *AIInputEventProcessor) UnRegisterSyncCallback(syncType string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.syncCallback != nil {
		delete(p.syncCallback, syncType)
	}
}

func (p *AIInputEventProcessor) SetFreeInputCallback(callback func(event *ypb.AIInputEvent) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.freeInputCallback = callback
}

func (p *AIInputEventProcessor) UnRegisterFreeInputCallback(inputType string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.freeInputCallback = nil
}

func (p *AIInputEventProcessor) processEvent(event *ypb.AIInputEvent) error {
	if event.IsSyncMessage {
		p.mu.Lock()
		callback, exists := p.syncCallback[event.SyncType]
		p.mu.Unlock()
		if exists && callback != nil {
			return callback(event)
		}
	}
	if event.IsFreeInput {
		p.mu.Lock()
		callBack := p.freeInputCallback
		p.mu.Unlock()
		if callBack != nil {
			return callBack(event)
		}
	}
	return nil
}

func (p *AIInputEventProcessor) RegisterMirrorOfAIInputEvent(id string, f func(*ypb.AIInputEvent)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mirrorCallback[id] = f
}

func (p *AIInputEventProcessor) CallMirrorOfAIInputEvent(event *ypb.AIInputEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, f := range p.mirrorCallback {
		f(event)
	}
}

func (p *AIInputEventProcessor) UnregisterMirrorOfAIInputEvent(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.mirrorCallback, id)
}
