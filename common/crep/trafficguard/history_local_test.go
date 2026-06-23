package trafficguard

import (
	"database/sql"
	"fmt"

	// 仅当显式启用真实历史仿真(TRAFFICGUARD_HISTORY_DB)时才需要 sqlite 驱动。
	// 它是测试依赖, 不影响非测试构建; CI 不设置该环境变量, 这段代码不会真正执行查询。
	_ "github.com/mattn/go-sqlite3"
)

// loadHistoryFlows 从给定的 yakit 历史 sqlite 抽取真实 HTTP 流量(请求+响应拼接)。
// 仅本地开发仿真用: 测试默认不调用它; 只有 TRAFFICGUARD_HISTORY_DB 指向有效库时才触发。
// 返回错误(而非 panic), 调用方在出错时直接忽略、退回合成语料, 保证 CI 永不因它失败。
func loadHistoryFlows(dbPath string, limit int) ([][]byte, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT request || char(10) || char(10) || response FROM http_flows
		 WHERE length(response) BETWEEN 200 AND 200000
		 ORDER BY length(response) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query http_flows: %w", err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err == nil && len(blob) > 0 {
			out = append(out, []byte(blob))
		}
	}
	return out, nil
}
