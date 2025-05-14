package yak

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"strings"
)

func makeArgs(ctx context.Context, execParams []*ypb.ExecParamItem) []string {
	args := []string{"yak"}
	canFilter := true
	for _, p := range execParams {
		switch p.Key {
		case "__yakit_plugin_names__": // 直接查询插件名
			tempName, err := utils.SaveTempFile(p.Value, "yakit-plugin-selector-*.txt")
			if err != nil {
				log.Errorf("save temp file failed: %v", err)
				return nil
			}
			args = append(args, "--yakit-plugin-file", tempName)
			canFilter = false
			go func() {
				select {
				case <-ctx.Done():
					os.Remove(tempName)
				}
			}()

		case "__yakit_plugin_filter__": // 筛选情况
			if !canFilter {
				continue
			}
			var pluginFilter *ypb.QueryYakScriptRequest
			var pluginName []string
			err := json.Unmarshal([]byte(p.Value), pluginFilter)
			if err != nil {
				log.Errorf("unmarshal plugin filter failed: %v", err)
				continue
			}
			yakit.FilterYakScript(consts.GetGormProfileDatabase(), pluginFilter).Pluck("script_name", pluginName)
			tempName, err := utils.SaveTempFile(strings.Join(pluginName, "|"), "yakit-plugin-selector-*.txt")
			if err != nil {
				log.Errorf("save temp file failed: %v", err)
				continue
			}
			args = append(args, "--yakit-plugin-file", tempName)
		default:
			args = append(args, "--"+p.Key, p.Value)
		}
	}

	return args
}

func BindAIConfigToEngine(nIns *antlr4yak.Engine, ag *Agent) {
	nIns.GetVM().RegisterMapMemberCallHandler("aiagent", "ExecuteForge", func(i interface{}) interface{} {
		ofunc, ok := i.(func(forgeName string, i any, opts ...Option) (any, error))
		if ok {
			return func(forgeName string, i any, opts ...Option) (any, error) {
				opts = append(opts, ag.SubOption()...)
				return ofunc(forgeName, i, opts...)
			}
		}
		return i
	})

	nIns.GetVM().RegisterMapMemberCallHandler("aiagent", "CreateForge", func(i interface{}) interface{} {
		originFunc, ok := i.(func(name string, opts ...aiforge.Option) *aiforge.ForgeBlueprint)
		if ok {
			return func(name string, opts ...aiforge.Option) *aiforge.ForgeBlueprint {
				opts = append(opts, aiforge.WithAIDOptions(ag.AIDOptions()...))
				return originFunc(name, opts...)
			}
		}
		return i
	})
}
