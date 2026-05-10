package reactloops

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/utils"
)

// defaultPromptSummaryBytes 给前端 "上下文成分" 面板的 summary 默认上限.
//
// 历史值: 120 (老路径, 还把空格 / 换行压平, 用户基本看不到任何有效内容).
// 中间方案: 4096 (升 34x, 但 8K 量级的段仍被截, 用户看到 "(truncated, total 8693 bytes)"
// 反而想了解段细节的诉求被打断).
//
// 当前值: 0 (无上限, 完整透传段内容). 用户实测段体量在数 KB ~ 数十 KB 量级,
// EmitPromptProfile event 走本地 ipc, 这个量级 payload 完全可承受;
// 调用方如果出于带宽 / 渲染成本想强制截断, 可以显式给 BuildStatus 传正数 maxBytes.
//
// 关键词: defaultPromptSummaryBytes, prompt_profile summary 不截断, 上下文成分完整展示
const defaultPromptSummaryBytes = 0

const lastPromptObservationLoopKey = "last_ai_decision_prompt_observation"
const lastPromptObservationStatusLoopKey = "last_ai_decision_prompt_observation_status"
const ReActPromptObservationStatusKey = "re-act-prompt-observation-status"

type PromptSectionRole string

// PromptSectionRole 是上下文字节统计图 / 上下文成分面板用来按段分类的枚举.
//
// 注意 SemiDynamic 段在 P1.1 之后物理上拆成两块 (semi-dynamic-1 +
// semi-dynamic-2), 字节统计图需要把这两块作为独立类型分开统计, 否则跨 turn
// 字节抖动会被合并掩盖, 让面板趋势线不稳定. 因此引入两个新 Role:
//   - PromptSectionRoleSemiDynamic1 ("semi_dynamic_1") -> "半动态段1"
//   - PromptSectionRoleSemiDynamic2 ("semi_dynamic_2") -> "半动态段2"
//
// 老 PromptSectionRoleSemiDynamic ("semi_dynamic") 保留供老 caller 与未拆分的
// 测试 fixture 使用 (liteforge / aireduce / 老快照对比), 新 aireact 主路径渲染
// 与观测树都用 1/2 拆分版本.
//
// 关键词: PromptSectionRole 拆 SemiDynamic1/2, 字节统计独立分类, P1.1
const (
	PromptSectionRoleHighStatic    PromptSectionRole = "high_static"
	PromptSectionRoleFrozenBlock   PromptSectionRole = "frozen_block"
	PromptSectionRoleSemiDynamic   PromptSectionRole = "semi_dynamic"
	PromptSectionRoleSemiDynamic1  PromptSectionRole = "semi_dynamic_1"
	PromptSectionRoleSemiDynamic2  PromptSectionRole = "semi_dynamic_2"
	PromptSectionRoleTimelineOpen  PromptSectionRole = "timelineOpen"
	PromptSectionRoleDynamic       PromptSectionRole = "dynamic"
)

type PromptSectionRoleZH string

const (
	PromptSectionRoleZHHighStatic    PromptSectionRoleZH = "高静态段"
	PromptSectionRoleZHFrozenBlock   PromptSectionRoleZH = "冻结块"
	PromptSectionRoleZHSemiDynamic   PromptSectionRoleZH = "半动态段"
	PromptSectionRoleZHSemiDynamic1  PromptSectionRoleZH = "半动态段1"
	PromptSectionRoleZHSemiDynamic2  PromptSectionRoleZH = "半动态段2"
	PromptSectionRoleZHTimelineOpen  PromptSectionRoleZH = "时间线开放段"
	PromptSectionRoleZHDynamic       PromptSectionRoleZH = "动态段"
)

type PromptSectionObservation struct {
	Key          string                      `json:"key"`
	Label        string                      `json:"label"`
	Role         PromptSectionRole           `json:"role"`
	RoleZh       PromptSectionRoleZH         `json:"role_zh"`
	Included     bool                        `json:"included"`
	Compressible bool                        `json:"compressible"`
	Bytes        int                         `json:"bytes"`
	Lines        int                         `json:"lines"`
	Content      string                      `json:"content,omitempty"`
	Children     []*PromptSectionObservation `json:"children,omitempty"`
}

type PromptObservationRoleStat struct {
	RoleName   PromptSectionRole   `json:"role_name"`
	RoleNameZh PromptSectionRoleZH `json:"role_name_zh"`
	RoleBytes  int                 `json:"role_bytes"`
}

