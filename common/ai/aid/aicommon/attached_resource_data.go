package aicommon

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// AttachedResourceData is one parsed, immutable attached resource. Collection
// operations such as parsing, deduplication and group binding live outside the
// instance so a resource never processes the list containing itself.
type AttachedResourceData interface {
	Type() string
	DedupKey() string
	BindLoopData(loop ReActLoopIF) error
	ToAttachData(loop ReActLoopIF) string
}

func attachedResourceDedupKey(typ string, parts ...string) string {
	normalizedType := NormalizeAttachedResourceType(typ)
	hash := sha256.New()
	_, _ = hash.Write([]byte(normalizedType))
	for _, part := range parts {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(part))
	}
	return fmt.Sprintf("%s:%x", normalizedType, hash.Sum(nil))
}

type AttachedResourceDataFactory func(resource *AttachedResource) (AttachedResourceData, error)

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

	if data.HasType(AttachedResourceTypeFile) && data.HasKey(CONTEXT_PROVIDER_KEY_FILE_CONTENT) {
		return newAttachedFileContentResourceData(data)
	}

	attachedResourceDataFactories.RLock()
	factory := attachedResourceDataFactories.items[data.NormalizedType()]
	attachedResourceDataFactories.RUnlock()
	if factory == nil {
		return newDefaultAttachedResourceData(data)
	}
	resource, err := factory(data)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, utils.Errorf("attached resource factory returned nil for type: %s", strings.TrimSpace(data.Type))
	}
	return resource, nil
}

// ParseAttachedResourceDataList filters placeholders, parses every resource,
// keeps only the first resource for each DedupKey and returns parse failures
// separately so callers can log them without losing valid resources.
func ParseAttachedResourceDataList(resources []*AttachedResource) ([]AttachedResourceData, []error) {
	parsed := make([]AttachedResourceData, 0, len(resources))
	seen := make(map[string]struct{})
	errs := make([]error, 0)
	for _, resource := range NonEmptyAttachedResources(resources) {
		data, err := ParseAttachedResourceData(resource)
		if err != nil {
			errs = append(errs, utils.Wrapf(err, "parse attached resource type=%q", resource.NormalizedType()))
			continue
		}
		key := data.DedupKey()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		parsed = append(parsed, data)
	}
	return parsed, errs
}
