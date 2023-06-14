package antlr4nasl

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func FilterRootScriptsWithDbModelType(scripts []*yakit.NaslScript) []*yakit.NaslScript {
	newScripts := []*yakit.NaslScript{}
	tmp := map[string]struct{}{}
	for _, script := range scripts {
		var dep []string
		err := json.Unmarshal([]byte(script.Dependencies), &dep)
		if err != nil {
			continue
		}
		for _, d := range dep {
			tmp[d] = struct{}{}
		}
	}
	for _, script := range scripts {
		if _, ok := tmp[script.OriginFileName]; !ok {
			newScripts = append(newScripts, script)
		}
	}
	return newScripts
}
func FilterRootScripts(scripts []*NaslScriptInfo) []*NaslScriptInfo {
	//忽略了循环依赖
	rootScripts := []*NaslScriptInfo{}
	tmp := map[string]struct{}{}
	for _, info := range scripts {
		for _, dependency := range info.Dependencies {
			tmp[dependency] = struct{}{}
		}
	}
	for _, info := range scripts {
		if _, ok := tmp[info.OriginFileName]; !ok {
			rootScripts = append(rootScripts, info)
		}
	}
	return rootScripts
}
