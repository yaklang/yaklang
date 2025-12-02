package aicommon

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

// GuardianEmitter defines the interface for emitting events in the Guardian system.
// it provides some restricted methods for emitting events based on an ordinary Emitter
type GuardianEmitter interface {
	EmitStatus(key string, value any) (*schema.AiOutputEvent, error)
	EmitStructured(nodeId string, result any) (*schema.AiOutputEvent, error)
	EmitGuardianStreamEvent(nodeId string, startTime time.Time, reader io.Reader) (*schema.AiOutputEvent, error)
	EmitJson(typeName schema.EventType, nodeId string, i any) (*schema.AiOutputEvent, error)
	WaitForStream()
}

type guardianEmitter struct {
	*Emitter
}

func NewGuardianEmitter(coordinatorId string, emitter func(*schema.AiOutputEvent)) *guardianEmitter {
	baseEmitter := func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		emitter(e)
		return e, nil
	}
	return &guardianEmitter{
		Emitter: NewEmitter(coordinatorId, baseEmitter),
	}
}

func (e *guardianEmitter) EmitGuardianStreamEvent(nodeId string, startTime time.Time, reader io.Reader) (*schema.AiOutputEvent, error) {
	return e.EmitSystemStreamEvent(nodeId, startTime, reader, "")
}

func (e *guardianEmitter) EmitJson(typeName schema.EventType, nodeId string, i any) (*schema.AiOutputEvent, error) {
	return e.EmitJSON(typeName, nodeId, i)
}

var _ GuardianEmitter = (*guardianEmitter)(nil)

// GuardianEventTrigger defines a function type for handling AI output events in the Guardian system.
type GuardianEventTrigger func(event *schema.AiOutputEvent, emitter GuardianEmitter, aicaller AICaller)

// GuardianMirrorStreamTrigger defines a function type for triggering a mirror stream in the Guardian system.
type GuardianMirrorStreamTrigger func(unlimitedChan *chanx.UnlimitedChan[*schema.AiOutputEvent], emitter GuardianEmitter)

type AsyncGuardian struct {
	ctx                  context.Context
	unlimitedInput       *chanx.UnlimitedChan[*schema.AiOutputEvent]
	rwm                  *sync.RWMutex
	emitter              GuardianEmitter
	mirrorCallback       map[string]*mirrorEventStream
	eventTriggerCallback map[schema.EventType][]GuardianEventTrigger
	aiCaller             AICaller
}

type mirrorEventStream struct {
	triggerCallbackOnce *sync.Once

	unlimitedChan *chanx.UnlimitedChan[*schema.AiOutputEvent]
	emitter       GuardianEmitter
	trigger       GuardianMirrorStreamTrigger
}

func NewAsyncGuardian(ctx context.Context, coordinatorId string) *AsyncGuardian {
	g := &AsyncGuardian{
		ctx:                  ctx,
		emitter:              NewGuardianEmitter(coordinatorId, func(event *schema.AiOutputEvent) {}),
		unlimitedInput:       chanx.NewUnlimitedChan[*schema.AiOutputEvent](ctx, 1000),
		rwm:                  new(sync.RWMutex),
		eventTriggerCallback: make(map[schema.EventType][]GuardianEventTrigger),
		mirrorCallback:       make(map[string]*mirrorEventStream),
	}
	ch := make(chan struct{})
	go g.eventloop(ch)
	select {
	case _, ok := <-ch:
		if ok {
			log.Info("AsyncGuardian eventloop started")
		}
	}
	return g
}

func (a *AsyncGuardian) SetOutputEmitter(coordinatorId string, emitter func(event *schema.AiOutputEvent)) {
	a.rwm.Lock()
	defer a.rwm.Unlock()
	a.emitter = NewGuardianEmitter(coordinatorId, emitter)
}

