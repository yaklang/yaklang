package yakscripttools

import (
	"context"
	"embed"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"io/fs"
	"strings"

	_ "github.com/yaklang/yaklang/common/yak"
)

//go:embed yakscriptforai/**
var yakScriptFS embed.FS

func GetYakScriptAiTools(name ...string) []*aitool.Tool {
	efs := filesys.NewEmbedFS(yakScriptFS)
	tools := []*aitool.Tool{}
	_ = filesys.Recursive(".", filesys.WithFileSystem(efs), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		filename := info.Name()
		_, filename = efs.PathSplit(filename)
		if efs.Ext(filename) != ".yak" {
			return nil
		}
		toolname := strings.TrimSuffix(filename, ".yak")

		found := false
		for _, i := range name {
			if i == toolname {
				found = true
			}
		}
		if !found {
			return nil
		}

		content, err := efs.ReadFile(s)
		if err != nil {
			return nil
		}
		prog, err := static_analyzer.SSAParse(string(content), "yak")
		if err != nil {
			log.Warnf(`static_analyzer.SSAParse(string(content), "yak") error: %v`, err)
			return err
		}
		var desc []string
		prog.Ref("__DESC__").ForEach(func(value *ssaapi.Value) {
			if !value.IsConstInst() {
				return
			}
			desc = append(desc, value.String())
		})
		tool := yakcliconvert.ConvertCliParameterToTool(toolname, prog)
		at, err := aitool.NewFromMCPTool(
			tool,
			aitool.WithDescription(strings.Join(desc, "; ")),
			aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				runtimeId := params.GetString("runtime_id")
				if runtimeId == "" {
					runtimeId = uuid.New().String()
				}
				yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(i *ypb.ExecResult) error {
					if i.IsMessage {
						stdout.Write([]byte(yaklib.ConvertExecResultIntoLog(i)))
					}
					return nil
				}, runtimeId)
				engine := yak.NewYakitVirtualClientScriptEngine(yakitClient)

				var args []string
				for k, v := range params {
					args = append(args, "--"+k, fmt.Sprint(v))
				}
				cliApp := yak.GetHookCliApp(args)
				engine.RegisterEngineHooks(func(ae *antlr4yak.Engine) error {
					yak.BindYakitPluginContextToEngine(
						ae,
						yak.CreateYakitPluginContext(
							runtimeId,
						).WithContext(
							ctx,
						).WithContextCancel(
							cancel,
						).WithCliApp(
							cliApp,
						).WithYakitClient(
							yakitClient,
						),
					)
					return nil
				})

				_, err = engine.ExecuteExWithContext(ctx, string(content), map[string]interface{}{
					"RUNTIME_ID":   runtimeId,
					"CTX":          ctx,
					"PLUGIN_NAME":  runtimeId + ".yak",
					"YAK_FILENAME": runtimeId + ".yak",
					"println": func(i ...any) {
						funk.ForEach(i, func(v any) {
							stdout.Write([]byte(utils.InterfaceToString(v)))
						})
					},
				})
				if err != nil {
					log.Errorf("execute ex with context failed: %v", err)
					return nil, err
				}
				return "", nil
			}))
		if err != nil {
			log.Errorf(`at.NewFromMCPTool(tool): %v`, err)
			return nil
		}
		tools = append(tools, at)
		return nil
	}))
	return tools
}
