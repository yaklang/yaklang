package loop_ssa_api_discovery

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// 用户可从提示中指定流水线执行到的阶段编号（顺序执行 1～N，不跳步）。
// 1：Phase1 攻击面发现与 HTTP 初验
// 2：Phase2 API 与架构分析报告
// 3：Phase3 SyntaxFlow 静态扫描
// 4：Phase4 动态验证管线
// 5：Phase5 最终报告（默认未指定时跑满 5）
const (
	PipelineStageMin     = 1
	PipelineStageFullMax = 5
	pipelineStageLegacyMax = 6
)

// NormalizePathFromUserInput removes common LLM/Markdown artifacts from a path string
// (e.g. escaped underscores `\_` → `_`, trimming quotes/backticks).
func NormalizePathFromUserInput(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "`'\"")
	for strings.Contains(p, `\_`) {
		p = strings.ReplaceAll(p, `\_`, `_`)
	}
	p = strings.ReplaceAll(p, `\ `, " ")
	p = strings.TrimSpace(p)
	return p
}

var (
	reCodePath = regexp.MustCompile(`(?im)^\s*code(?:\s*path)?\s*[:：=]\s*(.+?)\s*$`)
	reTarget   = regexp.MustCompile(`(?im)^\s*(?:target|靶机|目标(?:地址)?|base\s*url|host)\s*[:：=]\s*(.+?)\s*$`)
	reLanguage = regexp.MustCompile(`(?im)^\s*language\s*[:：=]\s*(.+?)\s*$`)

	rePipelineMaxStageLine = regexp.MustCompile(`(?im)^\s*(?:pipeline\s*max\s*stage|max\s*stage|流水线阶段上限|阶段上限)\s*[:：=]\s*(\d+)\s*$`)
	reRunToStageN          = regexp.MustCompile(`(?i)跑到第\s*(\d+)\s*阶段`)
	reRunToStageInline     = regexp.MustCompile(`(?i)(?:pipeline\s*max\s*stage|max\s*stage|流水线阶段上限|阶段上限)\s*[:：=]\s*(\d+)`)
	reStageLine            = regexp.MustCompile(`(?im)^\s*阶段\s*[:：=]\s*(\d+)\s*$`)
	reRunPhaseInline       = regexp.MustCompile(`(?i)跑到阶段\s*[:：=]?\s*(\d+)`)
	reDatabaseDSN          = regexp.MustCompile(`(?im)^\s*(?:database\s*dsn|session\s*db(?:\s*dsn)?)\s*[:：=]\s*(.+?)\s*$`)
	reSessionUUID          = regexp.MustCompile(`(?im)^\s*session\s*uuid\s*[:：=]\s*(.+?)\s*$`)
	rePipelineResume       = regexp.MustCompile(`(?im)^\s*(?:pipeline\s*resume|resume\s*pipeline|续跑|恢复流水线)\s*[:：=]?\s*(yes|true|1|是|on)\s*$`)
	rePipelineResumeStage  = regexp.MustCompile(`(?im)^\s*(?:pipeline\s*resume\s*from\s*stage|resume\s*from\s*stage|续跑阶段|从阶段续跑)\s*[:：=]\s*(\d+)\s*$`)
	// api-arch-test（推荐，避免 Markdown 转义下划线）；兼容 api_arch_test
	reApiArchTest = regexp.MustCompile(`(?im)^\s*api[-_]arch[-_]test\s*[:：=]?\s*(yes|true|1|是|on)\s*$`)
	reAuthLine             = regexp.MustCompile(`(?im)^\s*(?:auth|认证(?:信息)?|账户)\s*[:：=]\s*(.+?)\s*$`)
	reAuthPassword         = regexp.MustCompile(`(?im)^\s*(?:auth[-_]password|密码)\s*[:：=]\s*(.+?)\s*$`)
	rePromptVariant        = regexp.MustCompile(`(?im)^\s*(?:prompt[-_]variant|变体|策略)\s*[:：=]\s*(.+?)\s*$`)
	rePhase4ModeLine       = regexp.MustCompile(`(?im)^\s*(?:phase4[-_]?mode|phase\s*4\s*mode|第四阶段模式|深度挖掘模式)\s*[:：=]\s*(.+?)\s*$`)
	reAuthUsername         = regexp.MustCompile(`(?im)^\s*(?:auth[-_]username|用户名)\s*[:：=]\s*(.+?)\s*$`)
	reSkipDirectoryAnalysis = regexp.MustCompile(`(?im)^\s*(?:skip\s*(?:directory\s*analysis|bfs|dir\s*analysis)|skip\s*bfs)\s*[:：=]?\s*(?:yes|true|1|是|on|skip)\s*$`)
	reSkipDirectoryAnalysisCN = regexp.MustCompile(`(?im)^\s*目录分析\s*[:：=]\s*跳过\s*$`)
	reAllowPartialAuth      = regexp.MustCompile(`(?im)^\s*(?:partial[-_]auth|auth[-_]partial|部分鉴权)\s*[:：=]?\s*(?:yes|true|1|是|on|allow|允许|开启)\s*$`)
	reFrameworkToolkitOn    = regexp.MustCompile(`(?im)^\s*(?:framework[-_]toolkit|框架工具包)\s*[:：=]?\s*(?:yes|true|1|是|on|开启|enable)\s*$`)
	reFrameworkToolkitOff   = regexp.MustCompile(`(?im)^\s*(?:framework[-_]toolkit|框架工具包)\s*[:：=]?\s*(?:no|false|0|否|off|disable|关闭)\s*$`)
	reFrameworkToolkitCNOn  = regexp.MustCompile(`(?im)^\s*框架工具包\s*[:：=]\s*开启\s*$`)
)

