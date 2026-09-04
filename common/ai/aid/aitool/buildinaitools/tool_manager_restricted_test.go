package buildinaitools

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestOnlyToolsPreventsDynamicLookupAndExtension(t *testing.T) {
	scoped := aitool.NewWithoutCallback("read_file", aitool.WithDescription("scoped"))
	host := aitool.NewWithoutCallback("read_file", aitool.WithDescription("host"))
	shell := aitool.NewWithoutCallback("bash")
	manager := NewToolManagerByToolGetter(func() []*aitool.Tool { return []*aitool.Tool{host, shell} },
		WithOnlyTools(scoped), WithEnableAllTools(), WithExtendTools([]*aitool.Tool{shell}, true), WithSearchToolEnabled(true), WithForgeSearchToolEnabled(true))
	tools, err := manager.GetEnableTools()
	if err != nil || len(tools) != 1 || tools[0] != scoped {
		t.Fatalf("restricted inventory changed: %v %v", tools, err)
	}
	actual, err := manager.GetToolByName("read_file")
	if err != nil || actual != scoped {
		t.Fatal("host tool replaced scoped callback")
	}
	for _, name := range []string{"bash", "get_file_content", "read_file_lines", "mcp_host_exec"} {
		if _, err := manager.GetToolByName(name); err == nil {
			t.Fatalf("guessed tool escaped authority: %s", name)
		}
	}
	if _, err := manager.SearchTools("keyword", "bash"); err == nil {
		t.Fatal("dynamic discovery expanded restricted tool set")
	}
	ordinary := NewToolManagerByToolGetter(func() []*aitool.Tool { return []*aitool.Tool{host, shell} }, WithEnableAllTools(), WithSearchToolEnabled(false), WithForgeSearchToolEnabled(false))
	if actual, err := ordinary.GetToolByName("bash"); err != nil || actual != shell {
		t.Fatalf("ordinary manager regressed: %v", err)
	}
}
