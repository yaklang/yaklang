package limitedmap

import (
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type Map[T any] struct {
	m    map[string]T
	lock *sync.RWMutex
}

func NewReadOnlyMap[T any](a map[string]T) *Map[T] {
	if utils.IsNil(a) {
		a = make(map[string]T)
	}
	return &Map[T]{
		m:    a,
		lock: new(sync.RWMutex),
	}
}

func (rom *Map[T]) Load(key string) (value T, ok bool) {
	rom.lock.RLock()
	defer rom.lock.RUnlock()
	value, ok = rom.m[key]
	return
}

func (rom *Map[T]) ForEach(h func(key string, value T) error) {
	rom.lock.RLock()
	defer rom.lock.RUnlock()
	for k, v := range rom.m {
		if err := h(k, v); err != nil {
			return
		}
	}
}