func (a *AsyncGuardian) SetAICaller(caller AICaller) {
	a.rwm.Lock()
	defer a.rwm.Unlock()
	a.aiCaller = CreateProxyAICaller(caller, func(req *AIRequest) *AIRequest {
		if req != nil {
			req.SetDetachCheckpoint(true)
		}
		return req
	})
}

func (a *AsyncGuardian) Feed(event *schema.AiOutputEvent) {
	if event == nil {
		return
	}
	if a.unlimitedInput != nil {
		a.unlimitedInput.SafeFeed(event)
	}
}

func (a *AsyncGuardian) RegisterEventTrigger(eventType schema.EventType, trigger GuardianEventTrigger) error {
	a.rwm.Lock()
	defer a.rwm.Unlock()

	if trigger == nil {
		return nil
	}

	if _, exists := a.eventTriggerCallback[eventType]; !exists {
		a.eventTriggerCallback[eventType] = make([]GuardianEventTrigger, 0)
	}
	a.eventTriggerCallback[eventType] = append(a.eventTriggerCallback[eventType], trigger)
	return nil
}

func (a *AsyncGuardian) RegisterMirrorStreamTrigger(mirror string, trigger GuardianMirrorStreamTrigger) error {
	a.rwm.Lock()
	defer a.rwm.Unlock()

	if trigger == nil {
		return nil
	}

	if _, exists := a.mirrorCallback[mirror]; !exists {
		a.mirrorCallback[mirror] = &mirrorEventStream{
			triggerCallbackOnce: new(sync.Once),
			unlimitedChan:       chanx.NewUnlimitedChan[*schema.AiOutputEvent](a.ctx, 1000),
			emitter:             a.emitter,
			trigger:             trigger,
		}
	}
	a.mirrorCallback[mirror].trigger = trigger
	return nil
}

func (a *AsyncGuardian) eventloop(ch chan struct{}) {
	onceLoop := new(sync.Once)

	outputChan := a.unlimitedInput.OutputChannel()
	for {
		onceLoop.Do(func() {
			close(ch)
		})
		select {
		case event, ok := <-outputChan:
			if !ok {
				log.Warn("AsyncGuardian input channel closed, exiting event loop")
				return
			}
			a.emitEvent(event)
		case <-a.ctx.Done():
			log.Debug("AsyncGuardian context closed, exiting event loop")
			return
		}
	}
}

func (a *AsyncGuardian) emitEvent(event *schema.AiOutputEvent) {
	a.rwm.RLock()
	defer a.rwm.RUnlock()

	if event == nil {
		log.Warn("Received nil event, skipping emit")
		return
	}

	if triggers, ok := a.eventTriggerCallback[event.Type]; ok {
		for _, trigger := range triggers {
			if trigger != nil {
				trigger(event, a.emitter, a.aiCaller)
			}
		}
	}

	for _, mirror := range a.mirrorCallback {
		mirror.triggerCallbackOnce.Do(func() {
			go func() {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("GuardianEmitter panic: %v", utils.ErrorStack(err))
					}
				}()
				mirror.trigger(mirror.unlimitedChan, a.emitter)
			}()
		})
		mirror.unlimitedChan.SafeFeed(event)
	}
}

func (a *AsyncGuardian) GetContext() context.Context {
	return a.ctx
}

func (a *AsyncGuardian) GetUnlimitedInput() *chanx.UnlimitedChan[*schema.AiOutputEvent] {
	return a.unlimitedInput
}

func (a *AsyncGuardian) GetRWMutex() *sync.RWMutex {
	return a.rwm
}

func (a *AsyncGuardian) GetEventTriggerCallback() map[schema.EventType][]GuardianEventTrigger {
	a.rwm.RLock()
	defer a.rwm.RUnlock()
	return a.eventTriggerCallback
}

func (a *AsyncGuardian) GetMirrorCallback() map[string]*mirrorEventStream {
	a.rwm.RLock()
	defer a.rwm.RUnlock()
	return a.mirrorCallback
}
