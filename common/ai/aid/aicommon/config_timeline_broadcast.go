package aicommon

import (
	"fmt"
	"sync"
)

// WithTimelineBroadcaster makes this Config join another Config's session
// broadcast group. Child configs normally inherit it through
// ConvertConfigToOptions.
func WithTimelineBroadcaster(b *TimelineBroadcaster) ConfigOption {
	return func(c *Config) error {
		if b == nil {
			return fmt.Errorf("timeline broadcaster is nil")
		}
		c.timelineBindingMu.Lock()
		defer c.timelineBindingMu.Unlock()
		if c.timelineBindingReady {
			return fmt.Errorf("timeline broadcaster must be configured during construction")
		}
		c.TimelineBroadcaster = b
		return nil
	}
}

// WithDisableTimelineBroadcast keeps helper timelines out of the session group.
func WithDisableTimelineBroadcast(disable bool) ConfigOption {
	return func(c *Config) error {
		c.timelineBindingMu.Lock()
		defer c.timelineBindingMu.Unlock()
		c.disableTimelineBroadcast = disable
		if c.timelineBindingReady {
			c.TimelineBroadcaster.Unsubscribe(c.timelineBroadcastSubscription)
			c.timelineBroadcastSubscription = 0
			if !disable {
				c.subscribeTimelineLocked(c.Timeline)
			}
		}
		return nil
	}
}

func (c *Config) initializeTimelineBroadcast() {
	if c == nil {
		return
	}
	c.timelineBindingMu.Lock()
	defer c.timelineBindingMu.Unlock()
	if c.TimelineBroadcaster == nil {
		c.TimelineBroadcaster = NewTimelineBroadcaster()
	}
	c.timelineBindingReady = true
	c.subscribeTimelineLocked(c.Timeline)
}

// SetTimeline switches this Config to another Timeline and moves its own
// subscription with it. Fork-owned subscriptions are managed by TimelineFork.
func (c *Config) SetTimeline(timeline *Timeline) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	c.timelineBindingMu.Lock()
	defer c.timelineBindingMu.Unlock()

	oldBroadcaster := c.TimelineBroadcaster
	oldSubscription := c.timelineBroadcastSubscription
	c.m.Lock()
	c.Timeline = timeline
	c.m.Unlock()
	if !c.timelineBindingReady {
		return nil
	}
	oldBroadcaster.Unsubscribe(oldSubscription)
	c.timelineBroadcastSubscription = 0
	c.subscribeTimelineLocked(timeline)
	return nil
}

func (c *Config) subscribeTimelineLocked(timeline *Timeline) {
	if timeline == nil || c.disableTimelineBroadcast {
		return
	}
	if c.TimelineBroadcaster == nil {
		c.TimelineBroadcaster = NewTimelineBroadcaster()
	}
	c.timelineBroadcastSubscription = c.TimelineBroadcaster.Subscribe(timeline.appendBroadcastItem)
}

// ReleaseTimelineSubscription removes this Config's own Timeline subscription.
func (c *Config) ReleaseTimelineSubscription() {
	if c == nil {
		return
	}
	c.timelineBindingMu.Lock()
	c.TimelineBroadcaster.Unsubscribe(c.timelineBroadcastSubscription)
	c.timelineBroadcastSubscription = 0
	c.timelineBindingMu.Unlock()
}

// SubscribeTimeline registers a Timeline which is owned outside this Config,
// such as a clean task-only branch. The returned function is idempotent.
func (c *Config) SubscribeTimeline(timeline *Timeline) (func(), error) {
	if c == nil || timeline == nil {
		return nil, fmt.Errorf("config and timeline are required")
	}
	c.timelineBindingMu.Lock()
	b := c.TimelineBroadcaster
	c.timelineBindingMu.Unlock()
	if b == nil {
		return nil, fmt.Errorf("config has no timeline broadcaster")
	}
	id := b.Subscribe(timeline.appendBroadcastItem)
	if id == 0 {
		return nil, fmt.Errorf("subscribe timeline broadcast failed")
	}
	var once sync.Once
	return func() {
		once.Do(func() { b.Unsubscribe(id) })
	}, nil
}

// BroadcastTimelineItem publishes one Timeline item to every live Timeline in
// this Config's session.
func (c *Config) BroadcastTimelineItem(item *TimelineItem) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if c.Ctx != nil && c.Ctx.Err() != nil {
		return c.Ctx.Err()
	}
	c.timelineBindingMu.Lock()
	b := c.TimelineBroadcaster
	c.timelineBindingMu.Unlock()
	if b == nil {
		return fmt.Errorf("config has no timeline broadcaster")
	}
	if err := b.Broadcast(item); err != nil {
		return err
	}
	emitTimelineItemAsync(c.GetEmitter(), item)
	return nil
}

// BroadcastTimelineText publishes a new text item.
func (c *Config) BroadcastTimelineText(text string) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	return c.BroadcastTimelineItem(NewTimelineItem(&TextTimelineItem{ID: c.AcquireId(), Text: text}))
}
