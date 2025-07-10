package utils

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"sync"
	"time"
)

type EventWatcherManager struct {
	triggerTime  time.Duration
	triggerCount int
	watchingMap  map[string]*chanx.UnlimitedChan[struct{}]
	mu           sync.Mutex
	ctx          context.Context
}

func NewEntityWatcher(ctx context.Context, triggerTime time.Duration, triggerCount int) *EventWatcherManager {
	return &EventWatcherManager{
		triggerTime:  triggerTime,
		triggerCount: triggerCount,
		watchingMap:  make(map[string]*chanx.UnlimitedChan[struct{}]),
		ctx:          ctx,
	}
}
func (ew *EventWatcherManager) StopWatch(key string) {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	if ch, exists := ew.watchingMap[key]; exists {
		ch.CloseForce()
		delete(ew.watchingMap, key)
	}
}

func (ew *EventWatcherManager) Watch(key string, callback func(key string), firstWatch func(key string)) {
	ew.mu.Lock()
	ch, exists := ew.watchingMap[key]
	ew.mu.Unlock()
	if !exists {
		firstWatch(key)
		ctx, cancel := context.WithCancel(ew.ctx)
		watchChannel := chanx.NewUnlimitedChan[struct{}](ctx, 2)
		ew.watchingMap[key] = watchChannel
		defer ew.StopWatch(key)
		defer cancel()
		triggerCount := ew.triggerCount
		count := 0

		tr := time.NewTimer(ew.triggerTime)
		var ok bool
		for !ok {
			select {
			case <-watchChannel.OutputChannel():
				count++
				if count >= triggerCount {
					ok = true
				}
			case <-tr.C:
				ok = true
			case <-ew.ctx.Done():
				return
			}
		}
		callback(key)
	} else {
		ch.SafeFeed(struct{}{})
	}
}
