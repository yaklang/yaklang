package loop_ssa_api_discovery

import (
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/reactive_data.txt
var reactiveDataTpl string

//go:embed prompts/qa_root_instruction.txt
var qaRootInstruction string

func withSsaDiscoveryRootPersistentContext() reactloops.ReActLoopOption {
	return reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
		if loop == nil || strings.TrimSpace(loop.Get("ssa_discovery_mode")) != SsaDiscoveryModeQAReview {
			return "", nil
		}
		s, err := utils.RenderTemplate(qaRootInstruction, map[string]any{"Nonce": nonce})
		if err != nil {
			return qaRootInstruction, nil
		}
		return s, nil
	})
}

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				withSsaDiscoveryRootPersistentContext(),
				reactloops.WithInitTask(buildInitTask(r)),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY, r, preset...)
		},
		reactloops.WithLoopDescription("Static-dynamic closed-loop security audit for authorized targets: local SSA attack-surface discovery → architecture report → SyntaxFlow static scan → live HTTP/greybox vuln verification with cross-validation → final audit report. Persists to SQLite and workdir."),
		reactloops.WithLoopDescriptionZh("静动闭环安全审计模式：针对本地源码与远程靶机，五阶段流水线（攻击面发现与 HTTP 初验→架构报告→SyntaxFlow 静态扫描→动态漏洞验证→总报告），静动结论交叉印证并写入 SQLite 与工作目录。"),
		reactloops.WithVerboseName("Static-Dynamic Closed-Loop Audit"),
		reactloops.WithVerboseNameZh("静动闭环安全审计"),
		reactloops.WithLoopUsagePrompt(`当用户提供本地代码目录与靶机 URL/主机，需要对源码做静态审计（SSA 攻击面发现、SyntaxFlow 扫描）并在靶机上动态验洞、交叉印证时使用。入口路由 full_pipeline（默认五阶段；缺 Code path/Target 时可用 LiteForge 提取）或 qa_review（审计问答，不跑五阶段流水线）。full_pipeline 可用自然语言限定「跑到第 N 阶段」，系统**顺序**执行 1～N、不跳步，例如「跑到第 3 阶段」「Pipeline max stage: 4」。阶段：1=攻击面发现（Yak 预分析+代码通读+verified_http_apis 探测）；2=API/架构报告；3=SyntaxFlow 静态扫描；4=动态验洞（鉴权→静态发现验证→灰盒批量检测）；5=最终审计报告。HTTP 端点真源为 **verified_http_apis**。首条消息含 Code path: 与 Target。`),
		reactloops.WithLoopOutputExample(`
* 对本地代码 + 靶机做静动闭环审计：
  {"@action": "ssa_api_discovery", "human_readable_thought": "对路径 X 的源码相对靶机 Y 做静动闭环安全审计"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop %s failed: %v", schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY, err)
	}
}
