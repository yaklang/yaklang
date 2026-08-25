package yakgrpc

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/contextmenu"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/yakscript"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	contextMenuExecutionTimeout = 5 * time.Minute
	maxContextMenuFlowCount     = 200
	maxContextMenuFlowBytes     = 32 * 1024 * 1024
	maxContextMenuPacketBytes   = 16 * 1024 * 1024
)

func (s *Server) QueryContextMenuActions(ctx context.Context, req *ypb.QueryContextMenuActionsRequest) (*ypb.QueryContextMenuActionsResponse, error) {
	if req == nil {
		req = &ypb.QueryContextMenuActionsRequest{}
	}
	if req.GetScene() != "" && !contextmenu.IsKnownScene(req.GetScene()) {
		return nil, utils.Errorf("unknown context-menu scene: %s", req.GetScene())
	}

	bindings, err := yakit.QueryEffectiveContextMenuBindings(s.GetProfileDatabase(), req.GetScene())
	if err != nil {
		return nil, err
	}
	scripts, err := s.queryContextMenuScripts(req.GetIncludeDisabled(), bindings)
	if err != nil {
		return nil, err
	}
	bindingByKey := make(map[string]*schema.ContextMenuBinding, len(bindings))
	for _, binding := range bindings {
		bindingByKey[contextMenuBindingKey(binding.PluginUUID, binding.ActionID)] = binding
	}

	response := &ypb.QueryContextMenuActionsResponse{MaxCustomPluginCount: int64(contextmenu.MaxCustomPluginsPerScene)}
	enabledCustomPlugins := make(map[string]struct{})
	scriptByUUID := make(map[string]*schema.YakScript, len(scripts))
	for _, script := range scripts {
		scriptByUUID[script.Uuid] = script
	}
	for _, binding := range bindings {
		bindingScene, ok := contextmenu.SceneForAction(binding.ActionID)
		if !ok || req.GetScene() == "" || bindingScene != req.GetScene() {
			continue
		}
		script, exists := scriptByUUID[binding.PluginUUID]
		if exists && binding.Enabled && !script.IsCorePlugin && contextMenuScriptImplements(script, binding.ActionID) {
			enabledCustomPlugins[binding.PluginUUID] = struct{}{}
		}
	}
	for _, script := range scripts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		capabilities := contextMenuScriptActions(script)
		for _, actionID := range capabilities {
			scene, ok := contextmenu.SceneForAction(actionID)
			if !ok || (req.GetScene() != "" && req.GetScene() != scene) {
				continue
			}
			binding := bindingByKey[contextMenuBindingKey(script.Uuid, actionID)]
			action := contextMenuActionFrom(script, actionID, binding)
			if !action.Enabled && !req.GetIncludeDisabled() {
				continue
			}
			response.Actions = append(response.Actions, action)
		}
	}
	response.EnabledCustomPluginCount = int64(len(enabledCustomPlugins))
	sort.SliceStable(response.Actions, func(i, j int) bool {
		if response.Actions[i].Sort != response.Actions[j].Sort {
			return response.Actions[i].Sort < response.Actions[j].Sort
		}
		if response.Actions[i].PluginName != response.Actions[j].PluginName {
			return response.Actions[i].PluginName < response.Actions[j].PluginName
		}
		return response.Actions[i].ActionID < response.Actions[j].ActionID
	})
	return response, nil
}

func (s *Server) SetContextMenuActionBinding(_ context.Context, req *ypb.SetContextMenuActionBindingRequest) (*ypb.ContextMenuAction, error) {
	if req == nil {
		return nil, utils.Error("context-menu binding request is nil")
	}
	script, err := s.getContextMenuManagedScript(req.GetPluginUUID())
	if err != nil {
		return nil, err
	}
	if !contextMenuScriptImplements(script, req.GetActionID()) {
		return nil, utils.Errorf("plugin %s does not implement action %s", script.ScriptName, req.GetActionID())
	}

	resultMode := req.GetResultMode()
	if script.Type == contextmenu.LegacyPluginType {
		resultMode = contextmenu.ResultModeAuto
	}
	binding, err := yakit.SetContextMenuBinding(s.GetProfileDatabase(), &schema.ContextMenuBinding{
		PluginUUID:   script.Uuid,
		ActionID:     req.GetActionID(),
		Enabled:      req.GetEnabled(),
		Sort:         req.GetSort(),
		Shortcut:     strings.TrimSpace(req.GetShortcut()),
		ResultMode:   resultMode,
		AskBeforeRun: req.GetAskBeforeRun(),
	})
	if err != nil {
		return nil, err
	}
	return contextMenuActionFrom(script, req.GetActionID(), binding), nil
}

