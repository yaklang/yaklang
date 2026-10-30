//go:build hids && linux

package filewatch

import (
	"context"
	"errors"
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
	spec                  model.FileCollectorSpec
	mu                    sync.RWMutex
	active                *filewatchRun
	directoryMu           sync.Mutex
	state                 filewatchCollectorState
	maxWatchedDirectories int
}

type filewatchRun struct {
	watcher *fsnotify.Watcher
	cancel  context.CancelFunc
	done    chan struct{}
	err     error
}

const defaultMaxWatchedDirectories = 8192
const directoryDiscoveryQueueSize = 64

var errFilewatchDirectoryLimitExceeded = errors.New("filewatch directory limit exceeded")

func New(spec model.FileCollectorSpec) hidscollector.Instance {
	return &Collector{
		spec:                  spec,
		state:                 newFilewatchCollectorState(spec.WatchPaths),
		maxWatchedDirectories: defaultMaxWatchedDirectories,
	}
}

func (c *Collector) Name() string {
	return "filewatch"
}

func (c *Collector) Start(ctx context.Context, sink chan<- model.Event) error {
	return c.start(ctx, sink, c.tryWatchNewDirectory)
}

func (c *Collector) start(ctx context.Context, sink chan<- model.Event, discover func(context.Context, *fsnotify.Watcher, string)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil {
		return errors.New("filewatch collector is already started")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		cancel()
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}

	for _, watchPath := range c.spec.WatchPaths {
		if err := c.addRecursive(ctx, watcher, watchPath); err != nil {
			cancel()
			_ = watcher.Close()
			c.state.setStopped()
			return err
		}
	}

	active := &filewatchRun{watcher: watcher, cancel: cancel, done: make(chan struct{})}
	c.active = active
	c.state.setRunning()

	go c.run(ctx, active, sink, discover)
	return nil
}

func (c *Collector) Close() error {
	c.mu.RLock()
	active := c.active
	c.mu.RUnlock()
	if active == nil {
		return nil
	}
	active.cancel()
	<-active.done
	return active.err
}

func (c *Collector) run(
	ctx context.Context,
	active *filewatchRun,
	sink chan<- model.Event,
	discover func(context.Context, *fsnotify.Watcher, string),
) {
	watcher := active.watcher
	directories := make(chan string, directoryDiscoveryQueueSize)
	reconcile := make(chan struct{}, 1)
	discoveryDone := make(chan struct{})
	go func() {
		defer close(discoveryDone)
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case path := <-directories:
				if ctx.Err() == nil {
					discover(ctx, watcher, path)
				}
			case <-reconcile:
				// A burst can exceed the bounded path queue. Merge those misses
				// into one root rescan so a temporary overload cannot leave a
				// permanently unwatched directory below the configured roots.
				for _, root := range c.spec.WatchPaths {
					if ctx.Err() != nil {
						return
					}
					discover(ctx, watcher, root)
				}
			}
		}
	}()
	defer func() {
		active.cancel()
		<-discoveryDone
		// fsnotify v1's Close is not safe to call concurrently. The run owns
		// its watcher; all external Close callers only cancel and join it.
		active.err = watcher.Close()
		c.mu.Lock()
		c.active = nil
		c.state.setStopped()
		close(active.done)
		c.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			c.state.observeReceived()
			observation := c.toEvent(event)
			if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
				c.forgetDirectoryTree(watcher, event.Name)
			}
			if event.Op&fsnotify.Create != 0 && observation.File != nil && observation.File.IsDir {
				// Traversing a moved-in tree must not pause delivery of unrelated
				// file events. One worker and a bounded queue cap discovery work.
				select {
				case directories <- event.Name:
				default:
					c.state.observeDropped()
					c.state.observeError(errors.New("filewatch directory discovery queue is full; coverage may be incomplete until configured roots are reconciled"))
					select {
					case reconcile <- struct{}{}:
					default:
					}
				}
			}
			c.publish(ctx, sink, observation)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			c.state.observeError(err)
		}
	}
}

func (c *Collector) addRecursive(ctx context.Context, watcher *fsnotify.Watcher, root string) error {
	return c.walkDirectories(ctx, watcher, root, false)
}

func (c *Collector) walkDirectories(ctx context.Context, watcher *fsnotify.Watcher, root string, allowMissing bool) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if allowMissing && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if shouldSkipDirectory(d) {
			return filepath.SkipDir
		}
		c.directoryMu.Lock()
		defer c.directoryMu.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		// WalkDir's entry may predate a concurrent rename/removal. Serialize
		// actual Add/forget operations and recheck the path before recording it.
		info, err := os.Lstat(path)
		if err != nil {
			if allowMissing && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			return err
		}
		if !info.IsDir() {
			return filepath.SkipDir
		}
		if c.state.hasDirectory(path) {
			return nil
		}
		if c.watchedDirectoryCount() >= c.maxWatchedDirectoryLimit() {
			return fmt.Errorf("%w: max_directories=%d root=%s", errFilewatchDirectoryLimitExceeded, c.maxWatchedDirectoryLimit(), root)
		}
		if err := watcher.Add(path); err != nil {
			if allowMissing && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			return fmt.Errorf("watch path %s: %w", path, err)
		}
		current, err := os.Lstat(path)
		if err != nil || !current.IsDir() || !os.SameFile(info, current) {
			_ = watcher.Remove(path)
			if allowMissing && (err == nil || errors.Is(err, fs.ErrNotExist)) {
				return filepath.SkipDir
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("watch path changed during discovery: %s", path)
		}
		c.state.observeDirectory(path)
		return nil
	})
}

func (c *Collector) forgetDirectoryTree(watcher *fsnotify.Watcher, root string) {
	c.directoryMu.Lock()
	defer c.directoryMu.Unlock()
	if !c.state.hasDirectoryTree(root) {
		return
	}
	for _, path := range c.state.forgetDirectoryTree(root) {
		// The kernel may already have removed the watch after deletion.
		_ = watcher.Remove(path)
	}
}

func (c *Collector) tryWatchNewDirectory(ctx context.Context, watcher *fsnotify.Watcher, path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	if err := c.walkDirectories(ctx, watcher, path, true); err != nil && ctx.Err() == nil {
		c.state.observeError(err)
	}
}

func (c *Collector) watchedDirectoryCount() int {
	c.state.mu.RLock()
	defer c.state.mu.RUnlock()

	return len(c.state.dirs)
}

func (c *Collector) maxWatchedDirectoryLimit() int {
	if c == nil || c.maxWatchedDirectories <= 0 {
		return defaultMaxWatchedDirectories
	}
	return c.maxWatchedDirectories
}

func shouldSkipDirectory(entry fs.DirEntry) bool {
	if entry == nil {
		return false
	}
	name := entry.Name()
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor", ".venv", "venv", "__pycache__", ".cache":
		return true
	default:
		return false
	}
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
