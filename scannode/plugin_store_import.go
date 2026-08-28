package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

const (
	pluginStoreImportStatusRunning   = "running"
	pluginStoreImportStatusSucceeded = "succeeded"
	pluginStoreImportStatusFailed    = "failed"

	pluginStoreImportDownloadTimeout = 5 * time.Minute
)

var sqliteHeader = []byte("SQLite format 3\x00")

func (b *legionJobBridge) handlePluginStoreImport(ctx context.Context, raw []byte) error {
	var command pluginv1.ImportPluginStoreCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal plugin store import command: %w", err)
	}

	commandID := strings.TrimSpace(command.GetMetadata().GetCommandId())
	currentNodeID := b.agent.node.CurrentNodeID()
	if err := validatePluginStoreImportCommand(currentNodeID, &command); err != nil {
		return b.publishPluginStoreSyncProgress(commandID, &pluginv1.PluginStoreSyncProgress{
			CommandId:    commandID,
			Status:       pluginStoreImportStatusFailed,
			ErrorCode:    "invalid_plugin_store_import_command",
			ErrorMessage: err.Error(),
			UpdatedAt:    timestamppb.New(time.Now().UTC()),
		})
	}

	// 导入同样复用同步状态机，进度通过 plugin.store.sync.result 通道回传。
	if snapshot := pluginStoreSync.snapshot(); snapshot != nil && snapshot.Status == pluginStoreSyncStatusRunning {
		return b.publishPluginStoreSyncProgress(commandID, &pluginv1.PluginStoreSyncProgress{
			CommandId:    commandID,
			Status:       pluginStoreImportStatusFailed,
			ErrorCode:    "plugin_store_sync_busy",
			ErrorMessage: "another plugin store sync or import is already running on this node",
			UpdatedAt:    timestamppb.New(time.Now().UTC()),
		})
	}

	go b.runPluginStoreImport(b.agent.node.GetRootContext(), commandID, &command)
	return nil
}

func (b *legionJobBridge) runPluginStoreImport(ctx context.Context, commandID string, command *pluginv1.ImportPluginStoreCommand) {
	pluginStoreSync.update(commandID, func(p *pluginv1.PluginStoreSyncProgress) {
		p.Status = pluginStoreImportStatusRunning
		p.CurrentPlugin = "downloading plugin database"
		p.Progress = 0
	})
	b.publishPluginStoreSyncProgress(commandID, pluginStoreSync.snapshot())

	progress, err := b.executePluginStoreImport(ctx, commandID, command)
	if err != nil {
		log.Errorf("plugin store import failed: command_id=%s err=%v", commandID, err)
		progress = pluginStoreSync.update(commandID, func(p *pluginv1.PluginStoreSyncProgress) {
			p.Status = pluginStoreImportStatusFailed
			p.ErrorCode = "plugin_store_import_failed"
			p.ErrorMessage = err.Error()
		})
	}
	b.publishPluginStoreSyncProgress(commandID, progress)
}

