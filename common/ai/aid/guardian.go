package aid

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type GuardianEventTrigger func(event *Event, emitter GuardianEmitter, aicaller AICaller)

type GuardianMirrorStreamTrigger func(unlimitedChan *chanx.UnlimitedChan[*Event], emitter GuardianEmitter)

type asyncGuardian struct {
	ctx                  context.Context
	unlimitedInput       *chanx.UnlimitedChan[*Event]
	callbackMutex        *sync.RWMutex
	outputEmitter        GuardianEmitter
	mirrorCallback       map[string]*mirrorEventStream
	eventTriggerCallback map[EventType][]GuardianEventTrigger
	aiCaller             AICaller
}

type mirrorEventStream struct {
	triggerCallbackOnce *sync.Once

	unlimitedChan *chanx.UnlimitedChan[*Event]
	emitter       GuardianEmitter
	trigger       GuardianMirrorStreamTrigger
}

func newAsyncGuardian(ctx context.Context, coordinatorId string) *asyncGuardian {
	g := &asyncGuardian{
		ctx:                  ctx,
		outputEmitter:        newGuardianEmitter(coordinatorId, func(event *Event) {}),
		unlimitedInput:       chanx.NewUnlimitedChan[*Event](ctx, 1000),
		callbackMutex:        new(sync.RWMutex),
		eventTriggerCallback: make(map[EventType][]GuardianEventTrigger),
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

func (a *asyncGuardian) setOutputEmitter(coordinatorId string, emitter func(*Event)) {
	a.callbackMutex.Lock()
	defer a.callbackMutex.Unlock()
	a.outputEmitter = newGuardianEmitter(coordinatorId, emitter)
}

func (a *asyncGuardian) setAiCaller(caller AICaller) {
	a.callbackMutex.Lock()
	defer a.callbackMutex.Unlock()
	a.aiCaller = caller
}

func (a *asyncGuardian) feed(event *Event) {
	if event == nil {
		return
	}
	a.unlimitedInput.SafeFeed(event)
}

func (a *asyncGuardian) registerEventTrigger(eventType EventType, trigger GuardianEventTrigger) error {
	a.callbackMutex.Lock()
	defer a.callbackMutex.Unlock()
	if _, ok := a.eventTriggerCallback[eventType]; !ok {
		a.eventTriggerCallback[eventType] = make([]GuardianEventTrigger, 0)
	}
	a.eventTriggerCallback[eventType] = append(a.eventTriggerCallback[eventType], trigger)
	return nil
}

func (a *asyncGuardian) registerMirrorEventTrigger(mirrorName string, trigger GuardianMirrorStreamTrigger) error {
	a.callbackMutex.Lock()
	defer a.callbackMutex.Unlock()
	if _, ok := a.mirrorCallback[mirrorName]; ok {
		return utils.Errorf("GuardianMirrorStreamTrigger for mirror name %s already registered", mirrorName)
	}
	a.mirrorCallback[mirrorName] = &mirrorEventStream{
		triggerCallbackOnce: new(sync.Once),
		unlimitedChan:       chanx.NewUnlimitedChan[*Event](a.ctx, 1000),
		emitter:             a.outputEmitter,
		trigger:             trigger,
	}
	return nil
}

func (a *asyncGuardian) eventloop(ch chan struct{}) {
	onceLoop := new(sync.Once)
	outputChan := a.unlimitedInput.OutputChannel()
	for {
		onceLoop.Do(func() {
			close(ch)
		})
		select {
		case result, ok := <-outputChan:
			if !ok {
				log.Warn("AsyncGuardian output channel closed")
				return
			}
			a.emitEvent(result)
		case <-a.ctx.Done():
			log.Warn("AsyncGuardian context closed")
			return
		}
	}
}

func (a *asyncGuardian) emitEvent(event *Event) {
	a.callbackMutex.RLock()
	defer a.callbackMutex.RUnlock()

	if triggers, ok := a.eventTriggerCallback[event.Type]; ok {
		for _, trigger := range triggers {
			trigger(event, a.outputEmitter, a.aiCaller)
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
				mirror.trigger(mirror.unlimitedChan, a.outputEmitter)
			}()
		})
		mirror.unlimitedChan.SafeFeed(event)
	}
}
