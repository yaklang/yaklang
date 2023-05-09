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
kbs = nasl.ScanTarget("182.54.177.31:3306",nasl.group("apache"))
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
oracleGroupScriptPath=` + "`" + `
/Users/z3/nasl/nasl-plugins/2013/oracle
/Users/z3/nasl/nasl-plugins/2014/oracle
/Users/z3/nasl/nasl-plugins/2022/oracle
/Users/z3/nasl/nasl-plugins/2023/oracle
/Users/z3/nasl/nasl-plugins/2015/oracle
/Users/z3/nasl/nasl-plugins/2017/oracle
/Users/z3/nasl/nasl-plugins/2019/oracle
/Users/z3/nasl/nasl-plugins/2021/oracle
/Users/z3/nasl/nasl-plugins/2020/oracle
/Users/z3/nasl/nasl-plugins/2018/oracle
/Users/z3/nasl/nasl-plugins/2016/oracle
` + "`" + `
apacheGroupScriptPath=` + "`" + `
/Users/z3/nasl/nasl-plugins/2013/apache
/Users/z3/nasl/nasl-plugins/2014/apache
/Users/z3/nasl/nasl-plugins/2022/apache
/Users/z3/nasl/nasl-plugins/2023/apache
/Users/z3/nasl/nasl-plugins/2012/apache
/Users/z3/nasl/nasl-plugins/2009/apache
/Users/z3/nasl/nasl-plugins/2017/apache
/Users/z3/nasl/nasl-plugins/2010/apache
/Users/z3/nasl/nasl-plugins/2019/apache
/Users/z3/nasl/nasl-plugins/2021/apache
/Users/z3/nasl/nasl-plugins/2020/apache
/Users/z3/nasl/nasl-plugins/2018/apache
/Users/z3/nasl/nasl-plugins/2011/apache
/Users/z3/nasl/nasl-plugins/2016/apache
` + "`" + `
nasl.RemoveDatabase()
oracleGroupScriptPath.Split("\n").Map(func(path) {
	err = nasl.UpdateDatabase(path,"oracle")
	if err{
		log.Error(err)
		return
	}
})
apacheGroupScriptPath.Split("\n").Map(func(path) {
	err = nasl.UpdateDatabase(path,"apache")
	if err{
		log.Error(err)
		return
	}
})
`,
		},
	}
	Run("测试初始化NaslScript到数据库", t, cases...)
}
