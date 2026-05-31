package aibalance

// handle_data_sink.go - 镜像数据落盘的 portal HTTP handlers
//
// 路由 (admin only, 由 AuthMiddleware 默认对未配置的 /portal/api/* 走 admin 校验):
//   GET  /portal/api/mirror-storage-config     读取落盘配置 + 实时用量
//   POST /portal/api/mirror-storage-config     保存落盘配置 (热应用)
//   GET  /portal/api/mirror-records/recent?n=  读取最近 N 条已落盘记录
//
// 关键词: handle_data_sink, 落盘配置读写, 最近记录查看

import (
	"net"
	"net/http"
	"strconv"
)

// handleGetMirrorStorageConfig: GET /portal/api/mirror-storage-config
// 关键词: handleGetMirrorStorageConfig, 落盘配置 + 实时计数
func (c *ServerConfig) handleGetMirrorStorageConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing get mirror storage config request")
	cfg, err := GetMirrorStorageConfig()
	if err != nil {
		c.logError("GetMirrorStorageConfig failed: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "failed to load storage config",
		})
		return
	}
	records, bytes, available := dataSinkStats()
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":            true,
		"enabled":            cfg.Enabled,
		"max_bytes":          cfg.MaxBytes,
		"reclaim_bytes":      cfg.ReclaimBytes,
		"check_interval_sec": cfg.CheckIntervalSec,
		"records":            records,
		"bytes":              bytes,
		"available":          available,
	})
}

// handleSetMirrorStorageConfig: POST /portal/api/mirror-storage-config
// body: {enabled, max_bytes, reclaim_bytes, check_interval_sec}
// 关键词: handleSetMirrorStorageConfig, 保存 + 热应用
func (c *ServerConfig) handleSetMirrorStorageConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing set mirror storage config request")
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
		return
	}
	var reqBody struct {
		Enabled          bool  `json:"enabled"`
		MaxBytes         int64 `json:"max_bytes"`
		ReclaimBytes     int64 `json:"reclaim_bytes"`
		CheckIntervalSec int64 `json:"check_interval_sec"`
	}
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}
	cfg := &AiMirrorStorageConfig{
		Enabled:          reqBody.Enabled,
		MaxBytes:         reqBody.MaxBytes,
		ReclaimBytes:     reqBody.ReclaimBytes,
		CheckIntervalSec: reqBody.CheckIntervalSec,
	}
	if err := SaveMirrorStorageConfig(cfg); err != nil {
		c.logError("SaveMirrorStorageConfig failed: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "failed to save storage config",
		})
		return
	}
	// 重新读出 (带兜底值) 再热应用, 保证内存配置与库一致。
	saved, err := GetMirrorStorageConfig()
	if err == nil {
		applyMirrorStorageConfig(saved)
	}
	c.logInfo("mirror storage config saved: enabled=%v max_bytes=%d reclaim_bytes=%d check_interval_sec=%d",
		cfg.Enabled, cfg.MaxBytes, cfg.ReclaimBytes, cfg.CheckIntervalSec)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "storage config saved",
	})
}

// handleGetMirrorRecentRecords: GET /portal/api/mirror-records/recent?n=20
// 关键词: handleGetMirrorRecentRecords, 最近 N 条已落盘记录
func (c *ServerConfig) handleGetMirrorRecentRecords(conn net.Conn, request *http.Request) {
	c.logInfo("processing get mirror recent records request")
	n := 20
	if v := request.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			if parsed > 200 {
				parsed = 200 // 上限保护, 避免一次拉太多
			}
			n = parsed
		}
	}
	records, err := dataSinkRecent(n)
	if err != nil {
		c.logWarn("dataSinkRecent failed: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "failed to read recent records",
		})
		return
	}
	if records == nil {
		records = []map[string]any{}
	}
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"records": records,
		"total":   len(records),
	})
}
