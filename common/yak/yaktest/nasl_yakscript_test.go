package yaktest

import (
	"github.com/yaklang/yaklang/common/consts"
	"testing"
)

func init() {
	consts.GetGormProjectDatabase()
}
func TestUpdateScript(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试更新NaslScript",
			Src:  `nasl.UpdateDatabase("/Users/z3/nasl/nasl-plugins/2023/apache")`,
		},
	}
	Run("测试从本地文件更新NaslScript到数据库", t, cases...)
}

func TestScanTarget(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试扫描目标",
			Src: `
kbs,err = nasl.ScanTarget("198.73.2.155:5061",nasl.group("oracle"))
dump(kbs)
`,
		},
	}
	Run("测试扫描目标", t, cases...)
}
func TestQueryAll(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试查询NaslScript",
			Src: `
naslScripts = nasl.QueryAllScript()
dump(naslScripts.Length())
`,
		},
	}
	Run("测试查询NaslScript", t, cases...)
}
func TestQueryGroupNames(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试查询NaslScript",
			Src: `
groupNames = nasl.QueryAllGroupNames()
dump(groupNames)
`,
		},
	}
	Run("测试查询NaslScript", t, cases...)
}
func TestInitNaslDatabase(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试初始化NaslScript",
			Src: `

libraryPath = "/Users/z3/Downloads/nasllib"
err = nasl.UpdateDatabase(libraryPath)
if err{
	log.Error(err)
}
`,
		},
	}
	Run("测试初始化NaslScript到数据库", t, cases...)
}