// ParsedUserInput carries structured fields extracted from free-form user text.
type ParsedUserInput struct {
	CodePath     string
	TargetRaw    string
	LanguageHint string
	// SessionDBDSN 非空时使用 PostgreSQL 作为 discovery 会话库。
	SessionDBDSN string
	// SessionUUID 外部任务绑定（如十链鉴 task_id）。
	SessionUUID string
	// PipelineMaxStage 为 0 表示未指定（跑满 PipelineStageFullMax）；否则为 1～5，流水线顺序执行至该阶段后结束。
	PipelineMaxStage int
	// PipelineResume 为 true 时从 session phase 推断 checkpoint 续跑。
	PipelineResume bool
	// PipelineResumeFromStage 显式指定从第 N 阶段续跑（1～5），优先于 PipelineResume。
	PipelineResumeFromStage int
	// ApiArchTest 已废弃：请使用独立 focus loop ssa_api_discovery_test_api_arch + cmd/run_api_arch_prompt_benchmark。
	ApiArchTest bool
	// PromptVariant 指定 api arch 测试变体 ID（如 v1_hybrid）；空则默认 v1_hybrid。
	PromptVariant string
	// AuthUsername 登录用户名；空时 benchmark 按变体序号分配 admin1..admin9。
	AuthUsername string
	// AuthPassword 登录密码（可与 AuthLine 二选一）。
	AuthPassword string
	// AuthLine 原始认证信息（user/pass、user:pass 或仅密码）。
	AuthLine string
	// AuthCredentialGroups 多组凭证；每组可含多个账号（见 admin_auth / user_auth / auth_group）。
	AuthCredentialGroups []UserCredentialGroup
	// SSH 远程源码拉取（见 remote_code_path / ssh_host 等）；bootstrap 前 SFTP 同步到 workDir/remote_code/…
	SSHHost          string
	SSHPort          int
	SSHUsername      string
	SSHPassword      string
	SSHPrivateKey    string
	SSHKeyPassphrase string
	RemoteCodePath   string // absolute path on remote SSH host
	// Phase4Mode deep_mining（默认）或 batch_scan（legacy 批量灰盒）。
	Phase4Mode string
	// SkipDirectoryAnalysis 为 true 时跳过 D step 目录 BFS，并从 code_unit_registry 回填 feature_inventory。
	SkipDirectoryAnalysis bool
	// AllowPartialAuth 为 true 时允许部分 realm 鉴权成功后继续 API 探测（未鉴权 realm 的接口程序化跳过）。
	AllowPartialAuth bool
	// FrameworkToolkitEnabled 为 true 时启用 Framework Toolkit 快车道（鉴权+API 提取+验证程序化）。
	FrameworkToolkitEnabled bool
}

