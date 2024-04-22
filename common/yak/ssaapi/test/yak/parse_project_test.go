package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func TestParseProject(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	err := vfs.AddDirByString("a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	err = vfs.AddFileToDir("a", "a.yak", `include "b/b.yak"; dump(b)`)
	if err != nil {
		t.Fatal(err)
	}
	vfs.AddFileToDir("b", "b.yak", `dump("in b.yak"); b = 3`)
	vfs.AddFileToDir("c", "c.yak", `include "b/b.yak"; dump(b + 1)`)

	prog, err := ssaapi.ParseProject(
		vfs,
		//ssaapi.WithFileSystemEntry("a/a.yak"),
		ssaapi.WithDatabaseProgramName("test"),
	)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	if len(prog.Ref("b")) <= 0 {
		t.Fatal("not found b")
	}
}
