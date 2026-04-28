package reactloops

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 此文件负责将 Yak 专注模式中的"静态"配置 __DUNDER__ 常量
// 转换为 ReActLoopOption 列表。这些选项在 ReActLoop 创建时一次性注入，
// 不依赖运行期闭包。
//
// 关键词: yak focus mode static options, dunder to ReActLoopOption

// CollectFocusModeStaticOptions 读取 caller 中的全部静态 dunder 配置项，
// 转换为对应的 ReActLoopOption 列表。
//
// 关键词: static dunder extraction, ReActLoopOption mapping
func CollectFocusModeStaticOptions(caller *FocusModeYakHookCaller) []ReActLoopOption {
	if caller == nil {
		return nil
	}
	var opts []ReActLoopOption

	// ---- 数值型阈值 ----
	if v, ok := caller.GetInt(FocusDunder_MaxIterations); ok && v > 0 {
		opts = append(opts, WithMaxIterations(v))
	}
	if v, ok := caller.GetInt(FocusDunder_MemorySizeLimit); ok && v > 0 {
		opts = append(opts, WithMemorySizeLimit(v))
	}
	if v, ok := caller.GetInt(FocusDunder_PeriodicVerificationInterval); ok && v >= 0 {
		opts = append(opts, WithPeriodicVerificationInterval(v))
	}
	if v, ok := caller.GetInt(FocusDunder_SameActionTypeSpinThreshold); ok && v > 0 {
		opts = append(opts, WithSameActionTypeSpinThreshold(v))
	}
	if v, ok := caller.GetInt(FocusDunder_SameLogicSpinThreshold); ok && v > 0 {
		opts = append(opts, WithSameLogicSpinThreshold(v))
	}
	if v, ok := caller.GetInt(FocusDunder_MaxConsecutiveSpinWarnings); ok && v >= 0 {
		opts = append(opts, WithMaxConsecutiveSpinWarnings(v))
	}

	// ---- 静态布尔开关，仅当 dunder 显式声明时才覆盖默认值 ----
	if b, ok := caller.GetBool(FocusDunder_AllowRAG); ok {
		opts = append(opts, WithAllowRAG(b))
	}
	if b, ok := caller.GetBool(FocusDunder_AllowAIForge); ok {
		opts = append(opts, WithAllowAIForge(b))
	}
	if b, ok := caller.GetBool(FocusDunder_AllowPlanAndExec); ok {
		opts = append(opts, WithAllowPlanAndExec(b))
	}
	if b, ok := caller.GetBool(FocusDunder_AllowToolCall); ok {
		opts = append(opts, WithAllowToolCall(b))
	}
	if b, ok := caller.GetBool(FocusDunder_AllowUserInteract); ok {
		opts = append(opts, WithAllowUserInteract(b))
	}
	if b, ok := caller.GetBool(FocusDunder_UseSpeedPriorityAI); ok && b {
		opts = append(opts, WithUseSpeedPriorityAICallback(true))
	}
	if b, ok := caller.GetBool(FocusDunder_EnableSelfReflection); ok {
		opts = append(opts, WithEnableSelfReflection(b))
	}
	if b, ok := caller.GetBool(FocusDunder_DisableLoopPerception); ok && b {
		opts = append(opts, WithDisableLoopPerception(true))
	}
	if b, ok := caller.GetBool(FocusDunder_NoEndLoadingStatus); ok && b {
		opts = append(opts, WithNoEndLoadingStatus(true))
	}

	// ---- 静态 prompt 模板字符串 ----
	if s := caller.GetString(FocusDunder_PersistentInstruction); s != "" {
		opts = append(opts, WithPersistentInstruction(s))
	}
	if s := caller.GetString(FocusDunder_ReflectionOutputExample); s != "" {
		opts = append(opts, WithReflectionOutputExample(s))
	}
	if s := caller.GetString(FocusDunder_ToolCallIntervalReviewExtraPrompt); s != "" {
		opts = append(opts, WithToolCallIntervalReviewExtraPrompt(s))
	}

	// ---- __VARS__ -> WithVars ----
	if vars := caller.GetMap(FocusDunder_Vars); len(vars) > 0 {
		opts = append(opts, WithVars(vars))
	}

	// ---- __AI_TAG_FIELDS__ -> WithAITagField / WithAITagFieldWithAINodeId ----
	if list := caller.GetSlice(FocusDunder_AITagFields); len(list) > 0 {
		for _, item := range list {
			tag, varName, nodeID, contentType := parseAITagFieldEntry(item)
			if tag == "" || varName == "" {
				log.Warnf("yak focus mode: skip invalid __AI_TAG_FIELDS__ entry %v", item)
				continue
			}
			if nodeID != "" || contentType != "" {
				opts = append(opts, WithAITagFieldWithAINodeId(tag, varName, nodeID, contentType))
			} else {
				opts = append(opts, WithAITagField(tag, varName))
			}
		}
	}

	return opts
}

// parseAITagFieldEntry 把 yak 中 __AI_TAG_FIELDS__ 列表里的单条项解析出来。
// 支持两种形式：
//  1. dict {"tag": "...", "var": "...", "node_id": "...", "content_type": "..."}
//  2. dict 字段名为 lower / camel 不一致时也尝试容错
func parseAITagFieldEntry(item any) (tag, varName, nodeID, contentType string) {
	if utils.IsNil(item) {
		return
	}
	m := utils.InterfaceToMapInterface(item)
	if len(m) == 0 {
		return
	}
	tag = utils.MapGetString(m, "tag")
	if tag == "" {
		tag = utils.MapGetString(m, "TagName")
	}
	varName = utils.MapGetString(m, "var")
	if varName == "" {
		varName = utils.MapGetString(m, "variable_name")
	}
	if varName == "" {
		varName = utils.MapGetString(m, "VariableName")
	}
	nodeID = utils.MapGetString(m, "node_id")
	if nodeID == "" {
		nodeID = utils.MapGetString(m, "AINodeId")
	}
	contentType = utils.MapGetString(m, "content_type")
	if contentType == "" {
		contentType = utils.MapGetString(m, "ContentType")
	}
	return
}
