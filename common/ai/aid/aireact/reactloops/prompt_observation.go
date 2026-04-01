package reactloops

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const lastPromptObservationLoopKey = "last_ai_decision_prompt_observation"
const lastPromptObservationStatusLoopKey = "last_ai_decision_prompt_observation_status"
const ReActPromptObservationStatusKey = "re-act-prompt-observation-status"

type PromptSectionRole string

const (
	PromptSectionRoleSystemPrompt PromptSectionRole = "system_prompt"
	PromptSectionRoleRuntimeCtx   PromptSectionRole = "runtime_context"
	PromptSectionRoleUserInput    PromptSectionRole = "user_input"
	PromptSectionRoleMixed        PromptSectionRole = "mixed"
)

type PromptSectionObservation struct {
	Key          string                      `json:"key"`
	Label        string                      `json:"label"`
	Role         PromptSectionRole           `json:"role"`
	Included     bool                        `json:"included"`
	Compressible bool                        `json:"compressible"`
	Bytes        int                         `json:"bytes"`
	Lines        int                         `json:"lines"`
	Content      string                      `json:"content,omitempty"`
	Children     []*PromptSectionObservation `json:"children,omitempty"`
}

type PromptObservationStats struct {
	SystemPromptBytes int `json:"system_prompt_bytes"`
	RuntimeCtxBytes   int `json:"runtime_context_bytes"`
	UserInputBytes    int `json:"user_input_bytes"`
	MixedBytes        int `json:"mixed_bytes"`
	CompressibleBytes int `json:"compressible_bytes"`
	FixedBytes        int `json:"fixed_bytes"`
}

type PromptObservation struct {
	LoopName             string                      `json:"loop_name"`
	Nonce                string                      `json:"nonce"`
	GeneratedAt          time.Time                   `json:"generated_at"`
	PromptBytes          int                         `json:"prompt_bytes"`
	PromptLines          int                         `json:"prompt_lines"`
	SectionCount         int                         `json:"section_count"`
	IncludedSectionCount int                         `json:"included_section_count"`
	Stats                PromptObservationStats      `json:"stats"`
	Sections             []*PromptSectionObservation `json:"sections"`
}

type PromptSectionStatus struct {
	Key         string                 `json:"key"`
	Label       string                 `json:"label"`
	Role        PromptSectionRole      `json:"role"`
	Included    bool                   `json:"included"`
	CanCompress bool                   `json:"can_compress"`
	Bytes       int                    `json:"bytes"`
	Lines       int                    `json:"lines"`
	Summary     string                 `json:"summary,omitempty"`
	Children    []*PromptSectionStatus `json:"children,omitempty"`
}

type PromptObservationStatus struct {
	LoopName             string                 `json:"loop_name"`
	Nonce                string                 `json:"nonce"`
	PromptBytes          int                    `json:"prompt_bytes"`
	PromptLines          int                    `json:"prompt_lines"`
	SectionCount         int                    `json:"section_count"`
	IncludedSectionCount int                    `json:"included_section_count"`
	SystemPromptBytes    int                    `json:"system_prompt_bytes"`
	RuntimeCtxBytes      int                    `json:"runtime_context_bytes"`
	UserInputBytes       int                    `json:"user_input_bytes"`
	CompressibleBytes    int                    `json:"compressible_bytes"`
	FixedBytes           int                    `json:"fixed_bytes"`
	Sections             []*PromptSectionStatus `json:"sections"`
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
		Compressible: compressible,
		Content:      content,
	}
	section.refreshMetrics()
	return section
}

func buildPromptObservation(loopName string, nonce string, prompt string, sections []*PromptSectionObservation) *PromptObservation {
	observation := &PromptObservation{
		LoopName:    loopName,
		Nonce:       nonce,
		GeneratedAt: time.Now(),
		PromptBytes: len(prompt),
		PromptLines: countPromptLines(prompt),
		Sections:    sections,
	}

	for _, section := range sections {
		observation.SectionCount += countPromptSections(section)
		collectPromptObservationStats(section, observation)
	}
	return observation
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
	switch section.Role {
	case PromptSectionRoleSystemPrompt:
		observation.Stats.SystemPromptBytes += bytes
	case PromptSectionRoleRuntimeCtx:
		observation.Stats.RuntimeCtxBytes += bytes
	case PromptSectionRoleUserInput:
		observation.Stats.UserInputBytes += bytes
	case PromptSectionRoleMixed:
		observation.Stats.MixedBytes += bytes
	}
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
) []*PromptSectionObservation {
	return []*PromptSectionObservation{
		buildBackgroundPromptSection(infos),
		newPromptSectionObservation(
			"user_query",
			"User Query",
			PromptSectionRoleUserInput,
			false,
			userInput,
		),
		newPromptSectionObservation(
			"extra_capabilities",
			"Extra Capabilities",
			PromptSectionRoleRuntimeCtx,
			true,
			extraCapabilities,
		),
		newPromptSectionObservation(
			"persistent_context",
			"Persistent Context",
			PromptSectionRoleSystemPrompt,
			false,
			persistent,
		),
		newPromptSectionObservation(
			"skills_context",
			"Skills Context",
			PromptSectionRoleRuntimeCtx,
			true,
			skillsContext,
		),
		newPromptSectionObservation(
			"reactive_data",
			"Reactive Data",
			PromptSectionRoleRuntimeCtx,
			true,
			reactiveData,
		),
		newPromptSectionObservation(
			"injected_memory",
			"Injected Memory",
			PromptSectionRoleRuntimeCtx,
			true,
			memory,
		),
		newPromptSectionObservation(
			"schema",
			"Schema",
			PromptSectionRoleSystemPrompt,
			false,
			schema,
		),
		newPromptSectionObservation(
			"output_example",
			"Output Example",
			PromptSectionRoleSystemPrompt,
			true,
			outputExample,
		),
	}
}

