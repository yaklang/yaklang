package ssaapi

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func TestCompileWithDatabase_SmokingTest(t *testing.T) {
	progName := uuid.New().String()
	prog, err := ssaapi.Parse(`
dump("HJello")
a = i => i + 1
dump(a(3))
`, ssaapi.WithDatabaseProgramName(progName))
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	count := 0

	m := omap.NewGeneralOrderedMap()

	// test source code
	for result := range ssadb.YieldIrCodesProgramName(consts.GetGormProjectDatabase(), context.Background(), progName) {
		count++
		result.Show()
		if result.SourceCodeHash == "" {
			spew.Dump(result)
			t.Fatal("source code hash is empty")
		} else {
			t.Log("source code hash", result.SourceCodeHash)
		}
		m.Set(result.SourceCodeHash, struct{}{})
	}
	if m.Len() != 1 {
		t.Fatal("source code hash is not unique")
	}
	if count <= 0 {
		t.Fatal("no result in ir code database")
	}
}
