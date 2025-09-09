package aicommon

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type ContextProvider func(config AICallerConfigIf, emitter *Emitter, key string) (string, error)

type ContextProviderManager struct {
	maxBytes int
	m        sync.RWMutex
	callback *omap.OrderedMap[string, ContextProvider]
}

func NewContextProviderManager() *ContextProviderManager {
	return &ContextProviderManager{
		maxBytes: 10 * 1024, // 10KB
		callback: omap.NewOrderedMap(make(map[string]ContextProvider)),
	}
}

func (r *ContextProviderManager) Register(name string, cb ContextProvider) {
	r.m.Lock()
	defer r.m.Unlock()
	if r.callback.Have(name) {
		log.Warnf("context provider %s already registered, ignore, if you want to use new callback, unregister first", name)
		return
	}
	r.callback.Set(name, func(config AICallerConfigIf, emitter *Emitter, key string) (_ string, finalErr error) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("context provider %s panic: %v", name, err)
				utils.PrintCurrentGoroutineRuntimeStack()
				finalErr = utils.Errorf("context provider %s panic: %v", name, err)
			}
		}()
		return cb(config, emitter, key)
	})
}

func (r *ContextProviderManager) Unregister(name string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.callback.Delete(name)
}

func (r *ContextProviderManager) Execute(config AICallerConfigIf, emitter *Emitter) string {
	r.m.RLock()
	defer r.m.RUnlock()

	if r.callback.Len() == 0 {
		return ""
	}

	var buf bytes.Buffer
	r.callback.ForEach(func(name string, cb ContextProvider) bool {
		result, err := cb(config, emitter, name)
		if err != nil {
			result = `[Error getting context: ` + err.Error() + `]`
		}
		flag := utils.RandStringBytes(4)
		buf.WriteString(fmt.Sprintf("<|AUTO_PROVIDE_CTX_[%v]_START key=%v|>\n", flag, name))
		buf.WriteString(result)
		buf.WriteString(fmt.Sprintf("\n<|AUTO_PROVIDE_CTX_[%v]_END|>", flag))
		return true
	})

	result := buf.String()
	if len(result) > r.maxBytes {
		shrinkSize := int(float64(r.maxBytes) * 0.8)
		result = utils.ShrinkString(result, shrinkSize)
		log.Warnf("context provider result exceeded maxBytes (%d), shrunk to %d characters", r.maxBytes, shrinkSize)
	}

	return result
}
