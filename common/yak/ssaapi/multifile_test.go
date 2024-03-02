package ssaapi

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"strconv"
	"testing"
)

func TestMultiFile(t *testing.T) {
	outterFile := consts.TempFileFast(`a = () => {
	return "abc"
}`)
	outterFile = strconv.Quote(outterFile)
	prog, err := Parse(`
include ` + outterFile + `

result = a()
dump(result)
`)
	if err != nil {
		t.Fatal(err)
	}
	result := prog.Show().Ref("result").GetTopDefs().Get(0)
	spew.Dump(result.String())

	if result.GetConstValue() != "abc" {
		t.Fatal("result is not abc")
	}
}
