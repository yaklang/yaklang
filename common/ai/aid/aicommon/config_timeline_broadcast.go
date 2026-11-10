package aicommon

import "fmt"

// WithTimelineBroadcaster makes this Config join another session's broadcast
// group. Child configs normally inherit it through ConvertConfigToOptions.
func WithTimelineBroadcaster(b *TimelineBroadcaster) ConfigOption {
	return func(c *Config) error {
		if b == nil {
			return fmt.Errorf("timeline broadcaster is nil")
		}
		c.setTimelineBroadcaster(b)
		return nil
	}
}

func (c *Config) timelineBroadcastState() (*Timeline, *TimelineBroadcaster) {
	if c == nil {
		return nil, nil
	}
	if c.m == nil {
		return c.Timeline, c.TimelineBroadcaster
	}
	c.m.Lock()
	defer c.m.Unlock()
	return c.Timeline, c.TimelineBroadcaster
}

func (c *Config) setTimelineBroadcaster(b *TimelineBroadcaster) {
	if c == nil {
		return
	}
	if c.m == nil {
		c.TimelineBroadcaster = b
		return
	}
	c.m.Lock()
	c.TimelineBroadcaster = b
	c.m.Unlock()
}

func (c *Config) setTimelineForBroadcast(timeline *Timeline) *TimelineBroadcaster {
	if c == nil {
		return nil
	}
	if c.m == nil {
		c.Timeline = timeline
		return c.TimelineBroadcaster
	}
	c.m.Lock()
	c.Timeline = timeline
	broadcaster := c.TimelineBroadcaster
	c.m.Unlock()
	return broadcaster
}

func configTimelineBroadcastID(c *Config) string {
	return fmt.Sprintf("config:%p", c)
}

func (c *Config) initializeTimelineBroadcast() {
	if c == nil {
		return
	}
	timeline, broadcaster := c.timelineBroadcastState()
	if broadcaster == nil {
		broadcaster = NewTimelineBroadcaster()
		c.setTimelineBroadcaster(broadcaster)
	}
	if timeline != nil {
		broadcaster.Subscribe(configTimelineBroadcastID(c), timeline.appendBroadcastItem)
	}
}

// SetTimeline switches this Config to another Timeline. The broadcaster
// replaces this Config's previous callback because the subscription key is
// stable for the Config lifetime.
func (c *Config) SetTimeline(timeline *Timeline) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	broadcaster := c.setTimelineForBroadcast(timeline)
	if broadcaster == nil {
		return nil
	}
	if timeline == nil {
		broadcaster.Unsubscribe(configTimelineBroadcastID(c))
		return nil
	}
	broadcaster.Subscribe(configTimelineBroadcastID(c), timeline.appendBroadcastItem)
	return nil
}

// ReleaseTimelineSubscription removes this Config's Timeline callback.
func (c *Config) ReleaseTimelineSubscription() {
	if c == nil {
		return
	}
	_, broadcaster := c.timelineBroadcastState()
	broadcaster.Unsubscribe(configTimelineBroadcastID(c))
}

// SubscribeTimeline registers a Timeline owned outside this Config, such as a
// clean task-only branch. The returned function is idempotent.
func (c *Config) SubscribeTimeline(timeline *Timeline) (func(), error) {
	if c == nil || timeline == nil {
		return nil, fmt.Errorf("config and timeline are required")
	}
	_, broadcaster := c.timelineBroadcastState()
	if broadcaster == nil {
		return nil, fmt.Errorf("config has no timeline broadcaster")
	}
	key := "timeline:" + timeline.GetTimelineID()
	broadcaster.Subscribe(key, timeline.appendBroadcastItem)
	return func() { broadcaster.Unsubscribe(key) }, nil
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
	_, broadcaster := c.timelineBroadcastState()
	if broadcaster == nil {
		return fmt.Errorf("config has no timeline broadcaster")
	}
	if err := broadcaster.Broadcast(item); err != nil {
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
