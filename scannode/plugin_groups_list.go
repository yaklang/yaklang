package scannode

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

// 端口扫描管理器 (hook.NewMixPluginCaller) 仅能加载这些类型的插件；
// mitm/yak/codec 插件对端口扫描流程无效，统计时单列 compatible_total。
var portScanCompatiblePluginTypes = map[string]struct{}{
	"port-scan": {},
	"nuclei":    {},
	"nasl":      {},
}

func isPortScanCompatiblePluginType(pluginType string) bool {
	_, ok := portScanCompatiblePluginTypes[strings.ToLower(strings.TrimSpace(pluginType))]
	return ok
}

func (b *legionJobBridge) handlePluginGroupsList(ctx context.Context, raw []byte) error {
	var command pluginv1.ListPluginGroupsCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal plugin groups list command: %w", err)
	}

	currentNodeID := b.agent.node.CurrentNodeID()
	if err := validatePluginGroupsListCommand(currentNodeID, &command); err != nil {
		return b.publishPluginGroupsResult(ctx, command.GetMetadata().GetCommandId(), pluginGroupsListErrorResult("invalid_plugin_groups_command", err.Error()))
	}

	result, err := queryLocalPluginGroups()
	if err != nil {
		log.Errorf("plugin groups list failed: node_id=%s command_id=%s err=%v",
			currentNodeID, command.GetMetadata().GetCommandId(), err)
		result = pluginGroupsListErrorResult("plugin_groups_query_failed", err.Error())
	}
	return b.publishPluginGroupsResult(ctx, command.GetMetadata().GetCommandId(), result)
}

func validatePluginGroupsListCommand(nodeID string, command *pluginv1.ListPluginGroupsCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("plugin groups list metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("plugin groups list command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("plugin groups list target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != strings.TrimSpace(nodeID):
		return fmt.Errorf("plugin groups list target_node_id mismatch: %s", command.GetTargetNodeId())
	default:
		return nil
	}
}

type pluginGroupMembership struct {
	Group      string
	ScriptName string
	ScriptType string
}

func queryLocalPluginGroups() (*pluginv1.ListPluginGroupsResult, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, fmt.Errorf("profile database is not initialized")
	}

	var rows []pluginGroupMembership
	if err := db.Table("plugin_groups").
		Select("plugin_groups.`group` AS `group`, yak_scripts.script_name AS script_name, yak_scripts.type AS script_type").
		Joins("INNER JOIN yak_scripts ON yak_scripts.script_name = plugin_groups.yak_script_name AND yak_scripts.deleted_at IS NULL").
		Where("plugin_groups.deleted_at IS NULL").
		Where("plugin_groups.is_poc_built_in = ?", false).
		Where("plugin_groups.temporary_id = ?", "").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query plugin groups: %w", err)
	}

	members := make(map[string][]pluginGroupMembership, len(rows))
	for _, row := range rows {
		group := strings.TrimSpace(row.Group)
		if group == "" || strings.HasPrefix(group, "mcp-group-") {
			continue
		}
		members[group] = append(members[group], pluginGroupMembership{
			ScriptName: strings.TrimSpace(row.ScriptName),
			ScriptType: strings.TrimSpace(row.ScriptType),
		})
	}

	result := &pluginv1.ListPluginGroupsResult{}
	for group, items := range members {
		compatible := 0
		scripts := make([]*pluginv1.PluginGroupScript, 0, len(items))
		for _, item := range items {
			if item.ScriptName == "" {
				continue
			}
			scripts = append(scripts, &pluginv1.PluginGroupScript{
				Group:      group,
				ScriptName: item.ScriptName,
				ScriptType: item.ScriptType,
			})
			if isPortScanCompatiblePluginType(item.ScriptType) {
				compatible++
			}
		}
		if len(scripts) == 0 {
			continue
		}
		result.Groups = append(result.Groups, &pluginv1.PluginGroupInfo{
			Group:           group,
			Total:           int32(len(scripts)),
			CompatibleTotal: int32(compatible),
		})
		result.Scripts = append(result.Scripts, scripts...)
	}
	return result, nil
}

func pluginGroupsListErrorResult(code, message string) *pluginv1.ListPluginGroupsResult {
	return &pluginv1.ListPluginGroupsResult{
		ErrorCode:    code,
		ErrorMessage: message,
	}
}

func (b *legionJobBridge) publishPluginGroupsResult(ctx context.Context, commandID string, result *pluginv1.ListPluginGroupsResult) error {
	raw, err := proto.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal plugin groups result: %w", err)
	}

	publisher, ok := b.capabilityPublisher.(*capabilityEventPublisher)
	if !ok || publisher == nil {
		return fmt.Errorf("capability event publisher is not ready")
	}

	if err := publisher.PublishRaw(ctx, pluginGroupsResultSubject(commandID), raw); err != nil {
		return fmt.Errorf("publish plugin groups result: %w", err)
	}
	log.Infof("published plugin groups result: command_id=%s groups=%d", commandID, len(result.GetGroups()))
	return nil
}
