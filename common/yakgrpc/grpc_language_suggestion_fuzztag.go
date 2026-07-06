package yakgrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)
var _fuzztagSuggestions []*ypb.SuggestionDescription
var _fuzztagDescMap = make(map[string]string)
var _fuzztagSuggestionsOnce sync.Once
var tagDescFormatString = "**%s**\n\n%s\n\n**Example:**\n\n```http\n%s\n```"

func getFuzztagSuggestion(tagName string, labelFormatString string, tagDesc *mutate.FuzzTagDescription) (string, *ypb.SuggestionDescription) {
	tagLabel := fmt.Sprintf(labelFormatString, tagName, tagDesc.TagNameVerbose)
	return tagLabel, &ypb.SuggestionDescription{
		Label:       tagLabel,
		Description: tagDesc.Description,
		InsertText:  fmt.Sprintf(`%s($1)}}`, tagName),
		Kind:        CompletionKindFunction,
	}
}

func _getAllFuzztagSuggestionInfo() ([]*ypb.SuggestionDescription, map[string]string) {
	_fuzztagSuggestionsOnce.Do(func() {
		allTag := append(mutate.GetAllFuzztags(), append(mutate.FileTag(), mutate.CodecTag()...)...)
		allTag = append(allTag, mutate.HotPatchFuzztag(func(s string, f func(string)) error { return nil }), mutate.HotPatchDynFuzztag(func(s string, f func(string)) error { return nil }))
		allTag = append(allTag, &mutate.FuzzTagDescription{TagName: "request", Description: "原始请求", TagNameVerbose: "request", Examples: []string{"{{request}}"}})
		tagLabelFormatString := fmt.Sprintf("%%-%ds[%%s]", mutate.GetFuzztagMaxLength(allTag)+4)
		for _, tag := range allTag {
			// tag name suggestion
			tagLabel, tagSuggestion := getFuzztagSuggestion(tag.TagName, tagLabelFormatString, tag)
			_fuzztagSuggestions = append(_fuzztagSuggestions, tagSuggestion)
			_fuzztagDescMap[tag.TagName] = fmt.Sprintf(tagDescFormatString, tagLabel, tag.Description, strings.Join(tag.Examples, "\n"))

			for _, alias := range tag.Alias { // alias suggesion
				aliasLabel, aliasTagSuggestion := getFuzztagSuggestion(alias, tagLabelFormatString, tag)
				_fuzztagSuggestions = append(_fuzztagSuggestions, aliasTagSuggestion)
				_fuzztagDescMap[alias] = fmt.Sprintf(tagDescFormatString, aliasLabel, tag.Description, strings.Join(lo.Map(tag.Examples, func(item string, index int) string {
					return strings.Replace(item, tag.TagName, alias, 1)
				}), "\n"))
			}
		}
	})
	return _fuzztagSuggestions, _fuzztagDescMap
}

func getCodecPluginList() []*ypb.SuggestionDescription {
	var ret []*ypb.SuggestionDescription
	for _, codecScript := range yakit.QueryYakScriptByType(consts.GetGormProfileDatabase(), "codec") {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       codecScript.ScriptName,
			Description: codecScript.Help,
			InsertText:  codecScript.ScriptName,
		})
	}
	return ret
}

func getPayloadGroup() []*ypb.SuggestionDescription {
	var ret []*ypb.SuggestionDescription
	allPayloadGroup, _ := yakit.GetAllPayloadGroupName(consts.GetGormProfileDatabase())
	for _, groupName := range allPayloadGroup {
		ret = append(ret, &ypb.SuggestionDescription{
			Label:       groupName,
			Description: "",
			InsertText:  groupName,
		})
	}
	return ret
}

func FuzztagServer(req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, bool) {
	if req.GetYakScriptType() != "fuzztag" {
		return nil, false
	}
	ret := &ypb.YaklangLanguageSuggestionResponse{}
	switch req.GetInspectType() {
	case COMPLETION:
		// ret
		ret.SuggestionMessage = append(ret.SuggestionMessage, fuzztagCompletion(req.GetRange().GetCode(), "")...)
	case HOVER:
		// ret
		ret.SuggestionMessage = append(ret.SuggestionMessage, fuzztagHover(req.GetRange().Code, "")...)
	}
	return ret, true
}

func fuzztagHover(fuzztagCode string, hotPatchCode string) []*ypb.SuggestionDescription {
	var suggestions []*ypb.SuggestionDescription
	_, descMap := _getAllFuzztagSuggestionInfo()
	desc, ok := descMap[fuzztagCode]
	if ok {
		suggestions = append(suggestions, &ypb.SuggestionDescription{
			Label: desc,
		})
	}
	return suggestions
}

var hotPatchBlacklist = []string{"afterRequest", "beforeRequest", "mirrorHTTPFlow"}

func fuzztagCompletion(fuzztagCode string, hotPatchCode string) []*ypb.SuggestionDescription {
	var suggestions []*ypb.SuggestionDescription
	var hotPatchSuggestions []*ypb.SuggestionDescription

	if hotPatchCode != "" {
		prog, err := static_analyzer.SSAParse(hotPatchCode, string(plugin_type.PluginTypeYak), ssaapi.WithIgnoreSyntaxError(true))
		if err == nil {
			mainFunc, ok := prog.Program.Funcs.Get(string(ssa.MainFunctionName))
			if ok {
				for _, childFunc := range mainFunc.ChildFuncs {
					childFunc, ok := mainFunc.GetValueById(childFunc)
					if !ok || childFunc == nil {
						continue
					}
					if utils.StringArrayContains(hotPatchBlacklist, childFunc.GetName()) {
						continue
					}
					funcTyp, _ := ssa.ToFunctionType(childFunc.GetType())
					hotPatchSuggestions = append(hotPatchSuggestions, &ypb.SuggestionDescription{
						Label:       childFunc.GetName(),
						InsertText:  childFunc.GetName(),
						Kind:        CompletionKindFunction,
						Description: funcTyp.String(),
					})
				}
			}
		}
	}

	if strings.HasPrefix(fuzztagCode, "{{") {
		if strings.HasPrefix(fuzztagCode, "{{codec(") {
			return getCodecPluginList()
		}
		if strings.HasPrefix(fuzztagCode, "{{payload(") {
			return getPayloadGroup()
		}
		if strings.HasPrefix(fuzztagCode, "{{yak(") || strings.HasPrefix(fuzztagCode, "{{yak:dyn(") {
			return hotPatchSuggestions
		}
		suggestions, _ = _getAllFuzztagSuggestionInfo()
	}

	return suggestions
}

func (s *Server) FuzzTagSuggestion(ctx context.Context, req *ypb.FuzzTagSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, error) {
	ret := &ypb.YaklangLanguageSuggestionResponse{}
	if req.GetInspectType() == HOVER {
		ret.SuggestionMessage = append(ret.SuggestionMessage, fuzztagHover(req.GetFuzztagCode(), "")...)
	} else if req.GetInspectType() == COMPLETION {
		ret.SuggestionMessage = append(ret.SuggestionMessage, fuzztagCompletion(req.GetFuzztagCode(), req.GetHotPatchCode())...)
	}
	return applyExampleFenceToResponse(ret), nil
}