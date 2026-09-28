package sgnds

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Manager 是 SG-JDBC 的高层无感知入口。
//
// 用法：
//   db, err := sgnds.DefaultManager().Open("jdbc:nds://ip1:18600,ip2:18600/v_db?appname=app", "user", "pass")
//   rows, err := db.Query("SELECT * FROM t")
//
// 首次调用会自动从 Go 二进制内嵌 jar 启动本地 Java 代理。
type Manager struct {
	mu     sync.Mutex
	addr   string
	client *BridgeClient
	closed bool
}

var (
	defaultManager     *Manager
	defaultManagerOnce sync.Once
)

// DefaultManager 返回全局默认 Manager。
func DefaultManager() *Manager {
	defaultManagerOnce.Do(func() {
		defaultManager = &Manager{}
	})
	return defaultManager
}

// SetBridgeAddr 允许外部指定固定代理地址；不调用则自动启动内嵌代理。
func (m *Manager) SetBridgeAddr(addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addr = addr
}

// ensure 确保代理地址可用。
func (m *Manager) ensure(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", errors.New("sgnds manager is closed")
	}
	if m.addr != "" {
		return m.addr, nil
	}
	addr, err := EnsureBridgeRunning(ctx)
	if err != nil {
		return "", err
	}
	m.addr = addr
	m.client = NewBridgeClient(addr, 30*time.Second)
	return addr, nil
}

// Open 返回一个操作 SG-JDBC 的 *sql.DB。
// jdbcURL 示例：jdbc:nds://170.20.8.223:18600,170.20.8.224:18600/v_db?appname=app
func (m *Manager) Open(jdbcURL, user, password string) (*sql.DB, error) {
	addr, err := m.ensure(context.Background())
	if err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("sgnds://%s?jdbcurl=%s", addr, url.QueryEscape(jdbcURL))
	return sql.Open("sgnds", dsn)
}

// Close 停止默认 Manager 关联的本地代理（如果是由本 Manager 启动的）。
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	if bridgeLauncher != nil {
		return bridgeLauncher.Stop()
	}
	return nil
}

// ---- database/sql/driver 实现 ----

func init() {
	sql.Register("sgnds", &sgndsDriver{})
}

type sgndsDriver struct{}

func (d *sgndsDriver) Open(dsn string) (driver.Conn, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}
	jdbcURL, err := url.QueryUnescape(u.Query().Get("jdbcurl"))
	if err != nil {
		return nil, err
	}
	if jdbcURL == "" {
		return nil, errors.New("sgnds dsn missing jdbcurl")
	}
	user := u.Query().Get("user")
	password := u.Query().Get("password")
	if user == "" || password == "" {
		// 允许从 DSN 提取，否则默认占位；真实场景建议显式传。
	}
	return newSgndsConn(jdbcURL, user, password)
}

func (d *sgndsDriver) OpenConnector(dsn string) (driver.Connector, error) {
	return &sgndsConnector{driver: d, dsn: dsn}, nil
}

type sgndsConnector struct {
	driver *sgndsDriver
	dsn    string
}

func (c *sgndsConnector) Connect(ctx context.Context) (driver.Conn, error) {
	return c.driver.Open(c.dsn)
}
func (c *sgndsConnector) Driver() driver.Driver {
	return c.driver
}

// sgndsConn 实现 driver.Conn。
type sgndsConn struct {
	jdbcURL  string
	user     string
	password string
	connID   string
	client   *BridgeClient
}

func newSgndsConn(jdbcURL, user, password string) (*sgndsConn, error) {
	addr, err := DefaultManager().ensure(context.Background())
	if err != nil {
		return nil, err
	}
	c := NewBridgeClient(addr, 30*time.Second)
	connID, err := c.Open(context.Background(), jdbcURL, user, password)
	if err != nil {
		return nil, err
	}
	return &sgndsConn{
		jdbcURL:  jdbcURL,
		user:     user,
		password: password,
		connID:   connID,
		client:   c,
	}, nil
}

func (c *sgndsConn) Prepare(query string) (driver.Stmt, error) {
	return &sgndsStmt{conn: c, query: query}, nil
}

func (c *sgndsConn) Close() error {
	return c.client.Close(context.Background(), c.connID)
}

func (c *sgndsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("sgnds: transactions not supported in minimal driver")
}

type sgndsStmt struct {
	conn  *sgndsConn
	query string
}

func (s *sgndsStmt) Close() error { return nil }
func (s *sgndsStmt) NumInput() int {
	return -1 // 不预解析占位符
}
func (s *sgndsStmt) Exec(args []driver.Value) (driver.Result, error) {
	params := valuesToStrings(args)
	affected, err := s.conn.client.Exec(context.Background(), s.conn.connID, s.query, params)
	if err != nil {
		return nil, err
	}
	return driver.RowsAffected(affected), nil
}
func (s *sgndsStmt) Query(args []driver.Value) (driver.Rows, error) {
	params := valuesToStrings(args)
	res, err := s.conn.client.Query(context.Background(), s.conn.connID, s.query, params)
	if err != nil {
		return nil, err
	}
	return &sgndsRows{res: res, idx: -1}, nil
}

type sgndsRows struct {
	res *QueryResult
	idx int
}

func (r *sgndsRows) Columns() []string { return r.res.Columns }
func (r *sgndsRows) Close() error       { return nil }
func (r *sgndsRows) Next(dest []driver.Value) error {
	r.idx++
	if r.idx >= len(r.res.Rows) {
		return io.EOF
	}
	for i, v := range r.res.Rows[r.idx] {
		dest[i] = v
	}
	return nil
}

func valuesToStrings(args []driver.Value) []string {
	res := make([]string, len(args))
	for i, v := range args {
		switch x := v.(type) {
		case string:
			res[i] = x
		case []byte:
			res[i] = string(x)
		case nil:
			res[i] = "null"
		default:
			res[i] = fmt.Sprintf("%v", x)
		}
	}
	return res
}

// Exec 在默认 Manager 上执行非查询 SQL。
func Exec(jdbcURL, user, password, sql string, params []string) (int64, error) {
	db, err := DefaultManager().Open(jdbcURL, user, password)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	res, err := db.Exec(sql, stringSliceToInterface(params)...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Query 在默认 Manager 上执行查询。
func Query(jdbcURL, user, password, sql string, params []string) (*QueryResult, error) {
	db, err := DefaultManager().Open(jdbcURL, user, password)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(sql, stringSliceToInterface(params)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	res := &QueryResult{Columns: cols}
	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]string, len(cols))
		for i, v := range raw {
			if v == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		res.Rows = append(res.Rows, row)
	}
	return res, rows.Err()
}

func stringSliceToInterface(s []string) []interface{} {
	res := make([]interface{}, len(s))
	for i, v := range s {
		res[i] = v
	}
	return res
}

// ---- 便捷：从 URL 提取用户/密码 ----

// ParseJDBCURL 解析 SG-JDBC URL，返回 (jdbcURL, user, password, error)。
func ParseJDBCURL(input string) (string, string, string, error) {
	if !strings.HasPrefix(input, "jdbc:nds://") {
		return "", "", "", errors.New("not a jdbc:nds URL")
	}
	return input, "", "", nil
}

// Atoi 包装 strconv.Atoi，避免外部导入 strconv。
func Atoi(s string) (int, error) {
	return strconv.Atoi(s)
}
