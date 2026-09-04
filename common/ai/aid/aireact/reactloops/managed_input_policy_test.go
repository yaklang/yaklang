package reactloops

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/omap"
)

func TestManagedInputActionsExcludeDynamicEscapesAndKeepSignedActions(t *testing.T) {
	loop := &ReActLoop{actions: omap.NewEmptyOrderedMap[string, *LoopAction](), loopActions: omap.NewEmptyOrderedMap[string, LoopActionFactory]()}
	names := []string{"load_capability", "require_ai_blueprint", "loading_skills", "query_mcp_tools", "dispatch_sub_react_agents", "require_tool", "directly_call_tool"}
	for _, name := range names {
		previous, exists := GetLoopAction(name)
		name := name
		t.Cleanup(func() {
			if exists {
				actions.Set(name, previous)
			} else {
				actions.Delete(name)
			}
		})
		action := &LoopAction{ActionType: name}
		RegisterAction(action)
		loop.actions.Set(name, action)
	}
	loop.actions.Set("analyze_documents", &LoopAction{ActionType: "analyze_documents"})
	restrictManagedInputActions(loop)
	for _, name := range names[:5] {
		if _, ok := loop.actions.Get(name); ok {
			t.Fatalf("escape action survived: %s", name)
		}
	}
	for _, name := range []string{"require_tool", "directly_call_tool", "analyze_documents"} {
		if _, ok := loop.actions.Get(name); !ok {
			t.Fatalf("bounded/signed action removed: %s", name)
		}
	}
}
