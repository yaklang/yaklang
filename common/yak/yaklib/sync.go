package yaklib

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

type WaitGroupProxy struct {
	*sync.WaitGroup
}

func (w *WaitGroupProxy) Add(delta ...int) {
	n := 1
	if len(delta) > 0 {
		n = delta[0]
	}
	w.WaitGroup.Add(n)
}

func NewWaitGroup() *WaitGroupProxy {
	return &WaitGroupProxy{&sync.WaitGroup{}}
}

func NewSizedWaitGroup(size int) *utils.SizedWaitGroup {
	swg := utils.NewSizedWaitGroup(size)
	return swg
}

var SyncExport = map[string]interface{}{
	"NewWaitGroup":      NewWaitGroup,
	"NewSizedWaitGroup": NewSizedWaitGroup,
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
