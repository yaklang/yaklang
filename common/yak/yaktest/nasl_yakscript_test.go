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

libraryPath = "/Users/z3/nasl/nasl-plugins/gb_netapp_data_ontap_consolidation.nasl"
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
naslScanHandle = (hosts,ports)=>{
    opts = [nasl.family("")]
    if proxy != nil && proxy != ""{
        opts.Append(nasl.proxy(proxy))
    }
	opts.Append(nasl.preference({
		"Exclude printers from scan": false,
		//"Enable CGI scanning": false,
		"global_settings/debug_level": 1,
	}))
	opts.Append(nasl.riskHandle((risk)=>{
		log.info("found risk: %v", risk)
	}))
	//opts.Append(nasl.conditions({
	//	"family": "Web Servers",
	//}))
	opts.Append(nasl.plugin("gb_apache_tomcat_consolidation.nasl"))
    kbs ,err = nasl.Scan(hosts,ports,opts...)
    if err{
        log.error("%v", err)
    }
}

naslScanHandle("uat.sdeweb.hkcsl.com","443")
`
	err := yaklang.New().SafeEval(context.Background(), scanCode)
	if err != nil {
		t.Fatal(err)
	}
}
func TestScanByMixCaller(t *testing.T) {
	scanCode := `
res = servicescan.Scan("175.111.120.131","U:161")~
manager = hook.NewMixPluginCaller()~
manager.LoadPlugin("__NaslScript__mssqlserver_detect.nasl")
for i in res{
	manager.HandleServiceScanResult(i)
}
manager.Wait()
`
	err := yaklang.New().SafeEval(context.Background(), scanCode)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetAll(t *testing.T) {
	scanCode := `
all = nasl.QueryAllScripts({"category":"ACT_SETTINGS"})~
for i in all{
	println(i.OriginFileName)
}
`
	err := yaklang.New().SafeEval(context.Background(), scanCode)
	if err != nil {
		t.Fatal(err)
	}
}
