package scanner

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
)

//go:embed testdata/large.js
var largeJS string

//go:embed testdata/popper.js
var packedJS string

func TestScanFile(t *testing.T) {
	scan := NewScanner()
	scan.SetScriptKind(core.ScriptKindJS)
	scan.SetText(largeJS)
	require.NotNil(t, scan.Scan())
	scan.SetText(packedJS)
	require.NotNil(t, scan.Scan())
}
