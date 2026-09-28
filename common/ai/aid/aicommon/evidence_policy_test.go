package aicommon

import (
	"context"
	"testing"
)

func TestReadOnlyEvidenceBlocksInvokeAfterToolFlagOverride(t *testing.T) {
	config := NewConfig(context.Background(), WithDisableAutoSkills(true), WithReadOnlyEvidence(), WithDisableToolUse(false))
	if config.DisableToolUse {
		t.Fatal("test must exercise an attempted option override")
	}
	caller := &ToolCaller{config: config}
	// Nil tool/emitter are deliberate: policy must reject before inspecting or
	// executing any capability, including one injected after initial setup.
	if _, err := caller.invoke(nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("immutable evidence policy was bypassed")
	}
	child := NewConfig(context.Background(), ConvertConfigToOptions(config)...)
	if !child.IsReadOnlyEvidence() {
		t.Fatal("child agent lost immutable evidence policy")
	}
}
