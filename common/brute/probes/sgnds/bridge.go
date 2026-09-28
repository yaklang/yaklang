package sgnds

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

// BridgeClient 通过文本协议与本地 yakit-sg-jdbc-bridge 进程通信。
// 协议：每行一个 key=value 列表，字段间用 '&' 分隔。
type BridgeClient struct {
	addr    string
	timeout time.Duration
}

// NewBridgeClient 创建桥接客户端。
// addr 形如 "127.0.0.1:19090"。
func NewBridgeClient(addr string, timeout time.Duration) *BridgeClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &BridgeClient{addr: addr, timeout: timeout}
}

// Ping 检查桥接服务是否存活。
func (c *BridgeClient) Ping(ctx context.Context) (bool, error) {
	res, err := c.call(ctx, "action=ping")
	if err != nil {
		return false, err
	}
	return res["ok"] == "true" && res["pong"] == "true", nil
}

// Open 打开一个 JDBC 连接，返回 connId。
func (c *BridgeClient) Open(ctx context.Context, jdbcURL, user, password string) (string, error) {
	req := fmt.Sprintf("action=open&url=%s&user=%s&password=%s",
		escape(jdbcURL), escape(user), escape(password))
	res, err := c.call(ctx, req)
	if err != nil {
		return "", err
	}
	if res["ok"] != "true" {
		return "", bridgeError(res)
	}
	return res["connId"], nil
}

// Close 关闭指定连接。
func (c *BridgeClient) Close(ctx context.Context, connID string) error {
	res, err := c.call(ctx, fmt.Sprintf("action=close&connId=%s", escape(connID)))
	if err != nil {
		return err
	}
	if res["ok"] != "true" {
		return bridgeError(res)
	}
	return nil
}

// Query 在指定连接上执行查询。
func (c *BridgeClient) Query(ctx context.Context, connID, sql string, params []string) (*QueryResult, error) {
	req := fmt.Sprintf("action=query&connId=%s&sql=%s", escape(connID), escape(sql))
	if len(params) > 0 {
		req += "&params=" + escape(strings.Join(params, ","))
	}
	res, err := c.call(ctx, req)
	if err != nil {
		return nil, err
	}
	if res["ok"] != "true" {
		return nil, bridgeError(res)
	}
	qr := &QueryResult{
		Columns: splitUnescape(res["columns"], ","),
	}
	if res["rows"] != "" {
		rawRows := splitUnescape(res["rows"], ",")
		for _, row := range rawRows {
			qr.Rows = append(qr.Rows, splitUnescape(row, "|"))
		}
	}
	return qr, nil
}

// QueryResult 是查询结果的最小表示。
type QueryResult struct {
	Columns []string
	Rows    [][]string
}

func (c *BridgeClient) call(ctx context.Context, req string) (map[string]string, error) {
	d := net.Dialer{Timeout: c.timeout}
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(c.timeout))
	}

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	if _, err := w.WriteString(req + "\n"); err != nil {
		return nil, err
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	line, err := r.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("bridge closed connection")
		}
		return nil, err
	}
	return parseResponse(strings.TrimSpace(line)), nil
}

func bridgeError(res map[string]string) error {
	msg := res["error"]
	if msg == "" {
		msg = "bridge returned failure"
	}
	return errors.New(unescape(msg))
}

func parseResponse(line string) map[string]string {
	m := make(map[string]string)
	if line == "" {
		return m
	}
	parts := strings.Split(line, "&")
	for _, p := range parts {
		idx := strings.IndexByte(p, '=')
		if idx > 0 {
			m[p[:idx]] = unescape(p[idx+1:])
		}
	}
	return m
}

func escape(s string) string {
	return url.QueryEscape(s)
}

func unescape(s string) string {
	v, _ := url.QueryUnescape(s)
	return v
}

func splitUnescape(s string, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	for i := range parts {
		parts[i] = unescape(parts[i])
	}
	return parts
}