type PromptObservationStats struct {
	RoleStats         []PromptObservationRoleStat `json:"role_stats"`
	CompressibleBytes int                         `json:"compressible_bytes"`
	FixedBytes        int                         `json:"fixed_bytes"`
}

type PromptObservation struct {
	LoopName             string                      `json:"loop_name"`
	Nonce                string                      `json:"nonce"`
	GeneratedAt          time.Time                   `json:"generated_at"`
	PromptBytes          int                         `json:"prompt_bytes"`
	PromptTokens         int                         `json:"prompt_tokens"`
	PromptLines          int                         `json:"prompt_lines"`
	SectionCount         int                         `json:"section_count"`
	IncludedSectionCount int                         `json:"included_section_count"`
	Stats                PromptObservationStats      `json:"stats"`
	Sections             []*PromptSectionObservation `json:"sections"`
}

// PromptSectionStatus 是发往前端 (yakit "上下文成分" 面板 / EmitPromptProfile)
// 的单段状态, 是 PromptSectionObservation 的可序列化快照.
//
// 老字段 (key/label/role/included/can_compress/bytes/lines/summary/children)
// 保持向后兼容. 新增字段 (BytesPercent / EstimatedTokens / ContentHash /
// SummaryTruncated / RoleZh) 给前端展示提供"占比 / 成本估算 / 一致性指纹 /
// 是否被截断 / 中文角色名"五类信号, 方便用户判断哪些段在拖累命中率与体积.
//
// JSON 命名沿用 snake_case (与 grpcApi.ts AIContextSections 接口一致).
//
// 关键词: PromptSectionStatus, EmitPromptProfile, 上下文成分, bytes_percent,
// estimated_tokens, content_hash, summary_truncated, role_zh
type PromptSectionStatus struct {
	Key         string                 `json:"key"`
	Label       string                 `json:"label"`
	Role        PromptSectionRole      `json:"role"`
	RoleZh      PromptSectionRoleZH    `json:"role_zh"`
	Included    bool                   `json:"included"`
	CanCompress bool                   `json:"can_compress"`
	Bytes       int                    `json:"bytes"`
	Lines       int                    `json:"lines"`
	Summary     string                 `json:"summary,omitempty"`
	Children    []*PromptSectionStatus `json:"children,omitempty"`

	// BytesPercent 该段 (含 children) 字节占整 prompt 的百分比, 0-100, 保留两位小数.
	// 前端可直接用作进度条 / 排序依据, 帮用户找到"哪段最大".
	BytesPercent float64 `json:"bytes_percent,omitempty"`
	// EstimatedTokens 按 4 byte ≈ 1 token 估算的 token 数. 与上游真实 prompt_tokens
	// 不严格相等, 但同一 prompt 内段间相对量级可比, 可作为成本占比参考.
	EstimatedTokens int `json:"estimated_tokens,omitempty"`
	// ContentHash 本段 (含 children 渲染后) 内容的 sha1 前 8 字符 (16 hex chars).
	// 跨多次 prompt_profile 比对同 key 段的 hash 即可判断段内容是否抖动 -
	// 用于诊断 cache prefix 漂移 (同 key 不同 hash = prefix 不稳定).
	ContentHash string `json:"content_hash,omitempty"`
	// SummaryTruncated 仅当调用方显式给 BuildStatus 传正数 maxSummaryBytes,
	// 且段实际字节超过该上限时为 true. 默认 0 (无上限) 路径下 summary 全量,
	// 该字段恒为 false. 保留字段是为了向后兼容 + 未来可恢复显式截断场景.
	SummaryTruncated bool `json:"summary_truncated,omitempty"`
}

type PromptObservationStatus struct {
	LoopName             string                      `json:"loop_name"`
	Nonce                string                      `json:"nonce"`
	PromptBytes          int                         `json:"prompt_bytes"`
	PromptTokens         int                         `json:"prompt_tokens"`
	PromptLines          int                         `json:"prompt_lines"`
	SectionCount         int                         `json:"section_count"`
	IncludedSectionCount int                         `json:"included_section_count"`
	RoleStats            []PromptObservationRoleStat `json:"role_stats"`
	CompressibleBytes    int                         `json:"compressible_bytes"`
	FixedBytes           int                         `json:"fixed_bytes"`
	Sections             []*PromptSectionStatus      `json:"sections"`
}

func newPromptSectionObservation(
	key string,
	label string,
	role PromptSectionRole,
	compressible bool,
	content string,
) *PromptSectionObservation {
	section := &PromptSectionObservation{
		Key:          key,
		Label:        label,
		Role:         role,
		RoleZh:       promptSectionRoleZH(role),
		Compressible: compressible,
		Content:      content,
	}
	section.refreshMetrics()
	return section
}