// normalizeMarkdownEscapesInUserInput strips common Markdown artifacts (e.g. `\_` → `_`)
// so flag lines like api\_arch\_test still match api_arch_test / api-arch-test.
func normalizeMarkdownEscapesInUserInput(userText string) string {
	for strings.Contains(userText, `\_`) {
		userText = strings.ReplaceAll(userText, `\_`, `_`)
	}
	userText = strings.ReplaceAll(userText, `\:`, `:`)
	userText = strings.ReplaceAll(userText, `\=`, `=`)
	return userText
}

// extractUserInputFields parses labeled lines and heuristics; CodePath may be empty.
func extractUserInputFields(userText string) (*ParsedUserInput, error) {
	userText = normalizeMarkdownEscapesInUserInput(strings.TrimSpace(userText))
	if userText == "" {
		return nil, utils.Error("empty user input")
	}
	out := &ParsedUserInput{}

	if m := reCodePath.FindStringSubmatch(userText); len(m) > 1 {
		out.CodePath = NormalizePathFromUserInput(m[1])
	}
	if m := reTarget.FindStringSubmatch(userText); len(m) > 1 {
		out.TargetRaw = NormalizePathFromUserInput(m[1])
	}
	if m := reLanguage.FindStringSubmatch(userText); len(m) > 1 {
		out.LanguageHint = NormalizePathFromUserInput(m[1])
	}
	if m := reDatabaseDSN.FindStringSubmatch(userText); len(m) > 1 {
		out.SessionDBDSN = strings.TrimSpace(NormalizePathFromUserInput(m[1]))
	}
	if m := reSessionUUID.FindStringSubmatch(userText); len(m) > 1 {
		out.SessionUUID = strings.TrimSpace(NormalizePathFromUserInput(m[1]))
	}
	if rePipelineResume.MatchString(userText) {
		out.PipelineResume = true
	}
	if reApiArchTest.MatchString(userText) {
		out.ApiArchTest = true
	}
	if m := reAuthLine.FindStringSubmatch(userText); len(m) > 1 {
		out.AuthLine = strings.TrimSpace(m[1])
	}
	if m := reAuthPassword.FindStringSubmatch(userText); len(m) > 1 {
		out.AuthPassword = strings.TrimSpace(m[1])
		if out.AuthLine == "" {
			out.AuthLine = out.AuthPassword
		}
	}
	if m := rePromptVariant.FindStringSubmatch(userText); len(m) > 1 {
		out.PromptVariant = strings.TrimSpace(m[1])
	}
	if m := rePhase4ModeLine.FindStringSubmatch(userText); len(m) > 1 {
		out.Phase4Mode = NormalizePhase4Mode(strings.TrimSpace(m[1]))
	}
	if m := reAuthUsername.FindStringSubmatch(userText); len(m) > 1 {
		out.AuthUsername = strings.TrimSpace(m[1])
	}
	if m := rePipelineResumeStage.FindStringSubmatch(userText); len(m) > 1 {
		if n, err := strconv.Atoi(strings.TrimSpace(m[1])); err == nil && n >= PipelineStageMin && n <= pipelineStageLegacyMax {
			out.PipelineResumeFromStage = n
			out.PipelineResume = true
		}
	}

	if reSkipDirectoryAnalysis.MatchString(userText) || reSkipDirectoryAnalysisCN.MatchString(userText) {
		out.SkipDirectoryAnalysis = true
	}
	if reAllowPartialAuth.MatchString(userText) {
		out.AllowPartialAuth = true
	}
	if reFrameworkToolkitOn.MatchString(userText) || reFrameworkToolkitCNOn.MatchString(userText) {
		out.FrameworkToolkitEnabled = true
	}
	if reFrameworkToolkitOff.MatchString(userText) {
		out.FrameworkToolkitEnabled = false
	}

	applySSHFieldsFromUserText(out, userText)

	if out.CodePath == "" && !SSHRemoteSourceConfigured(out) {
		out.CodePath = NormalizePathFromUserInput(guessAbsolutePath(userText))
	}
	if out.TargetRaw == "" {
		out.TargetRaw = NormalizePathFromUserInput(guessTarget(userText))
	}
	if out.TargetRaw != "" {
		out.TargetRaw = NormalizeTargetString(out.TargetRaw)
	}

	if n := extractPipelineMaxStageFromText(userText); n != 0 {
		out.PipelineMaxStage = n
	}

	if groups := parseCredentialGroupsFromUserText(userText); len(groups) > 0 {
		out.AuthCredentialGroups = groups
	}
	ensureDefaultCredentialGroup(out)
	syncLegacyAuthFieldsFromGroups(out)

	return out, nil
}

