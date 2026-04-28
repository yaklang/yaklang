package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// 此文件负责把 yak 专注模式中 __ACTIONS__ / __OVERRIDE_ACTIONS__ /
// __ACTIONS_FROM_TOOLS__ / __ACTIONS_FROM_LOOPS__ 列表，
// 解析并桥接成对应的 ReActLoopOption。verifier / handler 是嵌在 dict 中的
// yak 闭包，通过 FocusModeYakHookCaller.CallFunction 调用，确保 ctx 超时与 panic 捕获。
//
// 关键词: yak focus mode actions, custom action registration, override action

// CollectFocusModeActionOptions 解析 caller 中所有 action 相关 dunder 列表，
// 返回对应的 ReActLoopOption 列表。toolLookup 用于把 __ACTIONS_FROM_TOOLS__ 中
// 的工具名解析为真正的 *aitool.Tool 实例（一般由 invoker / 上层 lookup 提供）。
//
// 关键词: action options collection, tool lookup adapter
func CollectFocusModeActionOptions(
	caller *FocusModeYakHookCaller,
	toolLookup func(name string) *aitool.Tool,
) []ReActLoopOption {
	if caller == nil {
		return nil
	}
	var opts []ReActLoopOption

	// __ACTIONS__: 自定义 Action 列表
	for _, item := range caller.GetSlice(FocusDunder_Actions) {
		if opt := buildActionOptionFromDict(caller, item, false); opt != nil {
			opts = append(opts, opt)
		}
	}

	// __OVERRIDE_ACTIONS__: 替换内置/已有同名 Action
	for _, item := range caller.GetSlice(FocusDunder_OverrideActions) {
		if opt := buildActionOptionFromDict(caller, item, true); opt != nil {
			opts = append(opts, opt)
		}
	}

	// __ACTIONS_FROM_TOOLS__: 字符串列表，每个元素是工具名，由 toolLookup 解析
	if toolLookup != nil {
		for _, item := range caller.GetSlice(FocusDunder_ActionsFromTools) {
			toolName := utils.InterfaceToString(item)
			if toolName == "" {
				continue
			}
			tool := toolLookup(toolName)
			if tool == nil {
				log.Warnf("yak focus mode: tool %q not found via lookup, skip", toolName)
				continue
			}
			opts = append(opts, WithRegisterLoopActionFromTool(tool))
		}
	}

	// __ACTIONS_FROM_LOOPS__: 字符串列表，每个元素是已注册 loop 的名字
	for _, item := range caller.GetSlice(FocusDunder_ActionsFromLoops) {
		loopName := utils.InterfaceToString(item)
		if loopName == "" {
			continue
		}
		opts = append(opts, WithActionFactoryFromLoop(loopName))
	}

	return opts
}

// buildActionOptionFromDict 把单条 yak dict 形式的 action 描述构造成
// ReActLoopOption。override=true 时使用 WithOverrideLoopAction（替换已有），
// 否则使用 WithRegisterLoopActionWithStreamField（新建注册）。
//
// 关键词: action dict to option, verifier closure, handler closure
func buildActionOptionFromDict(caller *FocusModeYakHookCaller, raw any, override bool) ReActLoopOption {
	entry := utils.InterfaceToMapInterface(raw)
	if len(entry) == 0 {
		log.Warnf("yak focus mode: action entry is not a dict: %v", raw)
		return nil
	}

	actionType := utils.MapGetString(entry, "type")
	if actionType == "" {
		actionType = utils.MapGetString(entry, "action_type")
	}
	if actionType == "" {
		actionType = utils.MapGetString(entry, "name")
	}
	if actionType == "" {
		log.Warnf("yak focus mode: action entry missing 'type', skip: %v", entry)
		return nil
	}

	description := utils.MapGetString(entry, "description")
	if description == "" {
		description = utils.MapGetString(entry, "desc")
	}

	asyncMode := utils.MapGetBool(entry, "async")
	outputExamples := utils.MapGetString(entry, "output_examples")

	// options 列表
	var optionList []aitool.ToolOption
	if optsRaw := utils.MapGetRaw(entry, "options"); !utils.IsNil(optsRaw) {
		optionList = ParseFocusModeActionOptions(utils.InterfaceToSliceInterface(optsRaw))
	}

	// stream_fields 列表
	var streamFields []*LoopStreamField
	if sfRaw := utils.MapGetRaw(entry, "stream_fields"); !utils.IsNil(sfRaw) {
		for _, sfItem := range utils.InterfaceToSliceInterface(sfRaw) {
			sfMap := utils.InterfaceToMapInterface(sfItem)
			if len(sfMap) == 0 {
				continue
			}
			streamFields = append(streamFields, &LoopStreamField{
				FieldName:   utils.MapGetString(sfMap, "field"),
				AINodeId:    utils.MapGetString(sfMap, "node_id"),
				Prefix:      utils.MapGetString(sfMap, "prefix"),
				ContentType: utils.MapGetString(sfMap, "content_type"),
			})
		}
	}

	// 提取 verifier / handler yak 闭包
	verifierFn := pickFunction(entry, "verifier")
	handlerFn := pickFunction(entry, "handler")

	if handlerFn == nil {
		log.Warnf("yak focus mode: action %q missing 'handler' function, skip", actionType)
		return nil
	}

	// 构造 verifier wrapper（可选）
	var verifier LoopActionVerifierFunc
	if verifierFn != nil {
		verifier = func(loop *ReActLoop, action *aicommon.Action) error {
			ret, err := caller.CallFunction("verifier:"+actionType, verifierFn, loop, action)
			if err != nil {
				return err
			}
			if utils.IsNil(ret) {
				return nil
			}
			if errVal, ok := ret.(error); ok {
				return errVal
			}
			s := utils.InterfaceToString(ret)
			if s != "" && s != "<nil>" {
				return utils.Error(s)
			}
			return nil
		}
	}

	handler := func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		_, err := caller.CallFunction("handler:"+actionType, handlerFn, loop, action, operator)
		if err != nil {
			log.Errorf("yak focus mode: action %q handler failed: %v", actionType, err)
			operator.Fail(err)
		}
	}

	if override {
		// 直接构造 LoopAction 然后 WithOverrideLoopAction 覆盖
		loopAction := &LoopAction{
			AsyncMode:      asyncMode,
			ActionType:     actionType,
			Description:    description,
			Options:        optionList,
			ActionVerifier: verifier,
			ActionHandler:  handler,
			StreamFields:   streamFields,
			OutputExamples: outputExamples,
		}
		return WithOverrideLoopAction(loopAction)
	}

	// 普通注册：使用 WithRegisterLoopActionWithStreamField 走完整路径
	if asyncMode {
		log.Infof("yak focus mode: action %q registered as async mode", actionType)
	}
	opt := WithRegisterLoopActionWithStreamField(actionType, description, optionList, streamFields, verifier, handler)
	if asyncMode || outputExamples != "" {
		// 包装一层用于额外补丁 AsyncMode / OutputExamples
		return func(r *ReActLoop) {
			opt(r)
			if a, ok := r.actions.Get(actionType); ok && a != nil {
				if asyncMode {
					a.AsyncMode = true
				}
				if outputExamples != "" {
					a.OutputExamples = outputExamples
				}
			}
		}
	}
	return opt
}

// pickFunction 从 dict 中尝试多个 key 提取 *yakvm.Function
func pickFunction(entry map[string]any, keys ...string) *yakvm.Function {
	for _, k := range keys {
		raw, ok := entry[k]
		if !ok {
			continue
		}
		if fn, ok := raw.(*yakvm.Function); ok {
			return fn
		}
	}
	return nil
}