func NewPromptSectionObservation(
	key string,
	label string,
	role PromptSectionRole,
	compressible bool,
	content string,
) *PromptSectionObservation {
	return newPromptSectionObservation(key, label, role, compressible, content)
}

func buildPromptObservation(loopName string, nonce string, prompt string, sections []*PromptSectionObservation) *PromptObservation {
	observation := &PromptObservation{
		LoopName:     loopName,
		Nonce:        nonce,
		GeneratedAt:  time.Now(),
		PromptBytes:  len(prompt),
		PromptTokens: ytoken.CalcTokenCount(prompt),
		PromptLines:  countPromptLines(prompt),
		Stats:        newPromptObservationStats(),
		Sections:     sections,
	}

	for _, section := range sections {
		observation.SectionCount += countPromptSections(section)
		collectPromptObservationStats(section, observation)
	}
	return observation
}

func BuildPromptObservation(loopName string, nonce string, prompt string, sections []*PromptSectionObservation) *PromptObservation {
	return buildPromptObservation(loopName, nonce, prompt, sections)
}

func countPromptSections(section *PromptSectionObservation) int {
	if section == nil {
		return 0
	}
	total := 1
	for _, child := range section.Children {
		total += countPromptSections(child)
	}
	return total
}

func collectPromptObservationStats(section *PromptSectionObservation, observation *PromptObservation) {
	if section == nil || observation == nil || !section.IsIncluded() {
		return
	}
	if len(section.Children) > 0 {
		for _, child := range section.Children {
			collectPromptObservationStats(child, observation)
		}
		return
	}
	observation.IncludedSectionCount++
	bytes := section.ContentBytes()
	observation.Stats.RoleStats = addPromptObservationRoleBytes(observation.Stats.RoleStats, section.Role, bytes)
	if section.Compressible {
		observation.Stats.CompressibleBytes += bytes
	} else {
		observation.Stats.FixedBytes += bytes
	}
}

func (s *PromptSectionObservation) IsIncluded() bool {
	if s == nil {
		return false
	}
	return s.Included
}

func (s *PromptSectionObservation) refreshMetrics() {
	if s == nil {
		return
	}
	s.Bytes = len(s.Content)
	s.Lines = countPromptLines(s.Content)
	s.Included = strings.TrimSpace(s.Content) != ""
	for _, child := range s.Children {
		if child == nil {
			continue
		}
		child.refreshMetrics()
		if child.Included {
			s.Included = true
		}
		s.Bytes += child.Bytes
		s.Lines += child.Lines
	}
}

func (s *PromptSectionObservation) ContentBytes() int {
	if s == nil {
		return 0
	}
	return s.Bytes
}

func (s *PromptSectionObservation) LineCount() int {
	if s == nil {
		return 0
	}
	return s.Lines
}

func countPromptLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func buildPromptSections(
	infos map[string]any,
	userInput string,
	persistent string,
	skillsContext string,
	reactiveData string,
	memory string,
	schema string,
	outputExample string,
	extraCapabilities string,
	sessionEvidence string,
) []*PromptSectionObservation {
	return []*PromptSectionObservation{
		buildBackgroundPromptSection(infos),
		newPromptSectionObservation(
			"user_query",
			"User Query",
			PromptSectionRoleDynamic,
			false,
			userInput,
		),
		newPromptSectionObservation(
			"extra_capabilities",
			"Extra Capabilities",
			PromptSectionRoleDynamic,
			true,
			extraCapabilities,
		),
		newPromptSectionObservation(
			"persistent_context",
			"Persistent Context",
			PromptSectionRoleHighStatic,
			false,
			persistent,
		),
		newPromptSectionObservation(
			"session_evidence",
			"Session Evidence",
			PromptSectionRoleTimelineOpen,
			true,
			sessionEvidence,
		),
		newPromptSectionObservation(
			"skills_context",
			"Skills Context",
			PromptSectionRoleSemiDynamic,
			true,
			skillsContext,
		),
		newPromptSectionObservation(
			"reactive_data",
			"Reactive Data",
			PromptSectionRoleDynamic,
			true,
			reactiveData,
		),
		newPromptSectionObservation(
			"injected_memory",
			"Injected Memory",
			PromptSectionRoleDynamic,
			true,
			memory,
		),
		newPromptSectionObservation(
			"schema",
			"Schema",
			PromptSectionRoleSemiDynamic,
			false,
			schema,
		),
		newPromptSectionObservation(
			"output_example",
			"Output Example",
			PromptSectionRoleHighStatic,
			true,
			outputExample,
		),
	}
}