func (s *Server) ExecuteContextMenuAction(req *ypb.ExecuteContextMenuActionRequest, stream ypb.Yak_ExecuteContextMenuActionServer) error {
	if req == nil {
		return utils.Error("context-menu execution request is nil")
	}
	script, err := s.getContextMenuScript(req.GetPluginUUID())
	if err != nil {
		return err
	}
	actionID := req.GetActionID()
	hookName, ok := contextmenu.HookForAction(actionID)
	if !ok {
		return utils.Errorf("unknown context-menu action: %s", actionID)
	}
	if !contextMenuScriptImplements(script, actionID) {
		return utils.Errorf("plugin %s does not implement action %s", script.ScriptName, actionID)
	}

	binding, bindingErr := yakit.GetContextMenuBinding(s.GetProfileDatabase(), script.Uuid, actionID)
	if !script.IsCorePlugin && (bindingErr != nil || !binding.Enabled) {
		return utils.Errorf("context-menu action %s/%s is not enabled", script.Uuid, actionID)
	}
	resultMode := contextmenu.ResultModeAuto
	if bindingErr == nil {
		resultMode = contextmenu.NormalizeResultMode(binding.ResultMode)
	}

	execCtx, cancel := context.WithTimeout(stream.Context(), contextMenuExecutionTimeout)
	defer cancel()
	hookArgs, ctxOptions, err := s.buildContextMenuHookArgs(execCtx, script, req)
	if err != nil {
		return err
	}
	resolvedParams, resolvedParamMap, err := resolveContextMenuParams(script, req.GetParams())
	if err != nil {
		return err
	}
	ctxOptions.Params = resolvedParamMap
	runtimeID := uuid.NewString()
	ctxOptions.RuntimeID = runtimeID
	actionCtx := contextmenu.NewActionContext(execCtx, ctxOptions)
	start := &ypb.ContextMenuActionEvent{
		RuntimeID:  runtimeID,
		Status:     "started",
		ResultMode: resultMode,
		PluginName: script.ScriptName,
	}
	if err := stream.Send(start); err != nil {
		return err
	}

	execParams := make([]*ypb.KVPair, 0, len(resolvedParams))
	for _, param := range resolvedParams {
		execParams = append(execParams, &ypb.KVPair{Key: param.GetKey(), Value: param.GetValue()})
	}
	execStream := yakscript.NewFakeStream(execCtx, func(result *ypb.ExecResult) error {
		return stream.Send(&ypb.ContextMenuActionEvent{
			RuntimeID:  runtimeID,
			Status:     "data",
			ResultMode: resultMode,
			PluginName: script.ScriptName,
			Result:     result,
		})
	})

	callResult, execErr := yakscript.ExecContextMenuScript(
		script,
		hookName,
		actionCtx,
		hookArgs,
		execStream,
		execParams,
		runtimeID,
		s.GetProjectDatabase(),
	)
	if execErr != nil {
		status := "failed"
		if execCtx.Err() == context.DeadlineExceeded {
			status = "timeout"
		} else if execCtx.Err() != nil {
			status = "cancelled"
		}
		return stream.Send(&ypb.ContextMenuActionEvent{
			RuntimeID:  runtimeID,
			Status:     status,
			Reason:     execErr.Error(),
			ResultMode: resultMode,
			PluginName: script.ScriptName,
		})
	}

	if callResult != nil {
		if actionID == contextmenu.ActionHTTPPacket {
			packetResult, ok := callResult.(*contextmenu.PacketActionResult)
			if !ok {
				return stream.Send(&ypb.ContextMenuActionEvent{
					RuntimeID: runtimeID, Status: "failed", ResultMode: resultMode, PluginName: script.ScriptName,
					Reason: "handleHTTPPacket must return context.NewPacketResult(...) or nil",
				})
			}
			if len(packetResult.Request) > maxContextMenuPacketBytes || len(packetResult.Response) > maxContextMenuPacketBytes {
				return stream.Send(&ypb.ContextMenuActionEvent{
					RuntimeID: runtimeID, Status: "failed", ResultMode: resultMode, PluginName: script.ScriptName,
					Reason: "packet action result exceeds the 16 MiB packet limit",
				})
			}
			if err := stream.Send(&ypb.ContextMenuActionEvent{
				RuntimeID: runtimeID, Status: "packet-result", ResultMode: resultMode, PluginName: script.ScriptName,
				PacketResult: &ypb.ContextMenuPacketActionResult{
					Request:             packetResult.Request,
					Response:            packetResult.Response,
					ReplaceRequest:      packetResult.ReplaceRequest,
					ReplaceResponse:     packetResult.ReplaceResponse,
					RequireConfirmation: packetResult.RequireConfirmation,
					PacketRevision:      req.GetPacketRevision(),
				},
			}); err != nil {
				return err
			}
		}
	}

	return stream.Send(&ypb.ContextMenuActionEvent{
		RuntimeID: runtimeID, Status: "completed", ResultMode: resultMode, PluginName: script.ScriptName,
	})
}

