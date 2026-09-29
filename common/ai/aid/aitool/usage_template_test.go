package aitool

import (
	"strings"
	"testing"
)

func TestRenderUsageForModeSelectsOnlyTheCurrentProtocol(t *testing.T) {
	usage := `Common {{PATH}} and [[ -f "$file" ]]
[[- if .FunctionCallMode -]]JSON command[[- else -]]AITAG command[[- end -]]`
	for _, tc := range []struct {
		mode    bool
		want    string
		missing string
	}{
		{mode: true, want: "JSON command", missing: "AITAG command"},
		{mode: false, want: "AITAG command", missing: "JSON command"},
	} {
		got, err := RenderUsageForMode(usage, tc.mode)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "{{PATH}}") || !strings.Contains(got, `[[ -f "$file" ]]`) || !strings.Contains(got, tc.want) || strings.Contains(got, tc.missing) {
			t.Fatalf("mode=%t rendered Usage incorrectly: %q", tc.mode, got)
		}
	}
}

func TestRenderUsageForModeRejectsOtherVariablesAndMalformedBranches(t *testing.T) {
	for _, usage := range []string{
		"[[- .UserInput -]]",
		"[[- if .OtherMode -]]other[[- end -]]",
		"[[- if .FunctionCallMode -]]missing end",
		"[[- else -]]unexpected",
	} {
		if _, err := RenderUsageForMode(usage, true); err == nil {
			t.Fatalf("expected invalid Usage template to fail: %q", usage)
		}
	}
}
