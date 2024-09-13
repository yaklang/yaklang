package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"os"
	"regexp"
)

//go:embed data.json
var data []byte

//go:embed white_list.txt
var whiteList string

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

	whiteName := utils.PrettifyListFromStringSplitEx(whiteList, "\n", "\r")

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
		if utils.StringArrayContains(whiteName, info.ScriptName) {
			fmt.Println("skip white list script:", info.ScriptName)
			continue
		}
		script, err := yakit.GetYakScriptIdOrName(db, 0, info.ScriptName)
		if err != nil {
			log.Errorf("get script: %s fail:%v", info.ScriptName, err)
			continue
		}
		fmt.Println(reTool.MatchString(script.Content))
		script.Content = reTool.ReplaceAllStringFunc(script.Content, func(s string) string {
			update := s + build(info)
			fmt.Printf("update script:%s from: [%s] to [%s]\n", script.ScriptName, s, update)
			return update
		})
		err = yakit.CreateOrUpdateYakScriptByName(db, script.ScriptName, script)
		if err != nil {
			log.Errorf("update script: %s fail:%v", info.ScriptName, err)
			continue
		}
		fmt.Println("update script:", info.ScriptName, "success")
	}
}
