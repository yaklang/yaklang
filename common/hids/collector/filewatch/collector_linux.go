//go:build hids && linux

package filewatch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	hidscollector "github.com/yaklang/yaklang/common/hids/collector"
	"github.com/yaklang/yaklang/common/hids/enrich"
	"github.com/yaklang/yaklang/common/hids/model"
)

type Collector struct {
	spec    model.FileCollectorSpec
	mu      sync.RWMutex
	watcher *fsnotify.Watcher
	state   filewatchCollectorState
}

func New(spec model.FileCollectorSpec) hidscollector.Instance {
	return &Collector{
		spec:  spec,
		state: newFilewatchCollectorState(spec.WatchPaths),
	}
}

func (c *Collector) Name() string {
	return "filewatch"
}

func (c *Collector) Start(ctx context.Context, sink chan<- model.Event) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}

	for _, watchPath := range c.spec.WatchPaths {
		if err := c.addRecursive(watcher, watchPath); err != nil {
			_ = watcher.Close()
			return err
		}
	}

	c.mu.Lock()
	c.watcher = watcher
	c.mu.Unlock()
	c.state.setRunning()

	go c.run(ctx, watcher, sink)
	return nil
}

func (c *Collector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.watcher == nil {
		return nil
	}
	err := c.watcher.Close()
	c.watcher = nil
	c.state.setStopped()
	return err
}

func (c *Collector) run(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	sink chan<- model.Event,
) {
	defer c.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			c.state.observeReceived()
			if event.Op&fsnotify.Create != 0 {
				c.tryWatchNewDirectory(watcher, event.Name)
			}
			c.publish(ctx, sink, c.toEvent(event))
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			c.state.observeError(err)
		}
	}
}

func (c *Collector) addRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("watch path %s: %w", path, err)
		}
		c.state.observeDirectory(path)
		return nil
	})
}

func (c *Collector) tryWatchNewDirectory(watcher *fsnotify.Watcher, path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	_ = c.addRecursive(watcher, path)
}

func (c *Collector) toEvent(event fsnotify.Event) model.Event {
	fileInfo, err := os.Lstat(event.Name)
	fileModel := &model.File{
		Path:      event.Name,
		Operation: event.Op.String(),
	}
	if err == nil {
		identity := enrich.FileIdentityFromFileInfo(fileInfo)
		fileModel.IsDir = identity.IsDir
		fileModel.Mode = identity.Mode
		fileModel.UID = identity.UID
		fileModel.GID = identity.GID
		fileModel.Owner = identity.Owner
		fileModel.Group = identity.Group
	}

	return model.Event{
		Type:      model.EventTypeFileChange,
		Source:    "filewatch",
		Timestamp: timeNowUTC(),
		Tags:      []string{"file", "filewatch"},
		File:      fileModel,
		Data: map[string]any{
			"backend": "filewatch",
		},
	}
}

func (c *Collector) publish(
	ctx context.Context,
	sink chan<- model.Event,
	event model.Event,
) {
	select {
	case sink <- event:
		c.state.observeEmitted(event.Timestamp)
	case <-ctx.Done():
	default:
		c.state.observeDropped()
	}
}

func (c *Collector) HealthSnapshot() hidscollector.HealthSnapshot {
	return c.state.snapshot()
}

func timeNowUTC() time.Time { return time.Now().UTC() }
