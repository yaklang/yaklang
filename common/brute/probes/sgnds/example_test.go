//go:build integration
// +build integration

package sgnds

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestExampleQuery 演示用户如何像普通数据库一样查询 SG-JDBC。
// 修改 jdbcURL/user/password 为你的现场参数后运行：
//
//	go test ./common/brute/probes/sgnds/ -tags integration -v -run TestExampleQuery -timeout 120s
func TestExampleQuery(t *testing.T) {
	jdbcURL := "jdbc:nds://170.20.8.223:18600,170.20.8.224:18600/v_18600_tjuvmp?appname=uvmp"
	user := "your_user"
	password := "your_password"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 第一次调用会自动从 Go 二进制释放 jar 并启动 Java 代理，用户无感知。
	res, err := Query(jdbcURL, user, password, "SELECT 1 FROM DUAL", nil)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	fmt.Println("columns:", res.Columns)
	for _, row := range res.Rows {
		fmt.Println("row:", row)
	}
}
