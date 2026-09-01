// Package legacydrivers 保留基于通用数据库驱动的旧认证实现，
// 仅供差分测试（differential_test.go）对照最小探针的行为。
//
// 本包不被主程序导入：驱动依赖（go-sql-driver/mysql、go-mssqldb、
// mongo-driver、go-pg）因此不会进入默认构建的依赖闭包。
// 差分验证通过后，本包将随驱动依赖一起删除。
package legacydrivers

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	mssql "github.com/denisenkom/go-mssqldb"
	"github.com/go-pg/pg/v10"
	"github.com/go-sql-driver/mysql"
	"github.com/yaklang/yaklang/common/utils"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultTimeout = 10 * time.Second

// MYSQLAuthLegacy 旧版 MySQL 认证（go-sql-driver/mysql 标准拨号）。
//
// 历史缺陷（均已由最小探针修复，差分基准同时修正以比较认证逻辑本身）：
//  1. 原实现用 url.PathEscape 编码密码再拼 DSN：`!` 被转义为 %21 等字符，
//     含特殊字符的正确密码全部被误判为 Access denied；
//  2. 原实现注入 TLS 优先的自定义 dialer：先发 ClientHello 污染连接，
//     即使密码正确也认证失败。
func MYSQLAuthLegacy(target, username, password string, needAuth bool) (ok, finished bool, err error) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = target
	cfg.DBName = "mysql"
	if needAuth {
		cfg.User = username
		cfg.Passwd = password
	}
	dsn := cfg.FormatDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return false, false, err
	}
	defer db.Close()
	_, err = db.Exec("select 1")
	if err != nil {
		errStr := err.Error()
		switch true {
		case strings.Contains(errStr, "timeout"),
			strings.Contains(errStr, "i/o timeout"),
			strings.Contains(errStr, "dial tcp"),
			strings.Contains(errStr, "bad connection"),
			strings.Contains(errStr, "EOF"),
			strings.Contains(errStr, "is not allowed to connect to"),
			strings.Contains(errStr, "connect: connection refused"):
			return false, true, err
		case strings.Contains(errStr, "Error 1045:"):
			return false, false, err
		case strings.Contains(errStr, "Error 1044:"), strings.Contains(errStr, "1044:"):
			return true, false, nil
		}
		return false, false, err
	}
	return true, false, nil
}

// MSSQLAuthLegacy 旧版 MSSQL 认证（go-mssqldb）。
func MSSQLAuthLegacy(target, username, password string, needAuth bool) (ok, finished bool, err error) {
	query := url.Values{}
	query.Add("encrypt", "disable")
	u := &url.URL{Scheme: "sqlserver", Host: target, RawQuery: query.Encode()}
	if needAuth {
		u.User = url.UserPassword(username, password)
	}
	connector, err := mssql.NewConnector(u.String())
	if err != nil {
		return false, true, err
	}

	db := sql.OpenDB(connector)
	db.SetMaxIdleConns(0)
	defer db.Close()
	if err = db.Ping(); err != nil {
		switch true {
		case strings.Contains(err.Error(), "i/o timeout"),
			strings.Contains(err.Error(), "invalid packet size"),
			strings.Contains(err.Error(), "connect: connection refused"):
			return false, true, err
		}
		return false, false, err
	}
	return true, false, nil
}

// MongoDBAuthLegacy 旧版 MongoDB 认证（mongo-driver）。
func MongoDBAuthLegacy(ctx context.Context, target, username, password string, needAuth bool) (bool, error) {
	host, port, _ := utils.ParseStringToHostPort(utils.AppendDefaultPort(target, 27017))
	addr := fmt.Sprintf("mongodb://%s:%d", host, port)
	clientOptions := options.Client().ApplyURI(addr)
	if needAuth {
		clientOptions = clientOptions.SetAuth(options.Credential{Username: username, Password: password})
	}
	mgoCli, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return false, err
	}
	defer mgoCli.Disconnect(ctx)
	if err = mgoCli.Ping(ctx, nil); err != nil {
		return false, err
	}
	return true, nil
}

// PostgresAuthLegacy 旧版 PostgreSQL 认证（go-pg）。
func PostgresAuthLegacy(target, username, password string) (ok, finished bool, err error) {
	db := pg.Connect(&pg.Options{
		Addr:     target,
		User:     username,
		Password: password,
		Database: "postgres",
	})
	defer db.Close()
	if _, err = db.Exec("select 1"); err != nil {
		switch true {
		case strings.Contains(err.Error(), "connect: connection refused"),
			strings.Contains(err.Error(), "no pg_hba.conf entry for host"),
			strings.Contains(err.Error(), "network unreachable"),
			strings.Contains(err.Error(), "i/o timeout"):
			return false, true, err
		}
		return false, false, err
	}
	return true, false, nil
}

// appendDefaultPort 与旧 utils.AppendDefaultPort 行为一致。
func appendDefaultPort(i string, defaultPort int) string {
	return utils.AppendDefaultPort(i, defaultPort)
}

// AppendDefaultPort 导出供差分测试使用。
func AppendDefaultPort(target string, port int) string { return appendDefaultPort(target, port) }