func (b *legionJobBridge) executePluginStoreImport(
	ctx context.Context,
	commandID string,
	command *pluginv1.ImportPluginStoreCommand,
) (*pluginv1.PluginStoreSyncProgress, error) {
	// 1. 下载上传的 db 文件到临时路径。用节点自己的 session 认证下载。
	session, ok := b.agent.node.GetSessionState()
	if !ok {
		return nil, fmt.Errorf("node session is not ready")
	}

	downloadCtx, cancel := context.WithTimeout(ctx, pluginStoreImportDownloadTimeout)
	defer cancel()

	tempPath, err := downloadPluginStoreDB(downloadCtx, command.GetArtifactUrl(), command.GetArtifactSha256(), session.SessionID, session.SessionToken)
	if err != nil {
		return nil, fmt.Errorf("download plugin db: %w", err)
	}
	defer os.Remove(tempPath)

	// 2. 校验文件头是 SQLite。
	header := make([]byte, len(sqliteHeader))
	file, err := os.Open(tempPath)
	if err != nil {
		return nil, fmt.Errorf("open downloaded db: %w", err)
	}
	if _, err := file.Read(header); err != nil {
		file.Close()
		return nil, fmt.Errorf("read db header: %w", err)
	}
	file.Close()
	if len(header) < len(sqliteHeader) || string(header[:len(sqliteHeader)]) != string(sqliteHeader) {
		return nil, fmt.Errorf("downloaded file is not a SQLite database")
	}

	pluginStoreSync.update(commandID, func(p *pluginv1.PluginStoreSyncProgress) {
		p.CurrentPlugin = "replacing local plugin database"
		p.Progress = 0.5
	})
	b.publishPluginStoreSyncProgress(commandID, pluginStoreSync.snapshot())

	// 3. 安全替换：备份旧库 → 原子 rename → 重新绑定 profile DB。
	currentPath := consts.GetCurrentProfileDatabasePath()
	backupPath := currentPath + ".import-bak"

	// 关闭当前 gorm 连接，避免文件锁冲突。
	consts.CloseProfileDatabase()

	if _, err := os.Stat(currentPath); err == nil {
		if err := os.Rename(currentPath, backupPath); err != nil {
			b.rebindProfileDatabase(currentPath)
			return nil, fmt.Errorf("backup current db: %w", err)
		}
	}

	if err := os.Rename(tempPath, currentPath); err != nil {
		// 回滚：恢复备份
		if _, bakErr := os.Stat(backupPath); bakErr == nil {
			os.Rename(backupPath, currentPath)
		}
		b.rebindProfileDatabase(currentPath)
		return nil, fmt.Errorf("replace db file: %w", err)
	}

	// 重新打开并绑定新库。
	if err := b.rebindProfileDatabase(currentPath); err != nil {
		// 打开失败，尝试回滚备份。
		if _, bakErr := os.Stat(backupPath); bakErr == nil {
			os.Rename(currentPath, currentPath+".failed")
			os.Rename(backupPath, currentPath)
			consts.CreateProfileDatabase(currentPath) // best-effort
		}
		return nil, fmt.Errorf("rebind profile database: %w", err)
	}

	// 4. 统计新库插件数作为信息性结果。
	pluginCount := countPluginsInProfileDB()

	final := pluginStoreSync.update(commandID, func(p *pluginv1.PluginStoreSyncProgress) {
		p.Status = pluginStoreImportStatusSucceeded
		p.Progress = 1
		p.Total = int32(pluginCount)
		p.Completed = int32(pluginCount)
		p.Succeeded = int32(pluginCount)
		p.CurrentPlugin = ""
	})
	log.Infof("plugin store import finished: command_id=%s plugins=%d backup=%s", commandID, pluginCount, backupPath)
	return final, nil
}

func (b *legionJobBridge) rebindProfileDatabase(path string) error {
	db, err := consts.CreateProfileDatabase(path)
	if err != nil {
		return err
	}
	consts.BindProfileDatabase(db, path)
	return nil
}

func countPluginsInProfileDB() int {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return 0
	}
	var count int64
	db.Model(&schema.YakScript{}).Where("deleted_at IS NULL").Count(&count)
	return int(count)
}

func downloadPluginStoreDB(ctx context.Context, url, expectedSHA256, sessionID, sessionToken string) (string, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", fmt.Errorf("artifact url is empty")
	}

	tempDir := os.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "plugin-store-import-*.db")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("clear temp path: %w", err)
	}

	// 构造带节点 session 认证的 HTTP 请求下载 db。
	parsed, err := neturl.Parse(url)
	if err != nil {
		return "", fmt.Errorf("parse artifact url: %w", err)
	}
	q := parsed.Query()
	q.Set("node_session_id", sessionID)
	parsed.RawQuery = q.Encode()
	authURL := parsed.String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
	if err != nil {
		return "", fmt.Errorf("build download request: %w", err)
	}
	if sessionToken != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(sessionToken))
	}
	client := &http.Client{Timeout: pluginStoreImportDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch artifact: unexpected status %d", resp.StatusCode)
	}

	hasher := sha256.New()
	out, err := os.Create(tempPath)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(io.MultiWriter(out, hasher), resp.Body); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("download plugin db: %w", err)
	}
	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA256 != "" && !strings.EqualFold(actualSHA, strings.TrimSpace(expectedSHA256)) {
		os.Remove(tempPath)
		return "", fmt.Errorf("sha256 mismatch: expected %s got %s", expectedSHA256, actualSHA)
	}
	return tempPath, nil
}

func validatePluginStoreImportCommand(nodeID string, command *pluginv1.ImportPluginStoreCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("plugin store import metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("plugin store import command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("plugin store import target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != strings.TrimSpace(nodeID):
		return fmt.Errorf("plugin store import target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetArtifactUrl()) == "":
		return fmt.Errorf("plugin store import artifact_url is required")
	default:
		return nil
	}
}