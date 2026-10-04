package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strings"
)

// ContextInput is a data-only compatibility container. It has no prompt builder,
// task lifecycle or memory operations. Runtime templates use ContextSnapshot.
type ContextInput struct {
	Query          string
	Timeline       *aicommon.Timeline
	PersistentData []string
}

func GetDefaultContextProvider() *ContextInput {
	return &ContextInput{Timeline: aicommon.NewTimeline(nil, nil)}
}
func (p *ContextInput) StoreQuery(query string) { p.Query = query }
func (p *ContextInput) Snapshot() *ContextSnapshot {
	v := &ContextSnapshot{Query: p.Query, CurrentTask: &TaskSnapshot{}, persistent: strings.Join(p.PersistentData, "\n")}
	if p.Timeline != nil {
		v.timeline = p.Timeline.Dump()
		v.frozen, v.open = p.Timeline.DumpFrozenOpen()
	}
	return v
}
func (p *ContextInput) PushPersistentData(values ...string) {
	p.PersistentData = append(p.PersistentData, values...)
}
func WithPromptContextProvider(p *ContextInput) aicommon.ConfigOption {
	return func(cfg *aicommon.Config) error {
		if p == nil {
			return nil
		}
		if p.Timeline != nil {
			if err := aicommon.WithTimeline(p.Timeline)(cfg); err != nil {
				return err
			}
		}
		return aicommon.WithAppendPersistentContext(p.PersistentData...)(cfg)
	}
}
