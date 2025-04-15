package tests

import (
	_ "embed"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed code/DynamicSecurityMetadataSource.java
var DynamicSecurityMetadataSource string

func TestRealJava_PanicInMemberCall(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("DynamicSecurityMetadataSource.java", DynamicSecurityMetadataSource)
	ssatest.CheckWithFS(vf, t, func(prog ssaapi.Programs) error {
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestRealJava_Ref(t *testing.T) {
	t.Run("__ref__ search", func(t *testing.T) {
		fs := filesys.NewRelLocalFs("C:\\Users\\10982\\IdeaProjects\\JavaSecLab\\src\\main\\java\\top\\whgojp\\common\\modules")
		ssatest.CheckWithFS(fs, t, func(programs ssaapi.Programs) error {
			vals, err := programs.SyntaxFlowWithError(`GetMapping.__ref__ as $a`)
			if err != nil {
				return err
			}
			a := vals.GetValues("a")
			a.Show()
			return nil
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
}
