package yakgrpc

import (
	"context"
	"yaklang/common/go-funk"
	"yaklang/common/utils"
	"yaklang/common/yak"
	"yaklang/common/yak/antlr4yak/yakvm"
	"yaklang/common/yak/yaklang"
	"yaklang/common/yakdocument"
	"yaklang/common/yakgrpc/ypb"
	"time"

	"github.com/davecgh/go-spew/spew"
)

var completionJsonCd = utils.NewCoolDown(5 * time.Second)
var (
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
		var suggestion = make([]*ypb.SuggestionDescription, len(stringBuildin))
		var index = 0
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
	tmpRes := yak.AnalyzeStaticYaklang(r.GetCode())
	es := funk.Map(tmpRes, func(i *yak.StaticAnalyzeResult) *ypb.StaticAnalyzeErrorResult {
		return &ypb.StaticAnalyzeErrorResult{
			Message:         []byte(i.Message),
			StartLineNumber: int64(i.StartLineNumber),
			EndLineNumber:   int64(i.EndLineNumber),
			StartColumn:     int64(i.StartColumn),
			EndColumn:       int64(i.EndColumn),
			RawMessage:      []byte(i.RawMessage),
			Severity:        i.Severity,
		}
	}).([]*ypb.StaticAnalyzeErrorResult)
	return &ypb.StaticAnalyzeErrorResponse{Result: es}, nil
}
