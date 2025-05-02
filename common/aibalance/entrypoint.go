package aibalance

import (
	"math/rand"
	"sync"

	"github.com/yaklang/yaklang/common/utils/omap"
)

type ModelEntry struct {
	ModelName string      `json:"model_name"`
	Providers []*Provider `json:"providers"`
}

type Entrypoint struct {
	ModelEntries *omap.OrderedMap[string, *ModelEntry]

	m sync.Mutex
}

func NewEntrypoint() *Entrypoint {
	return &Entrypoint{
		ModelEntries: omap.NewOrderedMap(make(map[string]*ModelEntry)),
	}
}

func (e *Entrypoint) CreateModelEntry(modelName string) *ModelEntry {
	return &ModelEntry{
		ModelName: modelName,
		Providers: []*Provider{},
	}
}

func (e *Entrypoint) AddProvider(modelName string, provider *Provider) {
	if entry, ok := e.ModelEntries.Get(modelName); ok {
		entry.Providers = append(entry.Providers, provider)
	} else {
		e.ModelEntries.Set(modelName, &ModelEntry{
			ModelName: modelName,
			Providers: []*Provider{provider},
		})
	}
}

func (e *Entrypoint) PeekProvider(modelName string) *Provider {
	if entry, ok := e.ModelEntries.Get(modelName); ok {
		if len(entry.Providers) > 0 {
			// 随机选择一个提供者
			randomIndex := rand.Intn(len(entry.Providers))
			return entry.Providers[randomIndex]
		}
	}
	return nil
}
