package loop_ssa_api_discovery

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// EnrichParsedWithAIExtract uses LiteForge to extract pipeline bootstrap fields from free-form user text.
// Regex/heuristic fields in base are kept; AI fills empty slots and can recover auth when Markdown escapes break line parsers.
func EnrichParsedWithAIExtract(ctx context.Context, r aicommon.AIInvokeRuntime, userText string, base *ParsedUserInput) (*ParsedUserInput, error) {
	if base == nil {
		return nil, utils.Error("nil parsed")
	}
	if r == nil || strings.TrimSpace(userText) == "" {
		return base, nil
	}
	prompt := utils.MustRenderTemplate(`从用户消息中提取 SSA API 发现流水线需要的结构化参数。路径用本机绝对路径；靶机可为 http(s) URL 或 host:port；认证可为 auth: user/pass、user:pass、user/pass 或单独的 auth-password。

用户输入：
<|USER_INPUT|>
{{ .UserText }}
<|END|>

规则：
- 没有明确信息的字段留空字符串，不要猜测默认密码
- auth 行可能写成 auth:、auth\\:、认证:、账户: 等形式；user/pass 分隔符可为 / 或 :
- 若 code_path 与 target 已在启发式解析中出现，仍可在 AI 字段中重复确认；合并时优先保留已有非空值`, map[string]any{"UserText": userText})

	act, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"ssa_discovery_extract_input",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("code_path",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("本地源代码根目录绝对路径")),
			aitool.WithStringParam("target",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("靶机 base URL 或 host:port")),
			aitool.WithStringParam("language_hint",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("java/go/javascript 等小写 hint")),
			aitool.WithStringParam("auth_username",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("登录用户名")),
			aitool.WithStringParam("auth_password",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("登录密码")),
			aitool.WithStringParam("auth_line",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("原始认证片段 user/pass 或仅密码")),
			aitool.WithIntegerParam("pipeline_max_stage",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("流水线阶段上限 1-5，无则 0")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("提取依据（可选）")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("ssa_input_extract", "reason"),
	)
	if err != nil {
		log.Warnf("ssa_api_discovery: ssa_discovery_extract_input failed: %v", err)
		return base, nil
	}
	overlay := &ParsedUserInput{
		CodePath:     NormalizePathFromUserInput(act.GetString("code_path")),
		TargetRaw:    NormalizePathFromUserInput(act.GetString("target")),
		LanguageHint: NormalizePathFromUserInput(act.GetString("language_hint")),
		AuthUsername: strings.TrimSpace(act.GetString("auth_username")),
		AuthPassword: strings.TrimSpace(act.GetString("auth_password")),
		AuthLine:     strings.TrimSpace(act.GetString("auth_line")),
	}
	if n := act.GetInt("pipeline_max_stage"); n > 0 {
		overlay.PipelineMaxStage = int(n)
	}
	if overlay.TargetRaw != "" {
		overlay.TargetRaw = NormalizeTargetString(overlay.TargetRaw)
	}
	log.Infof("ssa_api_discovery: ai input extract code=%q target=%q auth_user=%q has_pass=%v",
		utils.ShrinkString(overlay.CodePath, 60),
		utils.ShrinkString(overlay.TargetRaw, 60),
		overlay.AuthUsername,
		overlay.AuthPassword != "" || overlay.AuthLine != "",
	)
	return MergeParsedPreferBase(base, overlay), nil
}

// ResolveUserCredentials returns username/password from parsed input; empty password means caller may skip login.
func ResolveUserCredentials(parsed *ParsedUserInput) (username, password string) {
	if parsed == nil {
		return "", ""
	}
	if acc := firstCredentialAccount(parsed.AuthCredentialGroups); acc != nil {
		return acc.Username, acc.Password
	}
	if u := strings.TrimSpace(parsed.AuthUsername); u != "" {
		username = u
	}
	if p := strings.TrimSpace(parsed.AuthPassword); p != "" {
		password = p
	}
	if username != "" && password != "" {
		return username, password
	}
	if line := strings.TrimSpace(parsed.AuthLine); line != "" {
		u, p := parseAuthCredentials(line)
		if username == "" && u != "" {
			username = u
		}
		if password == "" && p != "" {
			password = p
		}
	}
	return username, password
}