func buildBackgroundPromptSection(infos map[string]any) *PromptSectionObservation {
	backgroundSection := newPromptContainerSection(
		"background",
		"Background",
		PromptSectionRoleTimelineOpen,
	)

	var children []*PromptSectionObservation
	if envContent := buildBackgroundEnvContent(infos); envContent != "" {
		children = append(children, newPromptSectionObservation(
			"background.environment",
			"Background / Environment",
			PromptSectionRoleTimelineOpen,
			true,
			envContent,
		))
	}
	if dynamicSection := buildBackgroundDynamicContextSection(infos); dynamicSection != nil {
		children = append(children, dynamicSection)
	}
	if aiForgeList := strings.TrimSpace(utils.InterfaceToString(infos["AIForgeList"])); aiForgeList != "" &&
		utils.InterfaceToBoolean(infos["AllowPlan"]) &&
		utils.InterfaceToBoolean(infos["ShowForgeList"]) {
		children = append(children, newPromptSectionObservation(
			"background.ai_forge_list",
			"Background / AI Forge List",
			PromptSectionRoleFrozenBlock,
			true,
			aiForgeList,
		))
	}
	if toolInventory := buildBackgroundToolInventoryContent(infos); toolInventory != "" {
		children = append(children, newPromptSectionObservation(
			"background.tool_inventory",
			"Background / Tool Inventory",
			PromptSectionRoleFrozenBlock,
			true,
			toolInventory,
		))
	}
	if timelineContent := buildBackgroundTimelineContent(infos); timelineContent != "" {
		children = append(children, newPromptSectionObservation(
			"background.timeline",
			"Background / Timeline",
			PromptSectionRoleTimelineOpen,
			true,
			timelineContent,
		))
	}
	backgroundSection.Children = children
	return finalizePromptContainerSection(backgroundSection)
}

