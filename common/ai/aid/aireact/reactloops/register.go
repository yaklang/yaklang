package reactloops

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

var loops = new(sync.Map)
var loopMetadata = new(sync.Map) // stores *LoopMetadata by loop name
var actions = new(sync.Map)

// LoopMetadata stores metadata about a loop for AI understanding
type LoopMetadata struct {
	Name                string // loop name
	Description         string // describes what this loop does
	DescriptionZh       string // describes what this loop does in Chinese
	OutputExamplePrompt string // example output for reflection_output_example.txt
	UsagePrompt         string // usage description for x-@action-rules in schema
	IsHidden            bool   // whether to hide this loop from the user
	VerboseName         string // display name in English for the frontend
	VerboseNameZh       string // display name in Chinese for the frontend
}

// LoopMetadataOption configures LoopMetadata
type LoopMetadataOption func(*LoopMetadata)

// WithLoopIsHidden sets whether to hide this loop from the user
func WithLoopIsHidden(hidden bool) LoopMetadataOption {
	return func(m *LoopMetadata) {
		m.IsHidden = hidden
	}
}

// WithLoopDescription sets the description of what this loop does
func WithLoopDescription(desc string) LoopMetadataOption {
	return func(m *LoopMetadata) {
		m.Description = desc
	}
}

// WithLoopDescriptionZh sets the Chinese description of what this loop does
func WithLoopDescriptionZh(desc string) LoopMetadataOption {
	return func(m *LoopMetadata) {
		m.DescriptionZh = desc
	}
}

// WithLoopOutputExample sets the example output prompt for reflection
func WithLoopOutputExample(example string) LoopMetadataOption {
	return func(m *LoopMetadata) {
		m.OutputExamplePrompt = example
	}
}

// WithLoopUsagePrompt sets the usage description for schema
func WithLoopUsagePrompt(usage string) LoopMetadataOption {
	return func(m *LoopMetadata) {
		m.UsagePrompt = usage
	}
}

// WithVerboseName sets the English display name for the frontend
func WithVerboseName(name string) LoopMetadataOption {
	return func(m *LoopMetadata) {
		m.VerboseName = name
	}
}

// WithVerboseNameZh sets the Chinese display name for the frontend
func WithVerboseNameZh(name string) LoopMetadataOption {
	return func(m *LoopMetadata) {
		m.VerboseNameZh = name
	}
}

func RegisterAction(action *LoopAction) {
	actions.Store(action.ActionType, action)
}

func GetLoopAction(name string) (*LoopAction, bool) {
	action, ok := actions.Load(name)
	if !ok {
		return nil, false
	}
	actionObj, ok := action.(*LoopAction)
	if !ok {
		return nil, false
	}
	return actionObj, true
}

type LoopFactory func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error)

func RegisterLoopFactory(
	name string,
	creator LoopFactory,
	opts ...LoopMetadataOption,
) error {
	_, ok := loops.Load(name)
	if ok {
		return utils.Errorf("reactloop[%v] already exists", name)
	}
	loops.Store(name, creator)

	// Store metadata if provided
	if len(opts) > 0 {
		meta := &LoopMetadata{Name: name}
		for _, opt := range opts {
			opt(meta)
		}
		loopMetadata.Store(name, meta)
	}

	return nil
}

func CreateLoopByName(name string, invoker aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
	factory, ok := loops.Load(name)
	if !ok {
		return nil, utils.Errorf("reactloop[%v] not found", name)
	}
	factoryCreator, ok := factory.(LoopFactory)
	if !ok {
		return nil, utils.Errorf("reactloop[%v] type assert error", name)
	}
	loopIns, err := factoryCreator(invoker, opts...)
	if err != nil {
		return nil, utils.Wrap(err, "failed to create loop instance")
	}
	if loopIns.onLoopInstanceCreated != nil {
		loopIns.onLoopInstanceCreated(loopIns)
	}
	return loopIns, nil
}

func GetLoopFactory(name string) (LoopFactory, bool) {
	factory, ok := loops.Load(name)
	if !ok {
		return nil, false
	}
	factoryCreator, ok := factory.(LoopFactory)
	if !ok {
		return nil, false
	}
	return factoryCreator, true
}

// GetLoopMetadata retrieves metadata for a registered loop
func GetLoopMetadata(name string) (*LoopMetadata, bool) {
	meta, ok := loopMetadata.Load(name)
	if !ok {
		return nil, false
	}
	metaObj, ok := meta.(*LoopMetadata)
	if !ok {
		return nil, false
	}
	return metaObj, true
}

func (m *LoopMetadata) GetDescriptionZh() string {
	if m == nil {
		return ""
	}
	return m.DescriptionZh
}

// GetAllLoopMetadata returns all registered loop metadata
func GetAllLoopMetadata() []*LoopMetadata {
	var result []*LoopMetadata
	loopMetadata.Range(func(key, value interface{}) bool {
		if meta, ok := value.(*LoopMetadata); ok {
			result = append(result, meta)
		}
		return true
	})
	return result
}
