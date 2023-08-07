package yaktest

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"testing"
)

func init() {
	consts.GetGormProjectDatabase()
	yaklang.Import("nasl", antlr4nasl.Exports)
}
func TestDeleteScript(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试从数据库删除Script",
			Src:  `nasl.RemoveDatabase()`,
		},
	}
	Run("测试从数据库删除Script", t, cases...)
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
naslScriptName = "gb_apache_tomcat_consolidation.nasl"
proxy = ""
naslScanHandle = (target)=>{
    opts = [nasl.plugin(naslScriptName)]
    if proxy != nil && proxy != ""{
        opts.Append(nasl.proxy(proxy))
    }
	opts.Append(nasl.riskHandle((risk)=>{
		log.info("found risk: %v", risk)
	}))
    kbs ,err = nasl.ScanTarget(target,opts...)
    if err{
        log.error("%v", err)
    }
}

naslScanHandle("183.234.44.226:8099")
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

func TestInitNaslDatabase(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试初始化NaslScript",
			Src: `

libraryPath = "/Users/z3/nasl/nasl-plugins"
err = nasl.UpdateDatabase(libraryPath)
if err{
	log.Error(err)
}
`,
		},
	}
	Run("测试初始化NaslScript到数据库", t, cases...)
}
func TestCommonScan(t *testing.T) {
	scanCode := `
proxy = ""
naslScanHandle = (target)=>{
    opts = [nasl.family("")]
    if proxy != nil && proxy != ""{
        opts.Append(nasl.proxy(proxy))
    }
	opts.Append(nasl.riskHandle((risk)=>{
		log.info("found risk: %v", risk)
	}))
	//opts.Append(nasl.conditions({
	//	"family": "Web Servers",
	//	"category": "ACT_GATHER_INFO",
	//}))
	opts.Append(nasl.plugin("mssqlserver_detect.nasl"))
    kbs ,err = nasl.ScanTarget(target,opts...)
    if err{
        log.error("%v", err)
    }
}

naslScanHandle("136.233.183.242:1433")
`
	err := yaklang.New().SafeEval(context.Background(), scanCode)
	if err != nil {
		t.Fatal(err)
	}
}
