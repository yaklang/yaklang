package scannode

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

const (
	pluginStoreSyncStatusRunning   = "running"
	pluginStoreSyncStatusSucceeded = "succeeded"
	pluginStoreSyncStatusFailed    = "failed"

	// 进度回传节流：商店全量下载上千个插件，逐条回传会刷爆 NATS。
	pluginStoreSyncReportInterval = 2 * time.Second
)

type pluginStoreSyncState struct {
	mu      sync.Mutex
	current *pluginv1.PluginStoreSyncProgress
}

var pluginStoreSync = &pluginStoreSyncState{}

func (s *pluginStoreSyncState) snapshot() *pluginv1.PluginStoreSyncProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return nil
	}
	return proto.Clone(s.current).(*pluginv1.PluginStoreSyncProgress)
}

func (s *pluginStoreSyncState) update(
	commandID string,
	mutate func(*pluginv1.PluginStoreSyncProgress),
) *pluginv1.PluginStoreSyncProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil || s.current.CommandId != commandID {
		s.current = &pluginv1.PluginStoreSyncProgress{
			CommandId: commandID,
			Status:    pluginStoreSyncStatusRunning,
			UpdatedAt: timestamppb.New(time.Now().UTC()),
		}
	}
	if s.current.CommandId != commandID {
		s.current = &pluginv1.PluginStoreSyncProgress{
			CommandId: commandID,
			Status:    pluginStoreSyncStatusRunning,
			UpdatedAt: timestamppb.New(time.Now().UTC()),
		}
	}
	mutate(s.current)
	s.current.UpdatedAt = timestamppb.New(time.Now().UTC())
	return proto.Clone(s.current).(*pluginv1.PluginStoreSyncProgress)
}

func (b *legionJobBridge) handlePluginStoreSync(ctx context.Context, raw []byte) error {
	var command pluginv1.SyncPluginStoreCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal plugin store sync command: %w", err)
	}

	commandID := strings.TrimSpace(command.GetMetadata().GetCommandId())
	currentNodeID := b.agent.node.CurrentNodeID()
	if err := validatePluginStoreSyncCommand(currentNodeID, &command); err != nil {
		return b.publishPluginStoreSyncProgress(commandID, &pluginv1.PluginStoreSyncProgress{
			CommandId:    commandID,
			Status:       pluginStoreSyncStatusFailed,
			ErrorCode:    "invalid_plugin_store_sync_command",
			ErrorMessage: err.Error(),
			UpdatedAt:    timestamppb.New(time.Now().UTC()),
		})
	}

	if snapshot := pluginStoreSync.snapshot(); snapshot != nil && snapshot.Status == pluginStoreSyncStatusRunning {
		return b.publishPluginStoreSyncProgress(commandID, &pluginv1.PluginStoreSyncProgress{
			CommandId:    commandID,
			Status:       pluginStoreSyncStatusFailed,
			ErrorCode:    "plugin_store_sync_busy",
			ErrorMessage: "another plugin store sync is already running on this node",
			UpdatedAt:    timestamppb.New(time.Now().UTC()),
		})
	}

	// 同步耗时可达分钟级，放到后台执行，不阻塞命令消费循环。
	go b.runPluginStoreSync(b.agent.node.GetRootContext(), commandID)
	return nil
}

func (b *legionJobBridge) runPluginStoreSync(ctx context.Context, commandID string) {
	progress, err := b.downloadPluginStore(ctx, commandID)
	if err != nil {
		log.Errorf("plugin store sync failed: command_id=%s err=%v", commandID, err)
		progress = b.reportPluginStoreFailure(commandID, "plugin_store_sync_failed", err.Error())
	}
	if publishErr := b.publishPluginStoreSyncProgress(commandID, progress); publishErr != nil {
		log.Errorf("publish plugin store sync final progress: %v", publishErr)
	}
}