func buildBackgroundPromptSection(infos map[string]any) *PromptSectionObservation {
	backgroundSection := newPromptContainerSection(
		"background",
		"Background",
		PromptSectionRoleMixed,
	)

	var children []*PromptSectionObservation
	if envContent := buildBackgroundEnvContent(infos); envContent != "" {
		children = append(children, newPromptSectionObservation(
			"background.environment",
			"Background / Environment",
			PromptSectionRoleRuntimeCtx,
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
			PromptSectionRoleRuntimeCtx,
			true,
			aiForgeList,
		))
	}
	if toolInventory := buildBackgroundToolInventoryContent(infos); toolInventory != "" {
		children = append(children, newPromptSectionObservation(
			"background.tool_inventory",
			"Background / Tool Inventory",
			PromptSectionRoleRuntimeCtx,
			true,
			toolInventory,
		))
	}
	if timelineContent := buildBackgroundTimelineContent(infos); timelineContent != "" {
		children = append(children, newPromptSectionObservation(
			"background.timeline",
			"Background / Timeline",
			PromptSectionRoleRuntimeCtx,
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
		PromptSectionRoleMixed,
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
			PromptSectionRoleRuntimeCtx,
			true,
			autoProvided,
		))
	}
	if prevUserInput != "" {
		children = append(children, newPromptSectionObservation(
			"background.dynamic_context.prev_user_input",
			"Background / Dynamic Context / Previous User Input",
			PromptSectionRoleUserInput,
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
		Included:     false,
		Compressible: false,
		Bytes:        0,
		Lines:        0,
		Content:      "",
	}
	return section
}

func finalizePromptContainerSection(section *PromptSectionObservation) *PromptSectionObservation {
	if section != nil {
		section.refreshMetrics()
	}
	return section
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
	buf.WriteString(fmt.Sprintf("|   SYSTEM: %-52d |\n", o.Stats.SystemPromptBytes))
	buf.WriteString(fmt.Sprintf("|   CTX   : %-52d |\n", o.Stats.RuntimeCtxBytes))
	buf.WriteString(fmt.Sprintf("|   USER  : %-52d |\n", o.Stats.UserInputBytes))
	buf.WriteString(fmt.Sprintf("|   MIXED : %-52d |\n", o.Stats.MixedBytes))
	buf.WriteString("+---------------------------------------------------------------+\n")
	buf.WriteString("| Section Tree (label / key / meta / summary)                   |\n")
	buf.WriteString("+---------------------------------------------------------------+\n")
	for idx, section := range o.Sections {
		appendPromptSectionCLI(&buf, section, "", idx == len(o.Sections)-1, maxPreviewBytes)
	}
	return strings.TrimRight(buf.String(), "\n")
}

func (o *PromptObservation) BuildStatus(maxSummaryBytes int) *PromptObservationStatus {
	if o == nil {
		return nil
	}
	if maxSummaryBytes <= 0 {
		maxSummaryBytes = 120
	}
	status := &PromptObservationStatus{
		LoopName:             o.LoopName,
		Nonce:                o.Nonce,
		PromptBytes:          o.PromptBytes,
		PromptLines:          o.PromptLines,
		SectionCount:         o.SectionCount,
		IncludedSectionCount: o.IncludedSectionCount,
		SystemPromptBytes:    o.Stats.SystemPromptBytes,
		RuntimeCtxBytes:      o.Stats.RuntimeCtxBytes,
		UserInputBytes:       o.Stats.UserInputBytes,
		CompressibleBytes:    o.Stats.CompressibleBytes,
		FixedBytes:           o.Stats.FixedBytes,
	}
	for _, section := range o.Sections {
		if item := buildPromptSectionStatus(section, maxSummaryBytes); item != nil {
			status.Sections = append(status.Sections, item)
		}
	}
	return status
}

func buildPromptSectionStatus(section *PromptSectionObservation, maxSummaryBytes int) *PromptSectionStatus {
	if section == nil {
		return nil
	}
	status := &PromptSectionStatus{
		Key:         section.Key,
		Label:       section.Label,
		Role:        section.Role,
		Included:    section.IsIncluded(),
		CanCompress: section.Compressible,
		Bytes:       section.ContentBytes(),
		Lines:       section.LineCount(),
		Summary:     renderPromptSectionPreview(section.Content, maxSummaryBytes),
	}
	for _, child := range section.Children {
		if childStatus := buildPromptSectionStatus(child, maxSummaryBytes); childStatus != nil {
			status.Children = append(status.Children, childStatus)
		}
	}
	return status
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
	case PromptSectionRoleSystemPrompt:
		return "system_prompt"
	case PromptSectionRoleRuntimeCtx:
		return "runtime_context"
	case PromptSectionRoleUserInput:
		return "user_input"
	case PromptSectionRoleMixed:
		return "mixed"
	default:
		return "unknown"
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
