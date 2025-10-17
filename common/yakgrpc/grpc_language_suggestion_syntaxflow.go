package yakgrpc

import (
	"context"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// func func LanguageServerAnalyzeProgram(req *ypb.YaklangLanguageSuggestionRequest) (*LanguageServerAnalyzerResult, error) {

func SyntaxFlowServer(req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, bool) {
	if req.YakScriptType != "syntaxflow" {
		return nil, false
	}
	ret := &ypb.YaklangLanguageSuggestionResponse{}
	switch req.GetInspectType() {
	case COMPLETION:
		// ret
		ret.SuggestionMessage = append(ret.SuggestionMessage, syntaxflowCompletion(req.GetRange())...)
	}

	return ret, true
}

func syntaxflowCompletion(rng *ypb.Range) []*ypb.SuggestionDescription {
	var suggestions []*ypb.SuggestionDescription
	if rng == nil {
		return suggestions
	}
	if rng.Code == "<" {
		suggestions = getNativeCallSuggestion()
	}

	if rng.Code == "<include(" {
		suggestions = getLibrarySuggestion()
	}

	return suggestions
}

var (
	native_call_suggestion_lock = sync.Mutex{}
	native_call_suggestion      = []*ypb.SuggestionDescription{}
	library_suggestion_lock     = sync.Mutex{}
	library_suggestion          = []*ypb.SuggestionDescription{}
)

func getNativeCallSuggestion() []*ypb.SuggestionDescription {
	native_call_suggestion_lock.Lock()
	defer native_call_suggestion_lock.Unlock()
	if len(native_call_suggestion) == 0 {
		// items := make([]*ypb.SuggestionDescription, 0, len(ssaapi.NativeCallDocuments))
		for _, call := range ssaapi.NativeCallDocuments {
			item := &ypb.SuggestionDescription{
				Label:             call.Name,
				Description:       call.Description,
				InsertText:        fmt.Sprintf("%s(${1})>", call.Name),
				JustAppend:        false,
				DefinitionVerbose: "",
				Kind:              "Function",
			}
			if call.Name == "include" {
				item.Command = "editor.action.triggerSuggest"
			}
			native_call_suggestion = append(native_call_suggestion, item)
		}
	}
	return native_call_suggestion
}

func getLibrarySuggestion() []*ypb.SuggestionDescription {
	library_suggestion_lock.Lock()
	defer library_suggestion_lock.Unlock()

	if len(library_suggestion) == 0 {

		// items := make([]*ypb.SuggestionDescription, 0)
		db := consts.GetGormProfileDatabase()
		sfbuildin.SyncEmbedRule()
		db = db.Where("allow_included = ? and included_name != ?", true, "")
		for rule := range sfdb.YieldSyntaxFlowRules(db, context.Background()) {
			item := &ypb.SuggestionDescription{
				Label:             rule.IncludedName,
				Description:       rule.Description,
				InsertText:        fmt.Sprintf(`"%s"`, rule.IncludedName),
				JustAppend:        false,
				DefinitionVerbose: "",
				Kind:              "File",
			}
			library_suggestion = append(library_suggestion, item)
		}
	}
	return library_suggestion
}
