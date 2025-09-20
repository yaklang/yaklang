package yaklangtools

import (
	"io"
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const YaklangToolName_SyntaxCheck = "check-yaklang-syntax"
const YaklangToolName_Document = "yak-document"

func CreateYaklangTools() ([]*aitool.Tool, error) {
	var err error
	factory := aitool.NewFactory()
	err = factory.RegisterTool(YaklangToolName_SyntaxCheck,
		aitool.WithDescription("run yaklang code syntax check"),
		aitool.WithStringParam("content", aitool.WithParam_Description("yaklang code content")),
		aitool.WithStringParam("path", aitool.WithParam_Description("yaklang code file path")),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			codeContent := params.GetString("content")
			if codeContent == "" {
				path := params.GetString("path")
				if path == "" {
					return nil, utils.Error("yaklang code content or path is required")
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return nil, utils.Errorf("read file %s failed: %s", path, err)
				}
				codeContent = string(content)
			}
			checkRes := static_analyzer.StaticAnalyze(codeContent, "yak", static_analyzer.Compile)
			return checkRes, nil
		}),
	)
	if err != nil {
		log.Errorf("register ls tool: %v", err)
	}

	err = factory.RegisterTool("yak-document",
		aitool.WithDescription("query yaklang document"),
		aitool.WithStringParam("keyword", aitool.WithParam_Description("yaklang code keyword")),
		aitool.WithStringParam("lib", aitool.WithParam_Description("yaklang code lib name")),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			codeContent := params.GetString("keyword")
			libName := params.GetString("lib")
			if libName != "" {
				codeContent = libName + "." + codeContent
			}
			docContent, err := yakurl.GetActionService().
				GetAction("yakdocument").
				Get(&ypb.RequestYakURLParams{
					Url: &ypb.YakURL{
						Location: codeContent,
					},
				})
			if err != nil {
				return nil, err
			}
			return docContent.Resources, nil
		}),
	)
	if err != nil {
		log.Errorf("register ls tool: %v", err)
	}
	return factory.Tools(), nil
}
