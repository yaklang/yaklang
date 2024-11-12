package filesys

import (
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"io/fs"
	"sync"
)

type PeepholeConfig struct {
	Size     int
	Callback func(f fi.FileSystem)

	cache *sync.Map
}

type PeepholeTrigger struct {
	Path  string
	Infos []fs.FileInfo
}

type PeepholeConfigOption func(*PeepholeConfig)

func WithPeepholeSize(i int) PeepholeConfigOption {
	return func(c *PeepholeConfig) {
		if i > 0 {
			c.Size = i
		}
	}
}

func WithPeepholeCallback(i func(system fi.FileSystem)) PeepholeConfigOption {
	return func(config *PeepholeConfig) {
		config.Callback = i
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

	config := &PeepholeConfig{
		Size:     3,
		Callback: nil,
		cache:    new(sync.Map),
	}

	for _, o := range opts {
		o(config)
	}
	
	recursiveErr := Recursive(start, WithFileSystem(f), WithFileStat(func(s string, info fs.FileInfo) error {
		dirName, fileName := f.PathSplit(s)
		if fileName == "" {
			return nil
		}

		var trigger *PeepholeTrigger
		anyTrigger, ok := config.cache.Load(dirName)
		if !ok {
			trigger = &PeepholeTrigger{
				Path: dirName,
			}
			config.cache.Store(dirName, trigger)
		} else {
			trigger = anyTrigger.(*PeepholeTrigger)
		}
		trigger.Infos = append(trigger.Infos, info)
		if len(trigger.Infos) >= config.Size {
			vfs := NewVirtualFs()
			for _, i := range trigger.Infos {
				raw, _ := f.ReadFile(i.Name())
				vfs.AddFile(i.Name(), string(raw))
			}
			if config.Callback != nil {
				config.Callback(vfs)
			}
			config.cache.Delete(dirName)
		}
		return nil
	}))
	config.cache.Range(func(key, value any) bool {
		result, ok := value.(*PeepholeTrigger)
		if !ok {
			return true
		}
		vfs := NewVirtualFs()
		for _, i := range result.Infos {
			raw, _ := f.ReadFile(i.Name())
			vfs.AddFile(i.Name(), string(raw))
		}
		if config.Callback != nil {
			config.Callback(vfs)
		}
		return true
	})
	return recursiveErr
}
