package aicommon

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

type AttachedResourceData interface {
	ToAttachData(loop ReActLoopIF) string
	Type() string
	BindLoopData(loop ReActLoopIF) error
	Unmarshal(raw string) error
}

type AttachedResourceDataFactory func() AttachedResourceData

var attachedResourceDataFactories = struct {
	sync.RWMutex
	items map[string]AttachedResourceDataFactory
}{
	items: make(map[string]AttachedResourceDataFactory),
}

func RegisterAttachedResourceDataFactory(typ string, factory AttachedResourceDataFactory, aliases ...string) {
	if strings.TrimSpace(typ) == "" || factory == nil {
		return
	}
	attachedResourceDataFactories.Lock()
	defer attachedResourceDataFactories.Unlock()

	attachedResourceDataFactories.items[NormalizeAttachedResourceType(typ)] = factory
	for _, alias := range aliases {
		if strings.TrimSpace(alias) == "" {
			continue
		}
		attachedResourceDataFactories.items[NormalizeAttachedResourceType(alias)] = factory
	}
}

func ParseAttachedResourceData(data *AttachedResource) (AttachedResourceData, error) {
	if data == nil {
		return nil, utils.Error("attached resource is nil")
	}

	attachedResourceDataFactories.RLock()
	factory := attachedResourceDataFactories.items[data.NormalizedType()]
	attachedResourceDataFactories.RUnlock()
	if factory == nil {
		resource := NewDefaultAttachedResourceData(data.Type, data.Key)
		if err := resource.Unmarshal(data.Value); err != nil {
			return nil, err
		}
		return resource, nil
	}

	resource := factory()
	if resource == nil {
		return nil, utils.Errorf("attached resource factory returned nil for type: %s", strings.TrimSpace(data.Type))
	}
	if err := resource.Unmarshal(data.Value); err != nil {
		return nil, err
	}
	return resource, nil
}
