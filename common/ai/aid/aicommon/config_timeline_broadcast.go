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
		broadcaster.Register(configTimelineBroadcastID(c), timeline.appendBroadcastItem)
	}
}

// SetTimeline switches this Config to another Timeline. The broadcaster
// replaces this Config's previous callback because the registration key is
// stable for the Config lifetime.
func (c *Config) SetTimeline(timeline *Timeline) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	var broadcaster *TimelineBroadcaster
	if c.m == nil {
		c.Timeline = timeline
		broadcaster = c.TimelineBroadcaster
	} else {
		c.m.Lock()
		c.Timeline = timeline
		broadcaster = c.TimelineBroadcaster
		c.m.Unlock()
	}
	if broadcaster == nil {
		return nil
	}
	if timeline == nil {
		broadcaster.Unregister(configTimelineBroadcastID(c))
		return nil
	}
	timeline.SoftBindConfig(c, c)
	broadcaster.Register(configTimelineBroadcastID(c), timeline.appendBroadcastItem)
	return nil
}

// UnregisterTimeline removes this Config's Timeline callback.
func (c *Config) UnregisterTimeline() {
	if c == nil {
		return
	}
	_, broadcaster := c.timelineBroadcastState()
	broadcaster.Unregister(configTimelineBroadcastID(c))
}

// RegisterTimeline registers a Timeline owned outside this Config, such as a
// clean task-only branch. The returned function is idempotent.
func (c *Config) RegisterTimeline(timeline *Timeline) (func(), error) {
	if c == nil || timeline == nil {
		return nil, fmt.Errorf("config and timeline are required")
	}
	_, broadcaster := c.timelineBroadcastState()
	if broadcaster == nil {
		return nil, fmt.Errorf("config has no timeline broadcaster")
	}
	key := "timeline:" + timeline.GetTimelineID()
	broadcaster.Register(key, timeline.appendBroadcastItem)
	return func() { broadcaster.Unregister(key) }, nil
}

// BroadcastTimelineItem publishes one or more Timeline items to every live
// Timeline in this Config's session.
func (c *Config) BroadcastTimelineItem(items ...*TimelineItem) error {
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
	if err := broadcaster.Broadcast(items...); err != nil {
		return err
	}
	return nil
}

// BroadcastTimelineText publishes a new text item.
func (c *Config) BroadcastTimelineText(text string) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	return c.BroadcastTimelineItem(NewTimelineItem(&TextTimelineItem{Text: text}))
}
