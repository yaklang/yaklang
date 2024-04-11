package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func Test_Multi_File(t *testing.T) {

	prog, err := ssaapi.Parse(`.\code\mutiFileDemo`,
		ssaapi.WithIsFilePath(true), ssaapi.WithLanguage("java"))
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()

}
