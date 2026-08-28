package main

import (
	"testing"
)

func TestClassifyPackage(t *testing.T) {
	tests := []struct {
		sym  string
		want string
	}{
		{"runtime.main", "runtime"},
		{"github.com/yaklang/yaklang/common/utils/lowhttp/poc.Get", "github.com/yaklang/yaklang/common/utils/lowhttp/poc"},
		{"fmt.Sprintf", "fmt"},
		{"strings.(*Builder).Grow", "strings"},
		{"github.com/yaklang/yaklang/common/yak/ssaapi.(*Program).Parse", "github.com/yaklang/yaklang/common/yak/ssaapi"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.sym, func(t *testing.T) {
			got := classifyPackage(tt.sym)
			if got != tt.want {
				t.Errorf("classifyPackage(%q) = %q, want %q", tt.sym, got, tt.want)
			}
		})
	}
}

func TestMatchModule(t *testing.T) {
	modulePkgs := map[string][]string{
		"poc": {"github.com/yaklang/yaklang/common/utils/lowhttp/poc"},
		"ssa": {"github.com/yaklang/yaklang/common/yak/ssaapi"},
	}
	tests := []struct {
		pkg  string
		want string
	}{
		{"github.com/yaklang/yaklang/common/utils/lowhttp/poc", "poc"},
		{"github.com/yaklang/yaklang/common/utils/lowhttp/poc/sub", "poc"},
		{"github.com/yaklang/yaklang/common/yak/ssaapi", "ssa"},
		{"runtime", ""},
		{"fmt", ""},
	}
	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			got := matchModule(tt.pkg, modulePkgs)
			if got != tt.want {
				t.Errorf("matchModule(%q) = %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

func TestBuildModulePackageMap(t *testing.T) {
	modules := []string{"poc", "ssa", "unknown_module"}
	m := buildModulePackageMap(modules)
	if _, ok := m["poc"]; !ok {
		t.Error("expected poc in map")
	}
	if _, ok := m["ssa"]; !ok {
		t.Error("expected ssa in map")
	}
	if _, ok := m["unknown_module"]; ok {
		t.Error("expected unknown_module to be absent from map")
	}
}
