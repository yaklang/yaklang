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
	c.RegisterBasicSyncHandlers()
	c.StartInputEventOnce.Do(func() {
		c.consumptionUUID = ksuid.New().String()
		validator := make(chan struct{})
		go func() {
			//log.Infof("config %s started, start to handle receiving loop", c.id)
			logOnce := new(sync.Once)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

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
		validator <- struct{}{}
	})
}


// processInputEvent processes a single input event and triggers ReAct loop
func (c *Config) processInputEvent(event *ypb.AIInputEvent) error {
	if c.DebugEvent {
		log.Infof("Processing input event: IsFreeInput=%v, IsInteractive=%v", event.IsFreeInput, event.IsInteractiveMessage)
	}

	if event.IsInteractiveMessage { // interactive message is fixed
		if event.InteractiveId != "" {
			err := jsonextractor.ExtractStructuredJSON(
				event.InteractiveJSONInput,
				jsonextractor.WithObjectCallback(func(data map[string]any) {
					sug, ok := data["suggestion"]
					if !ok || sug == "" {
						sug = "continue" // Default fallback if no suggestion provided
					}

					params := aitool.InvokeParams(data)
					c.Epm.Feed(event.InteractiveId, params)
				}),
			)
			if err != nil {
				return err
			}
		}
	}else if c.InputEventManager != nil {
		return c.InputEventManager.processEvent(event) // process other input events, can register different callbacks
	}
	return nil
}


type AIInputEventProcessor struct {
	SyncCallback map[string]func(event *ypb.AIInputEvent) error
	FreeInputCallback map[string]func(event *ypb.AIInputEvent) error
	mu sync.Mutex
}

func NewAIInputEventProcessor() *AIInputEventProcessor {
	return &AIInputEventProcessor{
		SyncCallback:     make(map[string]func(event *ypb.AIInputEvent) error),
		FreeInputCallback: make(map[string]func(event *ypb.AIInputEvent) error),
		mu: sync.Mutex{},
	}
}

func (p *AIInputEventProcessor) RegisterSyncCallback(syncType string, callback func(event *ypb.AIInputEvent) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.SyncCallback == nil {
		p.SyncCallback = make(map[string]func(event *ypb.AIInputEvent) error)
	}
	p.SyncCallback[syncType] = callback
}


func (p *AIInputEventProcessor) UnRegisterSyncCallback(syncType string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.SyncCallback != nil {
		delete(p.SyncCallback, syncType)
	}
}


func (p *AIInputEventProcessor) RegisterFreeInputCallback(inputType string, callback func(event *ypb.AIInputEvent) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.FreeInputCallback == nil {
		p.FreeInputCallback = make(map[string]func(event *ypb.AIInputEvent) error)
	}
	p.FreeInputCallback[inputType] = callback
}

func (p *AIInputEventProcessor) UnRegisterFreeInputCallback(inputType string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.FreeInputCallback != nil {
		delete(p.FreeInputCallback, inputType)
	}
}

func (p *AIInputEventProcessor) processEvent(event *ypb.AIInputEvent) error {
	if event.IsSyncMessage {
		p.mu.Lock()
		callback, exists := p.SyncCallback[event.SyncType]
		p.mu.Unlock()
		if exists && callback != nil {
			return callback(event)
		}
	}
	if event.IsFreeInput {
		p.mu.Lock()
		callback, exists := p.FreeInputCallback["free_input"]
		p.mu.Unlock()
		if exists && callback != nil {
			return callback(event)
		}
	}
	return nil
}