func buildBackgroundEnvContent(infos map[string]any) string {
	var lines []string
	currentTime := strings.TrimSpace(utils.InterfaceToString(infos["CurrentTime"]))
	osArch := strings.TrimSpace(utils.InterfaceToString(infos["OSArch"]))
	if currentTime != "" || osArch != "" {
		lines = append(lines, fmt.Sprintf("Current Time: %s | OS/Arch: %s", currentTime, osArch))
	}
	if wd := strings.TrimSpace(utils.InterfaceToString(infos["WorkingDir"])); wd != "" {
		lines = append(lines, "working dir: "+wd)
	}
	if glance := strings.TrimSpace(utils.InterfaceToString(infos["WorkingDirGlance"])); glance != "" {
		lines = append(lines, "working dir glance: "+glance)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func buildBackgroundDynamicContextSection(infos map[string]any) *PromptSectionObservation {
	dynamicContext := strings.TrimSpace(utils.InterfaceToString(infos["DynamicContext"]))
	if dynamicContext == "" {
		return nil
	}
	section := newPromptContainerSection(
		"background.dynamic_context",
		"Background / Dynamic Context",
		PromptSectionRoleDynamic,
	)

	prevTagIdx := strings.Index(dynamicContext, "<|PREV_USER_INPUT_")
	autoProvided := strings.TrimSpace(dynamicContext)
	prevUserInput := ""
	if prevTagIdx >= 0 {
		autoProvided = strings.TrimSpace(dynamicContext[:prevTagIdx])
		prevUserInput = strings.TrimSpace(dynamicContext[prevTagIdx:])
	}

	var children []*PromptSectionObservation
	if autoProvided != "" {
		children = append(children, newPromptSectionObservation(
			"background.dynamic_context.auto_provided",
			"Background / Dynamic Context / Auto Provided",
			PromptSectionRoleDynamic,
			true,
			autoProvided,
		))
	}
	if prevUserInput != "" {
		children = append(children, newPromptSectionObservation(
			"background.dynamic_context.prev_user_input",
			"Background / Dynamic Context / Previous User Input",
			PromptSectionRoleTimelineOpen,
			false,
			prevUserInput,
		))
	}
	section.Children = children
	return finalizePromptContainerSection(section)
}

func buildBackgroundToolInventoryContent(infos map[string]any) string {
	if !utils.InterfaceToBoolean(infos["AllowToolCall"]) {
		return ""
	}
	topToolsRaw := infos["TopTools"]
	topTools, ok := topToolsRaw.([]*aitool.Tool)
	if !ok || len(topTools) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, fmt.Sprintf(
		"enabled_tools=%d top_tools=%d has_more=%v",
		utils.InterfaceToInt(infos["ToolsCount"]),
		utils.InterfaceToInt(infos["TopToolsCount"]),
		utils.InterfaceToBoolean(infos["HasMoreTools"]),
	))
	for _, tool := range topTools {
		if tool == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
	}
	return strings.Join(lines, "\n")
}

func buildBackgroundTimelineContent(infos map[string]any) string {
	timeline := strings.TrimSpace(utils.InterfaceToString(infos["Timeline"]))
	if timeline == "" {
		return ""
	}
	return "# Timeline Memory\n" + timeline
}

func newPromptContainerSection(
	key string,
	label string,
	role PromptSectionRole,
) *PromptSectionObservation {
	section := &PromptSectionObservation{
		Key:          key,
		Label:        label,
		Role:         role,
		RoleZh:       promptSectionRoleZH(role),
		Included:     false,
		Compressible: false,
		Bytes:        0,
		Lines:        0,
		Content:      "",
	}
	return section
}

func NewPromptContainerSection(
	key string,
	label string,
	role PromptSectionRole,
) *PromptSectionObservation {
	return newPromptContainerSection(key, label, role)
}

func finalizePromptContainerSection(section *PromptSectionObservation) *PromptSectionObservation {
	if section != nil {
		section.refreshMetrics()
	}
	return section
}

func FinalizePromptContainerSection(section *PromptSectionObservation) *PromptSectionObservation {
	return finalizePromptContainerSection(section)
}

func newPromptObservationStats() PromptObservationStats {
	return PromptObservationStats{
		RoleStats: newPromptObservationRoleStats(),
	}
}

func newPromptObservationRoleStats() []PromptObservationRoleStat {
	roles := promptSectionRolesInOrder()
	stats := make([]PromptObservationRoleStat, 0, len(roles))
	for _, role := range roles {
		stats = append(stats, PromptObservationRoleStat{
			RoleName:   role,
			RoleNameZh: promptSectionRoleZH(role),
			RoleBytes:  0,
		})
	}
	return stats
}

func clonePromptObservationRoleStats(stats []PromptObservationRoleStat) []PromptObservationRoleStat {
	if len(stats) == 0 {
		return nil
	}
	cloned := make([]PromptObservationRoleStat, len(stats))
	copy(cloned, stats)
	return cloned
}

func addPromptObservationRoleBytes(stats []PromptObservationRoleStat, role PromptSectionRole, bytes int) []PromptObservationRoleStat {
	if bytes == 0 {
		return stats
	}
	idx := promptObservationRoleStatIndex(stats, role)
	if idx >= 0 {
		stats[idx].RoleBytes += bytes
		return stats
	}
	return append(stats, PromptObservationRoleStat{
		RoleName:   role,
		RoleNameZh: promptSectionRoleZH(role),
		RoleBytes:  bytes,
	})
}

func promptObservationRoleStatIndex(stats []PromptObservationRoleStat, role PromptSectionRole) int {
	for i := range stats {
		if stats[i].RoleName == role {
			return i
		}
	}
	return -1
}

// RenderCLIReport renders a human-readable prompt observation report for logs.
// It favors readability over compactness so engineers can quickly inspect:
// 1. Overall prompt volume and role split.
// 2. The section hierarchy used to build the prompt.
// 3. Per-section metadata and a short wrapped summary preview.
func (o *PromptObservation) RenderCLIReport(maxPreviewBytes int) string {
	if o == nil {
		return ""
	}
	if maxPreviewBytes <= 0 {
		maxPreviewBytes = 96
	}

	var buf strings.Builder
	buf.WriteString("+---------------------------------------------------------------+\n")
	buf.WriteString(fmt.Sprintf("| Loop: %-56s |\n", shrinkCLIField(o.LoopName, 56)))
	buf.WriteString(fmt.Sprintf("| Nonce: %-55s |\n", shrinkCLIField(o.Nonce, 55)))
	buf.WriteString(fmt.Sprintf("| Prompt Bytes: %-48d |\n", o.PromptBytes))
	buf.WriteString(fmt.Sprintf("| Prompt Lines: %-48d |\n", o.PromptLines))
	buf.WriteString(fmt.Sprintf("| Sections: %-52d |\n", o.SectionCount))
	buf.WriteString(fmt.Sprintf("| Included Sections: %-43d |\n", o.IncludedSectionCount))
	buf.WriteString(fmt.Sprintf("| Fixed Bytes: %-49d |\n", o.Stats.FixedBytes))
	buf.WriteString(fmt.Sprintf("| Compressible Bytes: %-42d |\n", o.Stats.CompressibleBytes))
	buf.WriteString("+---------------------------------------------------------------+\n")
	buf.WriteString("| Role Summary                                                   |\n")
	for _, stat := range o.Stats.RoleStats {
		buf.WriteString(fmt.Sprintf("|   %-14s: %-45d |\n", stat.RoleName, stat.RoleBytes))
	}
	buf.WriteString("+---------------------------------------------------------------+\n")
	buf.WriteString("| Section Tree (label / key / meta / summary)                   |\n")
	buf.WriteString("+---------------------------------------------------------------+\n")
	for idx, section := range o.Sections {
		appendPromptSectionCLI(&buf, section, "", idx == len(o.Sections)-1, maxPreviewBytes)
	}
	return strings.TrimRight(buf.String(), "\n")
}

// BuildStatus 把 PromptObservation 转为可发往前端的 PromptObservationStatus.
//
// maxSummaryBytes 控制每段 Summary 的截断上限 (按字节, 头部前缀截断保留换行,
// 不压平空格). 传 <= 0 时用 defaultPromptSummaryBytes;
// 当前默认 = 0 = 不截断, 段内容完整透传给前端 "上下文成分" 面板.
// 仅当调用方明确出于带宽 / 渲染成本考虑想截断时才传正数.
//
// 关键词: BuildStatus, maxSummaryBytes, defaultPromptSummaryBytes,
// 上下文成分完整展示
func (o *PromptObservation) BuildStatus(maxSummaryBytes int) *PromptObservationStatus {
	if o == nil {
		return nil
	}
	if maxSummaryBytes <= 0 {
		maxSummaryBytes = defaultPromptSummaryBytes
	}
	status := &PromptObservationStatus{
		LoopName:             o.LoopName,
		Nonce:                o.Nonce,
		PromptBytes:          o.PromptBytes,
		PromptTokens:         o.PromptTokens,
		PromptLines:          o.PromptLines,
		SectionCount:         o.SectionCount,
		IncludedSectionCount: o.IncludedSectionCount,
		RoleStats:            clonePromptObservationRoleStats(o.Stats.RoleStats),
		CompressibleBytes:    o.Stats.CompressibleBytes,
		FixedBytes:           o.Stats.FixedBytes,
	}
	for _, section := range o.Sections {
		if item := buildPromptSectionStatus(section, maxSummaryBytes, o.PromptBytes); item != nil {
			status.Sections = append(status.Sections, item)
		}
	}
	return status
}

func buildPromptSectionStatus(section *PromptSectionObservation, maxSummaryBytes int, totalPromptBytes int) *PromptSectionStatus {
	if section == nil {
		return nil
	}
	bytesValue := section.ContentBytes()
	preview, truncated := previewSectionContent(section.Content, maxSummaryBytes)
	status := &PromptSectionStatus{
		Key:              section.Key,
		Label:            section.Label,
		Role:             section.Role,
		RoleZh:           section.RoleZh,
		Included:         section.IsIncluded(),
		CanCompress:      section.Compressible,
		Bytes:            bytesValue,
		Lines:            section.LineCount(),
		Summary:          preview,
		SummaryTruncated: truncated,
		BytesPercent:     bytesPercentOfTotal(bytesValue, totalPromptBytes),
		EstimatedTokens:  estimateTokensFromBytes(bytesValue),
		ContentHash:      contentHash8(section.Content),
	}
	for _, child := range section.Children {
		if childStatus := buildPromptSectionStatus(child, maxSummaryBytes, totalPromptBytes); childStatus != nil {
			status.Children = append(status.Children, childStatus)
		}
	}
	return status
}

// previewSectionContent 给前端 "上下文成分" 面板生成段内容预览.
//
// 对比老的 renderPromptSectionPreview:
//   - 不再把 \n / \r / \t 替换成空格, 也不再用 strings.Fields 压缩多空格,
//     原文换行 / 缩进全部保留, 让前端能用 monospace 直接 <pre> 出来.
//   - 不再 head + tail 截断, 而是头部前缀截断, 因为 prompt 段头部通常是
//     "## 标题" + "字段说明" + "首批样例", 头部信息密度最高.
//   - 截断时在末尾追加 "\n... (truncated, total N bytes)" 提示, 用户一眼能看到
//     完整段大小, 配合 SummaryTruncated 标志位可触发"查看完整内容"按钮.
//
// maxBytes <= 0 时直接返回 trim 后的原文.
//
// 关键词: previewSectionContent, summary 保留换行, head 截断, truncated 提示
func previewSectionContent(content string, maxBytes int) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", false
	}
	if maxBytes <= 0 || len(trimmed) <= maxBytes {
		return trimmed, false
	}
	// 头部截断, 在最后一个换行处对齐, 避免行被切两半;
	// 找不到合适换行就硬截.
	cut := maxBytes
	if newline := strings.LastIndexByte(trimmed[:maxBytes], '\n'); newline > maxBytes/2 {
		cut = newline
	}
	preview := trimmed[:cut]
	preview = strings.TrimRight(preview, "\n")
	preview += fmt.Sprintf("\n... (truncated, total %d bytes)", len(trimmed))
	return preview, true
}