func (s *Server) queryContextMenuScripts(includeDisabled bool, bindings []*schema.ContextMenuBinding) ([]*schema.YakScript, error) {
	var scripts []*schema.YakScript
	db := s.GetProfileDatabase().Model(&schema.YakScript{}).
		Where("type IN (?)", []string{contextmenu.PluginType, contextmenu.LegacyPluginType})
	if !includeDisabled {
		uuidSet := make(map[string]struct{})
		for _, binding := range bindings {
			if binding.Enabled {
				uuidSet[binding.PluginUUID] = struct{}{}
			}
		}
		pluginUUIDs := make([]string, 0, len(uuidSet))
		for pluginUUID := range uuidSet {
			pluginUUIDs = append(pluginUUIDs, pluginUUID)
		}
		if len(pluginUUIDs) == 0 {
			db = db.Where("is_core_plugin = ?", true)
		} else {
			db = db.Where("is_core_plugin = ? OR uuid IN (?)", true, pluginUUIDs)
		}
	}
	if result := db.Order("script_name asc").Find(&scripts); result.Error != nil {
		return nil, result.Error
	}
	for _, script := range scripts {
		if err := s.ensureContextMenuManagedScriptUUID(script); err != nil {
			return nil, err
		}
	}
	return scripts, nil
}

func (s *Server) getContextMenuScript(pluginUUID string) (*schema.YakScript, error) {
	script, err := s.getContextMenuManagedScript(pluginUUID)
	if err != nil {
		return nil, err
	}
	if script.Type != contextmenu.PluginType {
		return nil, utils.Errorf("plugin %s is not a context-menu plugin", pluginUUID)
	}
	return script, nil
}

func (s *Server) getContextMenuManagedScript(pluginUUID string) (*schema.YakScript, error) {
	pluginUUID = strings.TrimSpace(pluginUUID)
	if pluginUUID == "" {
		return nil, utils.Error("context-menu plugin UUID is empty")
	}
	script, err := yakit.GetYakScriptByUUID(s.GetProfileDatabase(), pluginUUID)
	if err != nil {
		return nil, err
	}
	if script.Type != contextmenu.PluginType && script.Type != contextmenu.LegacyPluginType {
		return nil, utils.Errorf("plugin %s is not managed by context-menu settings", pluginUUID)
	}
	return script, nil
}

func (s *Server) ensureContextMenuManagedScriptUUID(script *schema.YakScript) error {
	return yakit.EnsureContextMenuScriptUUID(s.GetProfileDatabase(), script)
}

func contextMenuScriptImplements(script *schema.YakScript, actionID string) bool {
	if script == nil || !contextmenu.IsKnownBindingAction(actionID) {
		return false
	}
	if script.Type == contextmenu.LegacyPluginType {
		return contextmenu.LegacyScriptImplements(script.Tags, actionID)
	}
	if script.Type != contextmenu.PluginType || !contextmenu.IsKnownAction(actionID) {
		return false
	}
	capabilities, err := static_analyzer.GetContextMenuCapabilities(script.Content)
	if err != nil {
		return false
	}
	for _, capability := range capabilities {
		if capability == actionID {
			return true
		}
	}
	return false
}

