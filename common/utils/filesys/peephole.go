package filesys

import (
	"io/fs"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type PeepholeConfig struct {
	// peephole handler
	Size int

	Callback         func(f fi.FileSystem)
	fileContentCache *utils.SafeMap[[]byte]
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

func WithPeepholeCallback(i func(system fi.FileSystem)) PeepholeConfigOption {
	return func(config *PeepholeConfig) error {
		config.Callback = i
		return nil
	}
}

func defaultPeepholeConfig(opts ...PeepholeConfigOption) (*PeepholeConfig, error) {
	config := &PeepholeConfig{
		Callback: nil,
		// triggerCache:     utils.NewSafeMap[*PeepholeTrigger](),
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

func (c *PeepholeConfig) CallbackFS(f fi.FileSystem, trigger *PeepholeTrigger) {
	if c == nil || c.Callback == nil {
		return
	}
	// calculate step
	step := len(trigger.Infos)
	if c.Size != 0 {
		step = c.Size
	}

	// foreach trigger.Infos with size as step
	for i := 0; i < len(trigger.Infos); i += step {
		end := i + step
		if end > len(trigger.Infos) {
			end = len(trigger.Infos)
		}
		infos := trigger.Infos[i:end]
		path := trigger.Path

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
		c.Callback(vfs)
	}
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

	triggerCache.ForEach(func(key string, trigger *PeepholeTrigger) bool {
		c.CallbackFS(f, trigger)
		return true
	})

	return recursiveErr
}