// bytesPercentOfTotal 计算占总字节百分比, 保留两位小数, 0-100 区间.
// 关键词: bytesPercentOfTotal, BytesPercent
func bytesPercentOfTotal(bytes, total int) float64 {
	if total <= 0 || bytes <= 0 {
		return 0
	}
	pct := float64(bytes) * 100.0 / float64(total)
	if pct > 100 {
		pct = 100
	}
	// 保留两位小数, 避免 NaN / Inf
	return float64(int(pct*100+0.5)) / 100.0
}

// estimateTokensFromBytes 按 ASCII / 拉丁字符约 4 byte ≈ 1 token 的经验估算
// 给出段 token 量级. 不替代上游真实 token 计数, 只用于段间相对量级排序.
// 关键词: estimateTokensFromBytes, EstimatedTokens
func estimateTokensFromBytes(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	// 向上取整, 让 < 4 字节的段也至少给 1 token, 避免前端误以为 0 成本.
	return (bytes + 3) / 4
}

// contentHash8 取段原文 sha1 前 8 字符作为内容指纹, 给前端跨调用比对用.
// 同 key 不同 hash = 该段内容抖动 = 缓存 prefix 失稳, 用户可据此定位优化点.
// 空内容返回空串, 不浪费 16 字符传输.
// 关键词: contentHash8, ContentHash, cache prefix 抖动诊断
func contentHash8(content string) string {
	if content == "" {
		return ""
	}
	sum := sha1.Sum([]byte(content))
	return hex.EncodeToString(sum[:])[:8]
}

