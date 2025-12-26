package scannode

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
)

// RuleSyncConfig 规则同步配置
type RuleSyncConfig struct {
	ServerURL   string `json:"server_url"`   // Legion Server URL (e.g., http://localhost:8080)
	APIToken    string `json:"api_token"`    // API认证Token
	SyncEnabled bool   `json:"sync_enabled"` // 是否启用同步
}

// RuleSyncClient 规则同步客户端
type RuleSyncClient struct {
	config     *RuleSyncConfig
	httpClient *http.Client
	cacheDir   string
	mu         sync.RWMutex
}

// RuleSnapshotInfo 快照信息（从服务器获取）
type RuleSnapshotInfo struct {
	ID        int64  `json:"id"`
	Version   string `json:"version"`
	Hash      string `json:"hash"`
	RuleCount int64  `json:"rule_count"`
	Status    string `json:"status"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
}

// NewRuleSyncClient 创建规则同步客户端
func NewRuleSyncClient(config *RuleSyncConfig) *RuleSyncClient {
	cacheDir := filepath.Join(utils.GetHomeDirDefault("/tmp"), ".palm-desktop", "rule_cache")
	os.MkdirAll(cacheDir, 0755)

	return &RuleSyncClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		cacheDir: cacheDir,
	}
}

// GetLatestSnapshot 获取最新快照信息
func (c *RuleSyncClient) GetLatestSnapshot() (*RuleSnapshotInfo, error) {
	if c.config == nil || c.config.ServerURL == "" {
		return nil, utils.Error("rule sync not configured")
	}

	url := fmt.Sprintf("%s/api/syntaxflow/rules/snapshot", c.config.ServerURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.APIToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, utils.Wrap(err, "request snapshot list failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, utils.Errorf("get snapshot list failed: %d - %s", resp.StatusCode, string(body))
	}

	var result struct {
		Snapshots []*RuleSnapshotInfo `json:"snapshots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, utils.Wrap(err, "decode snapshot list failed")
	}

	if len(result.Snapshots) == 0 {
		return nil, utils.Error("no snapshots available")
	}

	return result.Snapshots[0], nil
}

// DownloadSnapshotZip 下载快照ZIP内容
// 服务端返回 application/octet-stream 二进制流
func (c *RuleSyncClient) DownloadSnapshotZip(hash string) ([]byte, error) {
	if c.config == nil || c.config.ServerURL == "" {
		return nil, utils.Error("rule sync not configured")
	}

	// 检查本地缓存
	cachePath := filepath.Join(c.cacheDir, hash+".zip")
	if data, err := os.ReadFile(cachePath); err == nil {
		log.Infof("loaded snapshot from cache: %s", hash)
		return data, nil
	}

	// 从服务器下载
	url := fmt.Sprintf("%s/api/syntaxflow/rules/snapshot/%s", c.config.ServerURL, hash)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.APIToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, utils.Wrap(err, "download snapshot failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, utils.Errorf("download snapshot failed: %d - %s", resp.StatusCode, string(body))
	}

	// 直接读取二进制流
	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, utils.Wrap(err, "read snapshot body failed")
	}

	// 保存到缓存
	if err := os.WriteFile(cachePath, zipData, 0644); err != nil {
		log.Warnf("save snapshot cache failed: %v", err)
	}

	log.Infof("downloaded snapshot: %s, size: %d bytes", hash, len(zipData))
	return zipData, nil
}

// HasLocalSnapshot 检查本地是否有指定快照
func (c *RuleSyncClient) HasLocalSnapshot(hash string) bool {
	cachePath := filepath.Join(c.cacheDir, hash+".zip")
	_, err := os.Stat(cachePath)
	return err == nil
}

// SyncAndImportLatest 同步最新规则并导入到本地数据库
// 使用sfdb.ImportRulesFromBytes直接导入ZIP格式的规则
func (c *RuleSyncClient) SyncAndImportLatest() (ruleCount int, err error) {
	if c.config == nil || !c.config.SyncEnabled {
		return 0, utils.Error("rule sync disabled")
	}

	// 1. 获取最新快照信息
	snapshot, err := c.GetLatestSnapshot()
	if err != nil {
		return 0, utils.Wrap(err, "get latest snapshot failed")
	}

	log.Infof("found latest snapshot: version=%s, hash=%s, rules=%d",
		snapshot.Version, snapshot.Hash, snapshot.RuleCount)

	// 2. 下载快照ZIP
	zipData, err := c.DownloadSnapshotZip(snapshot.Hash)
	if err != nil {
		return 0, utils.Wrap(err, "download snapshot failed")
	}

	// 3. 使用sfdb导入到本地数据库
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return 0, utils.Error("local database not available")
	}

	metadata, err := sfdb.ImportRulesFromBytes(context.Background(), db, zipData)
	if err != nil {
		return 0, utils.Wrap(err, "import rules from zip failed")
	}

	ruleCount = utils.InterfaceToInt(metadata["count"])
	log.Infof("successfully imported %d rules from snapshot %s", ruleCount, snapshot.Version)

	return ruleCount, nil
}

// SyncForHash 同步指定Hash的规则
func (c *RuleSyncClient) SyncForHash(hash string) (ruleCount int, err error) {
	if c.config == nil || !c.config.SyncEnabled {
		return 0, utils.Error("rule sync disabled")
	}

	// 1. 下载快照ZIP
	zipData, err := c.DownloadSnapshotZip(hash)
	if err != nil {
		return 0, utils.Wrap(err, "download snapshot failed")
	}

	// 2. 使用sfdb导入到本地数据库
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return 0, utils.Error("local database not available")
	}

	metadata, err := sfdb.ImportRulesFromBytes(context.Background(), db, zipData)
	if err != nil {
		return 0, utils.Wrap(err, "import rules from zip failed")
	}

	ruleCount = utils.InterfaceToInt(metadata["count"])
	log.Infof("successfully imported %d rules from snapshot %s", ruleCount, hash)

	return ruleCount, nil
}
