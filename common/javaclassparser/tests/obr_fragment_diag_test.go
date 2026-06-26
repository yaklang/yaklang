package tests

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

func TestOBRFragmentParseSlotReuseRegression(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/obr_fragment_parse_slot_reuse.class")
	if err != nil {
		t.Fatalf("read embedded class failed: %v", err)
	}

	source, derr := javaclassparser.Decompile(raw)
	if derr != nil {
		t.Fatalf("decompile returned error: %v", derr)
	}
	if strings.Contains(source, javaclassparser.DecompileStubMarker) {
		t.Fatalf("expected full OBRFragment decompilation, got stub\n----- source -----\n%s", source)
	}
	mustContain := []string{
		"getExportPackage().entrySet().iterator()",
		"getImportPackage().entrySet().iterator()",
		"getRequireBundle().entrySet().iterator()",
		"MAP.$(\"version\"",
	}
	for _, want := range mustContain {
		if !strings.Contains(source, want) {
			t.Fatalf("expected output to contain %q\n----- source -----\n%s", want, source)
		}
	}
}