// NormalizePipelineMaxStage 将解析到的阶段钳制到合法范围；n<=0 视为未限制（跑满全流程）。
// 兼容旧版「跑到第 6 阶段」输入，自动映射为 5。
func NormalizePipelineMaxStage(n int) int {
	if n <= 0 {
		return PipelineStageFullMax
	}
	if n == pipelineStageLegacyMax {
		return PipelineStageFullMax
	}
	if n < PipelineStageMin {
		return PipelineStageMin
	}
	if n > PipelineStageFullMax {
		return PipelineStageFullMax
	}
	return n
}

func extractPipelineMaxStageFromText(userText string) int {
	try := func(s string) int {
		var patterns = []*regexp.Regexp{
			reRunToStageN,
			rePipelineMaxStageLine,
			reRunToStageInline,
			reStageLine,
			reRunPhaseInline,
		}
		for _, re := range patterns {
			if m := re.FindStringSubmatch(s); len(m) > 1 {
				n, err := strconv.Atoi(strings.TrimSpace(m[1]))
				if err != nil {
					continue
				}
				if n >= PipelineStageMin && n <= pipelineStageLegacyMax {
					return n
				}
			}
		}
		return 0
	}
	if n := try(userText); n != 0 {
		return n
	}
	for _, line := range strings.Split(userText, "\n") {
		if n := try(strings.TrimSpace(line)); n != 0 {
			return n
		}
	}
	return 0
}

// ParseUserInputLenient extracts code path, target, and language without requiring Code path.
// Init may merge CodePath from the latest SQLite session in the same work directory.
func ParseUserInputLenient(userText string) (*ParsedUserInput, error) {
	return extractUserInputFields(userText)
}

// ParseUserInput extracts code path, target address, and optional language hint.
func ParseUserInput(userText string) (*ParsedUserInput, error) {
	out, err := extractUserInputFields(userText)
	if err != nil {
		return nil, err
	}
	if out.CodePath == "" {
		return nil, utils.Error("code path not found: use \"Code path: /abs/path\", or provide SSH remote source (ssh_host + remote_code_path + ssh_password/ssh_key); or continue a prior run in the same Yakit work dir")
	}
	return out, nil
}

func guessAbsolutePath(text string) string {
	reAbs := regexp.MustCompile(`(?:^|\s)(/[^\s:]+)`)
	if m := reAbs.FindStringSubmatch(text); len(m) > 1 {
		p := m[1]
		p = strings.TrimRight(p, ",.")
		return p
	}
	return ""
}

func guessTarget(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "/") && !strings.Contains(line, "://") {
			continue
		}
		if strings.Contains(line, "://") {
			if u, err := url.Parse(line); err == nil && u.Host != "" {
				return line
			}
		}
		if strings.Contains(line, ":") {
			if _, _, err := utils.ParseStringToHostPort(line); err == nil {
				return line
			}
		}
	}
	return ""
}

// AbsCodeDir returns absolute, cleaned directory path or an error.
func AbsCodeDir(p string) (string, error) {
	p = NormalizePathFromUserInput(p)
	if p == "" {
		return "", utils.Error("empty path")
	}
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", utils.Wrapf(err, "filepath.Abs")
		}
		p = abs
	}
	p = filepath.Clean(p)
	st, err := os.Stat(p)
	if err != nil {
		return "", utils.Wrapf(err, "stat code path")
	}
	if !st.IsDir() {
		return "", utils.Errorf("code path is not a directory: %s", p)
	}
	return p, nil
}
