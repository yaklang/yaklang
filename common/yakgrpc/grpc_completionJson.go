package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakdocument"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/davecgh/go-spew/spew"
)

var (
	completionJsonCd  = utils.NewCoolDown(5 * time.Second)
	completionJsonRaw []byte
)

func (s *Server) GetYakVMBuildInMethodCompletion(
	ctx context.Context,
	req *ypb.GetYakVMBuildInMethodCompletionRequest,
) (*ypb.GetYakVMBuildInMethodCompletionResponse, error) {
	if !yaklang.IsNew() {
		return &ypb.GetYakVMBuildInMethodCompletionResponse{}, nil
	}
	var sug []*ypb.MethodSuggestion
	stringBuildin := yakvm.GetStringBuildInMethod()
	if stringBuildin != nil && len(stringBuildin) > 0 {
		suggestion := make([]*ypb.SuggestionDescription, len(stringBuildin))
		index := 0
		for methodName, method := range stringBuildin {
			if ret, _ := method.VSCodeSnippets(); ret == "" {
				spew.Dump(method)
				continue
			}
			snippets, verbose := method.VSCodeSnippets()
			suggestion[index] = &ypb.SuggestionDescription{
				Label:             methodName,
				Description:       method.Description,
				InsertText:        snippets,
				DefinitionVerbose: verbose,
			}
			index++
		}
		sug = append(sug, &ypb.MethodSuggestion{
			FuzzKeywords: []string{
				"str", "host", "name", "word", "payload",
				"raw", "bytes", "packet", "packets",
			},
			ExactKeywords: []string{"s", "ss", "a", "b", "abc"},
			Suggestions:   suggestion,
			Verbose:       "(string)",
		})
	}

	if ret := yakvm.GetSliceBuildInMethod(); ret != nil && len(ret) > 0 {
		var descs []*ypb.SuggestionDescription
		for name, method := range ret {
			if s, _ := method.VSCodeSnippets(); s == "" {
				spew.Dump(method)
				continue
			}
			s, v := method.VSCodeSnippets()
			descs = append(descs, &ypb.SuggestionDescription{
				Label:             name,
				Description:       method.Description,
				InsertText:        s,
				DefinitionVerbose: v,
			})
		}
		sug = append(sug, &ypb.MethodSuggestion{
			FuzzKeywords: []string{
				"list", "slice", "all", "li",
				"raw", "names", "passwords", "payloads",
				"usernames", "dict", "ports", "hosts",
				"result", "numbers",
			},
			ExactKeywords: []string{"ll", "l", "a", "b", "li", "abc"},
			Suggestions:   descs,
			Verbose:       `(list)`,
		})
	}

	if ret := yakvm.GetMapBuildInMethod(); ret != nil && len(ret) > 0 {
		var descs []*ypb.SuggestionDescription
		for name, method := range ret {
			if s, _ := method.VSCodeSnippets(); s == "" {
				spew.Dump(method)
				continue
			}
			s, v := method.VSCodeSnippets()
			descs = append(descs, &ypb.SuggestionDescription{
				Label:             name,
				Description:       method.Description,
				InsertText:        s,
				DefinitionVerbose: v,
			})
		}
		sug = append(sug, &ypb.MethodSuggestion{
			FuzzKeywords: []string{
				"pair", "result", "raw", "map",
				"dict",
			},
			ExactKeywords: []string{"m", "dict", "d", "r"},
			Suggestions:   descs,
			Verbose:       `(map)`,
		})
	}

	return &ypb.GetYakVMBuildInMethodCompletionResponse{
		Suggestions: sug,
	}, nil
}

func (s *Server) GetYakitCompletionRaw(ctx context.Context, _ *ypb.Empty) (*ypb.YakitCompletionRawResponse, error) {
	if completionJsonRaw != nil {
		return &ypb.YakitCompletionRawResponse{RawJson: completionJsonRaw}, nil
	}

	completionJsonCd.Do(func() {
		libs := yak.EngineToLibDocuments(yaklang.New())
		completionJsonRaw, _ = yakdocument.LibDocsToCompletionJsonShort(libs...)
	})
	return &ypb.YakitCompletionRawResponse{RawJson: completionJsonRaw}, nil
}

func (s *Server) StaticAnalyzeError(ctx context.Context, r *ypb.StaticAnalyzeErrorRequest) (*ypb.StaticAnalyzeErrorResponse, error) {
	tmpRes := yak.StaticAnalyze(string(r.GetCode()), yak.WithStaticAnalyzePluginType(r.GetPluginType()))
	es := lo.Map(tmpRes, func(i *result.StaticAnalyzeResult, _ int) *ypb.StaticAnalyzeErrorResult {
		return &ypb.StaticAnalyzeErrorResult{
			Message:         []byte(i.Message),
			StartLineNumber: i.StartLineNumber,
			EndLineNumber:   i.EndLineNumber,
			StartColumn:     i.StartColumn,
			EndColumn:       i.EndColumn,
			// RawMessage:      []byte{},
			Severity: string(i.Severity),
			Tag:      string(i.Tag),
		}
	})
	return &ypb.StaticAnalyzeErrorResponse{Result: es}, nil
}