func appendPromptSectionCLI(buf *strings.Builder, section *PromptSectionObservation, prefix string, isLast bool, maxPreviewBytes int) {
	if buf == nil || section == nil {
		return
	}
	branch := "|-- "
	nextPrefix := prefix + "|   "
	if isLast {
		branch = "`-- "
		nextPrefix = prefix + "    "
	}

	buf.WriteString(prefix)
	buf.WriteString(branch)
	buf.WriteString(section.Label)
	buf.WriteString("\n")
	buf.WriteString(nextPrefix)
	buf.WriteString("key: ")
	buf.WriteString(section.Key)
	buf.WriteString("\n")
	buf.WriteString(nextPrefix)
	buf.WriteString(fmt.Sprintf("meta: role=%s, mode=%s, included=%s, size=%dB/%dL",
		sectionRoleLabel(section.Role),
		sectionCompressLabel(section.Compressible),
		sectionIncludeLabel(section.IsIncluded()),
		section.ContentBytes(),
		section.LineCount(),
	))
	buf.WriteString("\n")
	if preview := renderPromptSectionPreview(section.Content, maxPreviewBytes); preview != "" {
		appendWrappedCLIText(buf, nextPrefix, "summary: ", preview, 84)
	}

	for idx, child := range section.Children {
		appendPromptSectionCLI(buf, child, nextPrefix, idx == len(section.Children)-1, maxPreviewBytes)
	}
}

func renderPromptSectionPreview(content string, maxPreviewBytes int) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	content = strings.ReplaceAll(content, "\n", " \\n ")
	content = strings.Join(strings.Fields(content), " ")
	return utils.ShrinkString(content, maxPreviewBytes)
}

func sectionRoleLabel(role PromptSectionRole) string {
	switch role {
	case PromptSectionRoleHighStatic:
		return "high_static"
	case PromptSectionRoleFrozenBlock:
		return "frozen_block"
	case PromptSectionRoleSemiDynamic:
		return "semi_dynamic"
	case PromptSectionRoleSemiDynamic1:
		return "semi_dynamic_1"
	case PromptSectionRoleSemiDynamic2:
		return "semi_dynamic_2"
	case PromptSectionRoleTimelineOpen:
		return "timelineOpen"
	case PromptSectionRoleDynamic:
		return "dynamic"
	default:
		return "unknown"
	}
}

