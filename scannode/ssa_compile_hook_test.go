package scannode

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
)

func TestProductScanCompilerIsRegistered(t *testing.T) {
	if syntaxflow_scan.CompileProject == nil {
		t.Fatal("scannode must import ssa_compile so ScanProject can compile")
	}
}