func contextMenuActionFrom(script *schema.YakScript, actionID string, binding *schema.ContextMenuBinding) *ypb.ContextMenuAction {
	hookName, _ := contextmenu.HookForAction(actionID)
	if hookName == "" {
		hookName = contextmenu.LegacyActionName(actionID)
	}
	scene, _ := contextmenu.SceneForAction(actionID)
	executionType, _ := contextmenu.ExecutionTypeForAction(actionID)
	action := &ypb.ContextMenuAction{
		PluginUUID:         script.Uuid,
		PluginName:         script.ScriptName,
		ActionID:           actionID,
		HookName:           hookName,
		ResultMode:         contextmenu.ResultModeAuto,
		Params:             script.GetParams(),
		Locked:             script.IsCorePlugin,
		IsCorePlugin:       script.IsCorePlugin,
		Scene:              scene,
		PluginType:         script.Type,
		ExecutionType:      executionType,
		Help:               script.Help,
		HeadImg:            script.HeadImg,
		SupportsResultMode: script.Type == contextmenu.PluginType,
		IsAIPlugin:         contextmenu.HasTag(script.Tags, "AI工具"),
	}
	if binding != nil {
		action.Enabled = binding.Enabled
		action.Sort = binding.Sort
		action.Shortcut = binding.Shortcut
		action.ResultMode = contextmenu.NormalizeResultMode(binding.ResultMode)
		action.AskBeforeRun = binding.AskBeforeRun
	}
	if script.Type == contextmenu.LegacyPluginType {
		action.ResultMode = contextmenu.ResultModeAuto
	}
	if script.IsCorePlugin {
		action.Enabled = true
	}
	return action
}

func contextMenuScriptActions(script *schema.YakScript) []string {
	if script == nil {
		return nil
	}
	if script.Type == contextmenu.LegacyPluginType {
		return contextmenu.LegacyActionsForTags(script.Tags)
	}
	if script.Type != contextmenu.PluginType {
		return nil
	}
	capabilities, err := static_analyzer.GetContextMenuCapabilities(script.Content)
	if err != nil {
		log.Warnf("inspect context-menu plugin %s failed: %v", script.ScriptName, err)
		return nil
	}
	return capabilities
}

func contextMenuBindingKey(pluginUUID, actionID string) string {
	return pluginUUID + "\x00" + actionID
}