func promptSectionRoleZH(role PromptSectionRole) PromptSectionRoleZH {
	switch role {
	case PromptSectionRoleHighStatic:
		return PromptSectionRoleZHHighStatic
	case PromptSectionRoleFrozenBlock:
		return PromptSectionRoleZHFrozenBlock
	case PromptSectionRoleSemiDynamic:
		return PromptSectionRoleZHSemiDynamic
	case PromptSectionRoleSemiDynamic1:
		return PromptSectionRoleZHSemiDynamic1
	case PromptSectionRoleSemiDynamic2:
		return PromptSectionRoleZHSemiDynamic2
	case PromptSectionRoleTimelineOpen:
		return PromptSectionRoleZHTimelineOpen
	case PromptSectionRoleDynamic:
		return PromptSectionRoleZHDynamic
	default:
		return ""
	}
}

// promptSectionRolesInOrder 返回字节统计图与上下文成分面板按段排序展示用的
// Role 顺序. SemiDynamic1 / SemiDynamic2 紧跟在 FrozenBlock 之后, 顺序为
// 渲染物理顺序 (high_static -> frozen_block -> semi_dynamic_1 ->
// semi_dynamic_2 -> timeline_open -> dynamic). 老 PromptSectionRoleSemiDynamic
// 不在主路径出现, 排序到列表末尾兜底, 让老 caller 也能正确落位.
//
// 关键词: promptSectionRolesInOrder, 字节统计排序, P1.1 拆 SemiDynamic1/2
func promptSectionRolesInOrder() []PromptSectionRole {
	return []PromptSectionRole{
		PromptSectionRoleHighStatic,
		PromptSectionRoleFrozenBlock,
		PromptSectionRoleSemiDynamic1,
		PromptSectionRoleSemiDynamic2,
		PromptSectionRoleTimelineOpen,
		PromptSectionRoleDynamic,
		PromptSectionRoleSemiDynamic,
	}
}

func sectionCompressLabel(canCompress bool) string {
	if canCompress {
		return "compressible"
	}
	return "fixed"
}

func sectionIncludeLabel(included bool) string {
	if included {
		return "yes"
	}
	return "no"
}

func appendWrappedCLIText(buf *strings.Builder, prefix string, title string, content string, width int) {
	if buf == nil {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if width <= 0 {
		width = 84
	}

	linePrefix := prefix + title
	continuationPrefix := prefix + strings.Repeat(" ", len(title))
	remaining := content
	for remaining != "" {
		currentPrefix := continuationPrefix
		if linePrefix != "" {
			currentPrefix = linePrefix
		}

		limit := width - len(currentPrefix)
		if limit < 16 {
			limit = width
		}
		if len(remaining) <= limit {
			buf.WriteString(currentPrefix)
			buf.WriteString(remaining)
			buf.WriteString("\n")
			return
		}

		cut := strings.LastIndex(remaining[:limit], " ")
		if cut <= 0 {
			cut = limit
		}

		buf.WriteString(currentPrefix)
		buf.WriteString(strings.TrimSpace(remaining[:cut]))
		buf.WriteString("\n")
		remaining = strings.TrimSpace(remaining[cut:])
		linePrefix = ""
	}
}

func shrinkCLIField(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	return utils.ShrinkString(s, width)
}

func (r *ReActLoop) SetLastPromptObservation(observation *PromptObservation) {
	if r == nil {
		return
	}
	r.Set(lastPromptObservationLoopKey, observation)
}

func (r *ReActLoop) GetLastPromptObservation() *PromptObservation {
	if r == nil {
		return nil
	}
	observation, _ := r.GetVariable(lastPromptObservationLoopKey).(*PromptObservation)
	return observation
}

func (r *ReActLoop) SetLastPromptObservationStatus(status *PromptObservationStatus) {
	if r == nil {
		return
	}
	r.Set(lastPromptObservationStatusLoopKey, status)
}

func (r *ReActLoop) GetLastPromptObservationStatus() *PromptObservationStatus {
	if r == nil {
		return nil
	}
	status, _ := r.GetVariable(lastPromptObservationStatusLoopKey).(*PromptObservationStatus)
	return status
}

func (r *ReActLoop) emitPromptObservationStatus(status *PromptObservationStatus) {
	if r == nil || r.emitter == nil || status == nil {
		return
	}
	r.emitter.EmitPromptProfile(status)
}
