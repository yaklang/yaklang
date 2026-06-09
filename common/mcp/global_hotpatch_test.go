package mcp

import "testing"

func TestGlobalHotPatchToolSetRegistered(t *testing.T) {
	set, ok := globalToolSets["global_hotpatch"]
	if !ok {
		t.Fatalf("global_hotpatch tool set not registered")
	}
	for _, name := range []string{
		"get_global_hotpatch_config",
		"enable_global_hotpatch",
		"disable_global_hotpatch",
		"reset_global_hotpatch_config",
		"query_hotpatch_template_list",
	} {
		if _, exists := set.Tools[name]; !exists {
			t.Fatalf("tool not registered: %s", name)
		}
	}
}
