package filesys

import (
	"context"
	"io/fs"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type PeepholeConfig struct {
	// peephole handler
	Size int

	// context
	ctx context.Context

	Callback         func(count, total int, f fi.FileSystem)
	fileContentCache *utils.SafeMap[[]byte]
}

func (c *PeepholeConfig) isStop() bool {
	if c == nil || c.ctx == nil {
		return false
	}

	select {
	case <-c.ctx.Done():
		return true
	default:
		return false
	}
}

type PeepholeTrigger struct {
	Path  string
	Infos []fs.FileInfo
}

type PeepholeConfigOption func(*PeepholeConfig) error

func WithPeepholeSize(i int) PeepholeConfigOption {
	return func(c *PeepholeConfig) error {
		if i <= 0 {
			return utils.Errorf("size must be positive")
		}
		c.Size = i
		return nil
	}
}

func WithPeepholeContext(ctx context.Context) PeepholeConfigOption {
	return func(c *PeepholeConfig) error {
		c.ctx = ctx
		return nil
	}
}

func WithPeepholeCallback(i func(int, int, fi.FileSystem)) PeepholeConfigOption {
	return func(config *PeepholeConfig) error {
		config.Callback = i
		return nil
	}
}

func defaultPeepholeConfig(opts ...PeepholeConfigOption) (*PeepholeConfig, error) {
	config := &PeepholeConfig{
		ctx:              context.Background(),
		Callback:         nil,
		fileContentCache: utils.NewSafeMap[[]byte](),
	}

	for _, o := range opts {
		if err := o(config); err != nil {
			return nil, err
		}
	}
	if config.Size == 0 {
		return nil, utils.Errorf("size must be set")
	}
	return config, nil
}

func (c *PeepholeConfig) CallbackFS(f fi.FileSystem, triggers *utils.SafeMap[*PeepholeTrigger]) {
	if c == nil || c.Callback == nil {
		return
	}

	// calculate step
	step := c.Size

	totalCount := 0
	triggers.ForEach(func(key string, trigger *PeepholeTrigger) bool {
		for i := 0; i < len(trigger.Infos); i += step {
			totalCount++
		}
		return true
	})

	count := 0
	createFS := func(path string, infos []fs.FileInfo) {
		count++
		// create virtual fs
		vfs := NewVirtualFs()
		for _, i := range infos {
			name := i.Name()
			filePath := f.Join(path, name)
			// get file content
			raw, ok := c.fileContentCache.Get(filePath)
			if !ok {
				var err error
				raw, err = f.ReadFile(filePath)
				if err != nil {
					log.Errorf("read file %s error %v", filePath, err)
				}
				c.fileContentCache.Set(filePath, raw)
			}
			vfs.AddFile(name, string(raw))
		}
		c.Callback(count, totalCount, vfs)
	}

	triggers.ForEach(func(key string, trigger *PeepholeTrigger) bool {
		// foreach trigger.Infos with size as step
		for i := 0; i < len(trigger.Infos); i += step {
			if c.isStop() {
				return false
			}
			end := i + step
			if end > len(trigger.Infos) {
				end = len(trigger.Infos)
			}
			infos := trigger.Infos[i:end]
			path := trigger.Path

			createFS(path, infos)
		}
		return true
	})

}

func Peephole(f fi.FileSystem, opts ...PeepholeConfigOption) error {
	var start string
	for _, entryPath := range []string{
		".", "", "/",
	} {
		entries, _ := f.ReadDir(entryPath)
		if len(entries) > 0 {
			start = entryPath
			break
		}
	}
	if start == "" {
		return utils.Error("no entry found")
	}
	c, err := defaultPeepholeConfig(opts...)
	if err != nil {
		return err
	}

	triggerCache := utils.NewSafeMap[*PeepholeTrigger]()
	recursiveErr := Recursive(
		start,
		WithFileSystem(f),
		WithContext(c.ctx),
		WithFileStat(func(s string, info fs.FileInfo) error {
			dirName, fileName := f.PathSplit(s)
			if fileName == "" {
				return nil
			}
			trigger, ok := triggerCache.Get(dirName)
			if !ok {
				trigger = &PeepholeTrigger{
					Path: dirName,
				}
				triggerCache.Set(dirName, trigger)
			}
			trigger.Infos = append(trigger.Infos, info)
			return nil
		}),
	)

	c.CallbackFS(f, triggerCache)
	return recursiveErr
}