func (s *Server) buildContextMenuHookArgs(
	ctx context.Context,
	script *schema.YakScript,
	req *ypb.ExecuteContextMenuActionRequest,
) ([]any, contextmenu.ActionContextOptions, error) {
	options := contextmenu.ActionContextOptions{
		Scene:      req.GetActionID(),
		Source:     strings.TrimSpace(req.GetSource()),
		Trigger:    strings.TrimSpace(req.GetTrigger()),
		PluginUUID: script.Uuid,
		PluginName: script.ScriptName,
		ActionID:   req.GetActionID(),
	}
	if options.Source == "" {
		options.Source = req.GetActionID()
	}
	if options.Trigger == "" {
		options.Trigger = "context-menu"
	}

	switch req.GetActionID() {
	case contextmenu.ActionHistorySingle:
		if len(req.GetHTTPFlowIDs()) != 1 {
			return nil, options, utils.Error("history-single requires exactly one HTTP flow")
		}
		flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), req.GetHTTPFlowIDs()[0])
		if err != nil {
			return nil, options, err
		}
		fillContextMenuFlowOptions(&options, []*schema.HTTPFlow{flow})
		if options.RequestSize+options.ResponseSize > maxContextMenuFlowBytes {
			return nil, options, utils.Errorf("selected HTTP flow data is limited to %d bytes", maxContextMenuFlowBytes)
		}
		return []any{flow}, options, nil

	case contextmenu.ActionHistoryMulti:
		if len(req.GetHTTPFlowIDs()) < 2 {
			return nil, options, utils.Error("history-multi requires at least two HTTP flows")
		}
		if len(req.GetHTTPFlowIDs()) > maxContextMenuFlowCount {
			return nil, options, utils.Errorf("history-multi accepts at most %d HTTP flows", maxContextMenuFlowCount)
		}
		flows := make([]*schema.HTTPFlow, 0, len(req.GetHTTPFlowIDs()))
		var totalBytes int64
		for _, flowID := range req.GetHTTPFlowIDs() {
			select {
			case <-ctx.Done():
				return nil, options, ctx.Err()
			default:
			}
			flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), flowID)
			if err != nil {
				return nil, options, err
			}
			flows = append(flows, flow)
			totalBytes += int64(len(flow.Request) + len(flow.Response))
			if totalBytes > maxContextMenuFlowBytes {
				return nil, options, utils.Errorf("selected HTTP flow data is limited to %d bytes", maxContextMenuFlowBytes)
			}
		}
		fillContextMenuFlowOptions(&options, flows)
		return []any{flows}, options, nil

	case contextmenu.ActionHTTPPacket:
		if !req.GetHasRequest() && !req.GetHasResponse() {
			return nil, options, utils.Error("http-packet requires a request or response")
		}
		if len(req.GetRequest()) > maxContextMenuPacketBytes || len(req.GetResponse()) > maxContextMenuPacketBytes {
			return nil, options, utils.Errorf("each HTTP packet is limited to %d bytes", maxContextMenuPacketBytes)
		}
		options.HttpsState = contextmenu.NormalizeHttpsState(contextmenu.HttpsState(req.GetHttpsState()))
		options.HasRequest = req.GetHasRequest()
		options.HasResponse = req.GetHasResponse()
		options.RequestSize = int64(len(req.GetRequest()))
		options.ResponseSize = int64(len(req.GetResponse()))
		options.SelectionCount = 1
		return []any{req.GetRequest(), req.GetResponse()}, options, nil
	default:
		return nil, options, utils.Errorf("unknown context-menu action: %s", req.GetActionID())
	}
}

func resolveContextMenuParams(script *schema.YakScript, submitted []*ypb.ExecParamItem) ([]*ypb.ExecParamItem, map[string]any, error) {
	values := make(map[string]string, len(submitted))
	order := make([]string, 0, len(submitted))
	for _, param := range submitted {
		key := strings.TrimSpace(param.GetKey())
		if key == "" {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = param.GetValue()
	}

	if script != nil {
		for _, definition := range script.GetParams() {
			key := strings.TrimSpace(definition.GetField())
			if key == "" {
				continue
			}
			if _, exists := values[key]; exists {
				continue
			}
			if definition.GetRequired() && definition.GetDefaultValue() == "" {
				return nil, nil, utils.Errorf("required context-menu parameter is missing: %s", key)
			}
			values[key] = definition.GetDefaultValue()
			order = append(order, key)
		}
	}

	params := make([]*ypb.ExecParamItem, 0, len(order))
	paramMap := make(map[string]any, len(order))
	for _, key := range order {
		value := values[key]
		params = append(params, &ypb.ExecParamItem{Key: key, Value: value})
		paramMap[key] = value
	}
	return params, paramMap, nil
}

func fillContextMenuFlowOptions(options *contextmenu.ActionContextOptions, flows []*schema.HTTPFlow) {
	if options == nil {
		return
	}
	options.SelectionCount = len(flows)
	httpsCount := 0
	for _, flow := range flows {
		if flow == nil {
			continue
		}
		if flow.IsHTTPS {
			httpsCount++
		}
		if len(flow.Request) > 0 {
			options.HasRequest = true
		}
		if len(flow.Response) > 0 {
			options.HasResponse = true
		}
		options.RequestSize += int64(len(flow.Request))
		options.ResponseSize += int64(len(flow.Response))
	}
	switch {
	case len(flows) == 0:
		options.HttpsState = contextmenu.HttpsStateUnknown
	case httpsCount == 0:
		options.HttpsState = contextmenu.HttpsStateHTTP
	case httpsCount == len(flows):
		options.HttpsState = contextmenu.HttpsStateHTTPS
	default:
		options.HttpsState = contextmenu.HttpsStateMixed
	}
}
