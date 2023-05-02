package yaklib

import (
	"sync"
	"github.com/yaklang/yaklang/common/utils"
)

var SyncExport = map[string]interface{}{
	"NewWaitGroup":      func() *sync.WaitGroup { return new(sync.WaitGroup) },
	"NewSizedWaitGroup": func(size int) *utils.SizedWaitGroup { swg := utils.NewSizedWaitGroup(size); return &swg },
	"NewMutex": func() *sync.Mutex {
		return new(sync.Mutex)
	},
	"NewLock": func() *sync.Mutex {
		return new(sync.Mutex)
	},
	"NewMap": func() *sync.Map {
		return new(sync.Map)
	},
	"NewOnce": func() *sync.Once {
		return new(sync.Once)
	},
	"NewRWMutex": func() *sync.RWMutex {
		return new(sync.RWMutex)
	},
	"NewPool": func() *sync.Pool {
		return new(sync.Pool)
	},
	"NewCond": func() *sync.Cond {
		return sync.NewCond(new(sync.Mutex))
	},
}
