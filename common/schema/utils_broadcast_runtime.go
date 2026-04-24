package schema

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var runtimeScopedBroadcastThrottleInterval = 1.0

type RuntimeScopedBroadcastEvent struct {
	Type      string `json:"type"`
	RuntimeID string `json:"runtime_id"`
	Action    string `json:"action,omitempty"`
	ID        uint   `json:"id,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

type runtimeScopedBroadcastCenter struct {
	mu            sync.Mutex
	subscriberSeq uint64
	routes        map[string]*runtimeScopedBroadcastRoute
}

type runtimeScopedBroadcastRoute struct {
	key        string
	typeString string
	runtimeID  string
	center     *runtimeScopedBroadcastCenter

	mu           sync.Mutex
	subscribers  map[uint64]func(*RuntimeScopedBroadcastEvent)
	throttleByID map[string]func(func())
}

var runtimeBroadcastData = newRuntimeScopedBroadcastCenter()

func newRuntimeScopedBroadcastCenter() *runtimeScopedBroadcastCenter {
	return &runtimeScopedBroadcastCenter{
		routes: make(map[string]*runtimeScopedBroadcastRoute),
	}
}

func runtimeScopedBroadcastKey(typeString, runtimeID string) string {
	return typeString + "\x00" + runtimeID
}

func SubscribeRuntimeScopedBroadcast(typeString, runtimeID string, handler func(*RuntimeScopedBroadcastEvent)) func() {
	return runtimeBroadcastData.Subscribe(typeString, runtimeID, handler)
}

func PublishRuntimeScopedBroadcast(typeString, runtimeID, action string, ids ...uint) {
	runtimeBroadcastData.Publish(typeString, runtimeID, action, ids...)
}

func (c *runtimeScopedBroadcastCenter) Subscribe(typeString, runtimeID string, handler func(*RuntimeScopedBroadcastEvent)) func() {
	if c == nil || typeString == "" || runtimeID == "" || handler == nil {
		return func() {}
	}

	route := c.getOrCreateRoute(typeString, runtimeID)
	subscriberID := atomic.AddUint64(&c.subscriberSeq, 1)

	route.mu.Lock()
	route.subscribers[subscriberID] = handler
	route.mu.Unlock()

	return func() {
		c.unsubscribe(route.key, subscriberID)
	}
}

func (c *runtimeScopedBroadcastCenter) Publish(typeString, runtimeID, action string, ids ...uint) {
	if c == nil || typeString == "" || runtimeID == "" {
		return
	}

	route := c.getRoute(typeString, runtimeID)
	if route == nil {
		return
	}

	if len(ids) == 0 {
		route.publish(&RuntimeScopedBroadcastEvent{
			Type:      typeString,
			RuntimeID: runtimeID,
			Action:    action,
		})
		return
	}

	for _, id := range ids {
		route.publish(&RuntimeScopedBroadcastEvent{
			Type:      typeString,
			RuntimeID: runtimeID,
			Action:    action,
			ID:        id,
		})
	}
}

func (c *runtimeScopedBroadcastCenter) getRoute(typeString, runtimeID string) *runtimeScopedBroadcastRoute {
	key := runtimeScopedBroadcastKey(typeString, runtimeID)

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.routes[key]
}

func (c *runtimeScopedBroadcastCenter) getOrCreateRoute(typeString, runtimeID string) *runtimeScopedBroadcastRoute {
	key := runtimeScopedBroadcastKey(typeString, runtimeID)

	c.mu.Lock()
	defer c.mu.Unlock()

	if route, ok := c.routes[key]; ok {
		return route
	}

	route := &runtimeScopedBroadcastRoute{
		key:          key,
		typeString:   typeString,
		runtimeID:    runtimeID,
		center:       c,
		subscribers:  make(map[uint64]func(*RuntimeScopedBroadcastEvent)),
		throttleByID: make(map[string]func(func())),
	}
	c.routes[key] = route
	return route
}

func (c *runtimeScopedBroadcastCenter) unsubscribe(key string, subscriberID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	route, ok := c.routes[key]
	if !ok {
		return
	}

	route.mu.Lock()
	delete(route.subscribers, subscriberID)
	empty := len(route.subscribers) == 0
	route.mu.Unlock()

	if empty {
		delete(c.routes, key)
	}
}

func (r *runtimeScopedBroadcastRoute) publish(event *RuntimeScopedBroadcastEvent) {
	if event == nil {
		return
	}

	jsonMsg := utils.Jsonify(struct {
		Type      string `json:"type"`
		RuntimeID string `json:"runtime_id"`
		Action    string `json:"action,omitempty"`
		ID        uint   `json:"id,omitempty"`
	}{
		Type:      event.Type,
		RuntimeID: event.RuntimeID,
		Action:    event.Action,
		ID:        event.ID,
	})
	hash := utils.CalcMd5(r.key, jsonMsg)

	r.mu.Lock()
	caller, ok := r.throttleByID[hash]
	if !ok {
		caller = utils.NewThrottle(runtimeScopedBroadcastThrottleInterval)
		r.throttleByID[hash] = caller
	}
	r.mu.Unlock()

	caller(func() {
		r.dispatch(event)
	})
}

func (r *runtimeScopedBroadcastRoute) dispatch(event *RuntimeScopedBroadcastEvent) {
	r.mu.Lock()
	subscribers := make([]func(*RuntimeScopedBroadcastEvent), 0, len(r.subscribers))
	for _, subscriber := range r.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	r.mu.Unlock()

	eventToSend := *event
	eventToSend.Timestamp = time.Now().UnixNano()

	for _, subscriber := range subscribers {
		go safeCallRuntimeScopedSubscriber(subscriber, &eventToSend)
	}
}

func safeCallRuntimeScopedSubscriber(handler func(*RuntimeScopedBroadcastEvent), event *RuntimeScopedBroadcastEvent) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("runtime scoped broadcast subscriber panic: %v", err)
		}
	}()
	handler(event)
}
