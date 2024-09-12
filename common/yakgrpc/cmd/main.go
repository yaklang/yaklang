package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"os"
	"regexp"
)

//go:embed data.json
var data []byte

type YakScriptRiskInfo struct {
	ScriptName  string `json:"插件名称"`
	ScriptType  string `json:"插件类型"`
	Description string `json:"漏洞描述"`
	Solution    string `json:"修复建议"`
	Sync        string `json:"备注插件商店是否同步"`
}

func main() {
	var err error
	db := consts.GetGormProfileDatabase()
	if len(os.Args) >= 2 {
		path := os.Args[1]
		db, err = consts.CreateProfileDatabase(path)
		if err != nil {
			panic(err)
		}
	}
	reTool, err := regexp.Compile(`(?s)risk\.NewRisk\(.*?\),`)
	if err != nil {
		panic(err)
	}

	var dataInfo []*YakScriptRiskInfo
	err = json.Unmarshal(data, &dataInfo)
	if err != nil {
		log.Errorf("unmarshal data failed: %v", err)
		return
	}
	build := func(info *YakScriptRiskInfo) string {
		return fmt.Sprintf("risk.description(`%s`),risk.solution(`%s`),", info.Description, info.Solution)
	}

	for _, info := range dataInfo {
		if info.Sync != "已同步" {
			continue
		}
		script, err := yakit.GetYakScriptIdOrName(db, 0, info.ScriptName)
		if err != nil {
			log.Errorf("get script: %s fail:%v", info.ScriptName, err)
			continue
		}
		fmt.Println(reTool.MatchString(script.Content))
		script.Content = reTool.ReplaceAllStringFunc(script.Content, func(s string) string {
			return s + build(info)
		})
		err = yakit.CreateOrUpdateYakScriptByName(db, script.ScriptName, script)
		if err != nil {
			log.Errorf("update script: %s fail:%v", info.ScriptName, err)
			continue
		}
		fmt.Println("update script:", info.ScriptName, "success")
	}
}
