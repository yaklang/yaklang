package tests

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestSCAPackageJSONSnapshot(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("package.json", `{"name":"sca-app","version":"1.0.0","dependencies":{"express":"4.18.2"}}`)
	vfs.AddFile("main.ts", `console.log("snapshot");`)
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		r, err := programs.SyntaxFlowWithError("__dependency__.express.version as $version")
		if err != nil {
			return err
		}
		if got := r.GetValues("version"); len(got) != 1 {
			t.Fatalf("expected one dependency version, got %v", got)
		}
		return nil
	}, ssaapi.WithLanguage("ts"))
}