func (b *legionJobBridge) downloadPluginStore(
	ctx context.Context,
	commandID string,
) (*pluginv1.PluginStoreSyncProgress, error) {
	b.publishPluginStoreSyncProgress(
		commandID,
		pluginStoreSync.update(commandID, func(p *pluginv1.PluginStoreSyncProgress) {
			p.Status = pluginStoreSyncStatusRunning
			p.CurrentPlugin = "connecting to plugin store"
		}),
	)

	if err := yaklib.DownloadOnlineAuthProxy(consts.GetOnlineBaseUrl()); err != nil {
		return nil, fmt.Errorf("connect plugin store: %w", err)
	}

	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, fmt.Errorf("profile database is not initialized")
	}

	// 与 Yakit 桌面端「插件商店-下载全部」同一条下载链路
	// （DownloadOnlinePluginsBatch -> api/plugins/download，数组型过滤字段），
	// 不使用遗留的 DownloadYakitPluginAllWithToken 旧路径。
	stream := client.DownloadOnlinePluginsBatch(ctx, "", nil, "", nil, nil, "", 0, "", nil, "", nil, nil, nil, nil)
	if stream == nil {
		return nil, fmt.Errorf("download stream error: empty")
	}

	var (
		total     int64
		completed int32
		succeeded int32
		failed    int32
		lastSend  time.Time
	)
	for item := range stream.Chan {
		if item == nil || item.Plugin == nil {
			continue
		}
		if item.Total > 0 {
			total = item.Total
		}
		completed++
		if err := client.Save(db, item.Plugin); err != nil {
			failed++
			log.Errorf("plugin store sync: save [%s] failed: %v", item.Plugin.ScriptName, err)
		} else {
			succeeded++
		}

		now := time.Now()
		if now.Sub(lastSend) >= pluginStoreSyncReportInterval {
			lastSend = now
			b.publishPluginStoreSyncProgress(
				commandID,
				pluginStoreSync.update(commandID, func(p *pluginv1.PluginStoreSyncProgress) {
					p.Status = pluginStoreSyncStatusRunning
					p.Total = int32(total)
					p.Completed = completed
					p.Succeeded = succeeded
					p.Failed = failed
					p.CurrentPlugin = item.Plugin.ScriptName
					if total > 0 {
						p.Progress = float64(completed) / float64(total)
					}
				}),
			)
		}
	}

	// 商店不可达或返回空目录时下载流以零插件结束；这与“正常但无插件”无法区分，
	// 但 Yakit 社区商店始终有上千公共插件，零结果一律按连接失败上报。
	if total == 0 && completed == 0 {
		return nil, fmt.Errorf("plugin store returned no plugins (store unreachable or empty)")
	}

	final := pluginStoreSync.update(commandID, func(p *pluginv1.PluginStoreSyncProgress) {
		p.Status = pluginStoreSyncStatusSucceeded
		p.Total = int32(total)
		p.Completed = completed
		p.Succeeded = succeeded
		p.Failed = failed
		p.CurrentPlugin = ""
		if total > 0 {
			p.Progress = float64(completed) / float64(total)
		} else {
			p.Progress = 1
		}
	})
	log.Infof(
		"plugin store sync finished: command_id=%s total=%d succeeded=%d failed=%d",
		commandID, total, succeeded, failed,
	)
	return final, nil
}

func (b *legionJobBridge) reportPluginStoreFailure(commandID, code, message string) *pluginv1.PluginStoreSyncProgress {
	return pluginStoreSync.update(commandID, func(p *pluginv1.PluginStoreSyncProgress) {
		p.Status = pluginStoreSyncStatusFailed
		p.ErrorCode = code
		p.ErrorMessage = message
	})
}

func (b *legionJobBridge) handlePluginStoreSyncStatusQuery(ctx context.Context, raw []byte) error {
	var command pluginv1.QueryPluginStoreSyncStatusCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal plugin store sync status query: %w", err)
	}

	currentNodeID := b.agent.node.CurrentNodeID()
	if err := validatePluginStoreSyncStatusQuery(currentNodeID, &command); err != nil {
		return b.publishPluginStoreSyncProgress(command.GetMetadata().GetCommandId(), &pluginv1.PluginStoreSyncProgress{
			CommandId:    command.GetMetadata().GetCommandId(),
			Status:       pluginStoreSyncStatusFailed,
			ErrorCode:    "invalid_plugin_store_sync_status_query",
			ErrorMessage: err.Error(),
			UpdatedAt:    timestamppb.New(time.Now().UTC()),
		})
	}

	// 查询任意 commandID 的当前状态：节点只保留最近一次同步的结果，
	// 非当前 commandID 的查询返回空状态由平台侧判定为已过期。
	// 空 command_id 表示 latest 查询：返回最近一次同步的快照并发到基础结果 subject，
	// 快照本体保留真实 command_id，平台侧据此回填 sync_id。
	// 回复必须发到平台订阅的 subject（按被查询的 syncID 派生），而不是查询命令自身的 id。
	snapshot := pluginStoreSync.snapshot()
	if snapshot == nil {
		snapshot = &pluginv1.PluginStoreSyncProgress{
			CommandId: command.GetCommandId(),
			Status:    "",
			UpdatedAt: timestamppb.New(time.Now().UTC()),
		}
	}
	return b.publishPluginStoreSyncProgress(command.GetCommandId(), snapshot)
}

func (b *legionJobBridge) publishPluginStoreSyncProgress(
	commandID string,
	progress *pluginv1.PluginStoreSyncProgress,
) error {
	raw, err := proto.Marshal(progress)
	if err != nil {
		return fmt.Errorf("marshal plugin store sync progress: %w", err)
	}

	publisher, ok := b.capabilityPublisher.(*capabilityEventPublisher)
	if !ok || publisher == nil {
		return fmt.Errorf("capability event publisher is not ready")
	}

	publishCtx, cancel := context.WithTimeout(b.agent.node.GetRootContext(), 5*time.Second)
	defer cancel()
	if err := publisher.PublishRaw(publishCtx, pluginStoreSyncResultSubject(commandID), raw); err != nil {
		return fmt.Errorf("publish plugin store sync progress: %w", err)
	}
	return nil
}

func validatePluginStoreSyncCommand(nodeID string, command *pluginv1.SyncPluginStoreCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("plugin store sync metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("plugin store sync command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("plugin store sync target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != strings.TrimSpace(nodeID):
		return fmt.Errorf("plugin store sync target_node_id mismatch: %s", command.GetTargetNodeId())
	default:
		return nil
	}
}

func validatePluginStoreSyncStatusQuery(
	nodeID string,
	command *pluginv1.QueryPluginStoreSyncStatusCommand,
) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("plugin store sync status metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("plugin store sync status command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("plugin store sync status target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != strings.TrimSpace(nodeID):
		return fmt.Errorf("plugin store sync status target_node_id mismatch: %s", command.GetTargetNodeId())
	default:
		return nil
	}
}
