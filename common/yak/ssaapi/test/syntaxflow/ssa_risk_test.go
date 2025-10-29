package syntaxflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSSARisk_Normal(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("test.yak", `
a = source() 
b = source()
f = (arg) => {
	if c > 0 {
		arg = 2
	}
	sink(arg)
}
f(a)
	`)

	rule := `
source() as $source 
sink(* as $sink)
$sink #{
hook:<<<CODE
	* & $source as $dangerous
CODE
}->  
alert $dangerous for {
	desc: "this is an alert message"
	Title:"this is a title"
	level:"low"
}
	`
	ssatest.CheckProfileWithFS(vf, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
		prog.Show()
		result, err := prog.SyntaxFlowWithError(
			rule,
			ssaapi.QueryWithEnableDebug(true),
		)
		require.NoError(t, err)
		result.Show()

		dangerous := result.GetValues("dangerous")
		require.Len(t, dangerous, 1)
		log.Infof("data: %v", dangerous[0])

		// check risk database when database compile
		if p == ssatest.OnlyMemory {
			return nil
		}
		resultId, err := result.Save(schema.SFResultKindDebug)
		defer yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{result.GetProgramName()},
		})
		_ = resultId
		require.NoError(t, err)
		_, datas, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{result.GetProgramName()}}, nil)
		require.NoError(t, err)
		require.Len(t, datas, 1)
		data := datas[0]
		log.Infof("data: %v", data)
		require.NotEqual(t, data.CodeFragment, "")
		require.NotEqual(t, data.CodeSourceUrl, "")
		require.NotEqual(t, data.FunctionName, "")
		require.NotEqual(t, data.Line, 0)

		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}
