package reactloops

// 此文件负责把 Yak 专注模式 (Yak Focus Mode) 中的 metadata 类 __DUNDER__ 常量
// 转换为 LoopMetadataOption。这些常量是 boot 期就要解析的内容，会被注册到全局
// loopMetadata 表里供 ReAct 提示渲染使用。
//
// 关键词: yak focus mode metadata, dunder to LoopMetadataOption

// CollectFocusModeMetadataOptions 从 caller 中读取 metadata dunder 常量，
// 转换为 []LoopMetadataOption。defaultName 是当 __NAME__ 未声明时回退的名字
// （通常是文件名 trim 后缀）。
//
// 关键词: metadata extraction, dunder defaults
func CollectFocusModeMetadataOptions(
	caller *FocusModeYakHookCaller,
	defaultName string,
) (resolvedName string, opts []LoopMetadataOption) {
	if caller == nil {
		return defaultName, nil
	}

	resolvedName = caller.GetString(FocusDunder_Name)
	if resolvedName == "" {
		resolvedName = defaultName
	}

	if desc := caller.GetString(FocusDunder_Description); desc != "" {
		opts = append(opts, WithLoopDescription(desc))
	}
	if descZh := caller.GetString(FocusDunder_DescriptionZh); descZh != "" {
		opts = append(opts, WithLoopDescriptionZh(descZh))
	}
	if verbose := caller.GetString(FocusDunder_VerboseName); verbose != "" {
		opts = append(opts, WithVerboseName(verbose))
	}
	if verboseZh := caller.GetString(FocusDunder_VerboseNameZh); verboseZh != "" {
		opts = append(opts, WithVerboseNameZh(verboseZh))
	}
	if hidden, ok := caller.GetBool(FocusDunder_IsHidden); ok && hidden {
		opts = append(opts, WithLoopIsHidden(true))
	}
	if outputExample := caller.GetString(FocusDunder_OutputExample); outputExample != "" {
		opts = append(opts, WithLoopOutputExample(outputExample))
	}
	if usage := caller.GetString(FocusDunder_UsagePrompt); usage != "" {
		opts = append(opts, WithLoopUsagePrompt(usage))
	}
	return resolvedName, opts
}
