package coordinator

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// PlanResponse is immutable plan input, not a legacy runtime task graph.
type PlanResponse struct {
	RootTask *PlanNode `json:"root_task"`
	Document string    `json:"document,omitempty"`
}

type planSource struct {
	build func(*Session) (string, string, error)
}

// WithPresetPlan bypasses model plan generation, but never bypasses DAG
// validation, approval or the coordinator/worker execution gates.
func WithPresetPlan(data, document string) aicommon.ConfigOption {
	return aicommon.WithAppendOtherOption(planSource{build: func(*Session) (string, string, error) {
		return data, document, nil
	}})
}

// WithPlanMocker supplies a native preset task tree. It is called once for a new
// plan; restored plans retain their current definition and do not call it again.
func WithPlanMocker(build func(*Session) *PlanResponse) aicommon.ConfigOption {
	return aicommon.WithAppendOtherOption(planSource{build: func(s *Session) (string, string, error) {
		if build == nil {
			return "", "", fmt.Errorf("plan mocker is nil")
		}
		p := build(s)
		if p == nil || p.RootTask == nil {
			return "", "", fmt.Errorf("plan mocker returned no root task")
		}
		data, err := json.Marshal(p.RootTask)
		document := p.Document
		if document == "" {
			document = PlanDocument(p.RootTask)
		}
		return string(data), document, err
	}})
}

func nativePlanOptions(cfg *aicommon.Config) []aicommon.ConfigOption {
	var opts []aicommon.ConfigOption
	for _, option := range cfg.OtherOption {
		if source, ok := option.(planSource); ok {
			opts = append(opts, aicommon.WithAppendOtherOption(source))
		}
	}
	return opts
}

func (s *Session) preparePresetPlan() error {
	if s.controller.Snapshot().Plan != nil {
		return nil
	}
	var source *planSource
	for _, option := range s.OtherOption {
		if p, ok := option.(planSource); ok {
			copy := p
			source = &copy
		}
	}
	if source == nil {
		return nil
	}
	data, document, err := source.build(s)
	if err != nil {
		return err
	}
	_, err = s.controller.CreatePlan(s.GetContext(), data, document)
	if err == nil {
		s.Timeline.PushText(s.AcquireId(), "[PRESET_PLAN]\n当前计划已加载；完善后提交审核，无需再次创建。")
	}
	return err
}
