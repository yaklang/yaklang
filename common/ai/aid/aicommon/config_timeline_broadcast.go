package aicommon

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
)

// WithTimelineBroadcaster joins a running session's broadcast group. Options
// are resolved before subscribing, so its order relative to WithTimeline is safe.
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

// WithDisableTimelineBroadcast keeps one-shot/read-only helper timelines out
// of the running session's subscribers. It does not affect other configs.
func WithDisableTimelineBroadcast(disable bool) ConfigOption {
	return func(c *Config) error {
		c.timelineBindingMu.Lock()
		defer c.timelineBindingMu.Unlock()
		c.disableTimelineBroadcast = disable
		if c.timelineBindingReady {
			return c.bindTimelineLocked(c.Timeline)
		}
		return nil
	}
}

func (c *Config) initializeTimelineBroadcast() {
	c.timelineBindingMu.Lock()
	defer c.timelineBindingMu.Unlock()
	if c.TimelineBroadcaster == nil {
		// Legacy callers can share a live Timeline directly, without cloning
		// the parent's config options. Reuse that container's existing group.
		if c.Timeline != nil {
			c.TimelineBroadcaster = c.Timeline.getTimelineBroadcaster()
		}
		if c.TimelineBroadcaster == nil {
			c.TimelineBroadcaster = NewTimelineBroadcaster()
		}
	}
	c.timelineBindingReady = true
	if err := c.bindTimelineLocked(c.Timeline); err != nil {
		log.Warnf("subscribe config timeline: %v", err)
	}
}

// SetTimeline switches this config's subscription, without removing other
// configs or a fork handle that still owns the previous timeline.
func (c *Config) SetTimeline(timeline *Timeline) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	c.timelineBindingMu.Lock()
	defer c.timelineBindingMu.Unlock()
	if !c.timelineBindingReady {
		c.m.Lock()
		c.Timeline = timeline
		c.m.Unlock()
		return nil
	}
	return c.bindTimelineLocked(timeline)
}

func (c *Config) bindTimelineLocked(timeline *Timeline) error {
	old := c.timelineBroadcastLease
	var lease *timelineBroadcastLease
	if timeline != nil && !c.disableTimelineBroadcast {
		if c.TimelineBroadcaster == nil {
			c.TimelineBroadcaster = NewTimelineBroadcaster()
		}
		b := c.TimelineBroadcaster
		b.mu.Lock()
		if old != nil && !old.released && old.broadcaster == b && old.timelineID == timeline.GetTimelineID() {
			b.mu.Unlock()
			return nil
		}
		var err error
		lease, err = b.subscribeLocked(timeline, c.Ctx)
		if err != nil {
			b.mu.Unlock()
			return err
		}
		timeline.SoftBindConfig(c, c)
		b.mu.Unlock()
	}
	c.m.Lock()
	c.Timeline = timeline
	c.m.Unlock()
	c.timelineBroadcastLease = lease
	old.release()
	return nil
}

// ReleaseTimelineSubscription promptly detaches this config. Context
// cancellation is a fallback; repeated release is harmless.
func (c *Config) ReleaseTimelineSubscription() {
	if c == nil {
		return
	}
	c.timelineBindingMu.Lock()
	defer c.timelineBindingMu.Unlock()
	c.timelineBroadcastLease.release()
	c.timelineBroadcastLease = nil
}

// SubscribeTimeline reserves a live container before its child Config exists.
// The caller must release the returned handle even if child construction fails.
func (c *Config) SubscribeTimeline(timeline *Timeline) (func(), error) {
	if c == nil || timeline == nil {
		return nil, fmt.Errorf("config and timeline are required")
	}
	c.timelineBindingMu.Lock()
	b, ctx := c.TimelineBroadcaster, c.Ctx
	c.timelineBindingMu.Unlock()
	if b == nil {
		return nil, fmt.Errorf("config has no timeline broadcaster")
	}
	b.mu.Lock()
	lease, err := b.subscribeLocked(timeline, ctx)
	b.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return lease.release, nil
}

// BroadcastTimelineItem explicitly publishes one item to all live timelines in
// this config's session, including its own. Ordinary pushes remain local.
// Each recipient owns an independent value; the UI receives one event.
func (c *Config) BroadcastTimelineItem(item *TimelineItem) error {
	if c == nil || item == nil {
		return fmt.Errorf("config and timeline item are required")
	}
	c.timelineBindingMu.Lock()
	b, ctx := c.TimelineBroadcaster, c.Ctx
	c.timelineBindingMu.Unlock()
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if b == nil {
		return fmt.Errorf("config has no timeline broadcaster")
	}
	emitted, err := b.broadcast(item)
	if err == nil && emitted != nil {
		emitTimelineItemAsync(c.GetEmitter(), emitted)
	}
	return err
}

func (c *Config) BroadcastTimelineText(text string) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	return c.BroadcastTimelineItem(NewTimelineItem(&TextTimelineItem{ID: c.AcquireId(), Text: text}))
}
