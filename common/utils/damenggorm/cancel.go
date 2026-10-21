package damenggorm

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dm "gitee.com/chunanyong/dm"
)

const cancellationDialName = "yaklang-ssa-context-socket-"

type dialRequest struct {
	context    context.Context
	socket     *connectionSocket
	timeout    time.Duration
	connecting atomic.Bool
}

var socketDials struct {
	sync.Mutex
	count uint64
	free  []string
}

// The upstream cancellation watcher closes statements concurrently with their
// execution. Interrupt only the socket here; database/sql then serializes the
// driver's statement/transaction cleanup on the normal connection call path.
// Endpoint-group connection attempts in the pinned driver discard their context.
// Bind the dialer to one physical connection instead. Reuse retired names because
// the public registration API has no deletion operation; registry size is bounded
// by the high-water mark of concurrently allocated connections.
func registerSocketDial(request *dialRequest) (string, func()) {
	socketDials.Lock()
	var name string
	if n := len(socketDials.free); n > 0 {
		name = socketDials.free[n-1]
		socketDials.free = socketDials.free[:n-1]
	} else {
		socketDials.count++
		name = cancellationDialName + strconv.FormatUint(socketDials.count, 10)
	}
	socketDials.Unlock()
	request.connecting.Store(true)
	dm.RegisterDialContext(name, func(_ context.Context, addr string) (net.Conn, error) {
		if !request.connecting.Load() {
			request.socket.abort()
			return nil, driver.ErrBadConn
		}
		dialer := net.Dialer{Timeout: request.timeout, KeepAlive: 2 * time.Second}
		conn, err := dialer.DialContext(request.context, "tcp", addr)
		if err != nil {
			return nil, err
		}
		tracked := &trackedSocket{Conn: conn, state: request.socket}
		request.socket.mu.Lock()
		previous := request.socket.conn
		request.socket.conn = tracked
		invalid := request.socket.invalid.Load()
		request.socket.mu.Unlock()
		if previous != nil {
			_ = previous.Close()
		}
		if invalid {
			_ = tracked.Close()
			return nil, driver.ErrBadConn
		}
		return tracked, nil
	})
	var once sync.Once
	return name, func() {
		once.Do(func() {
			dm.RegisterDialContext(name, func(context.Context, string) (net.Conn, error) { return nil, driver.ErrBadConn })
			socketDials.Lock()
			socketDials.free = append(socketDials.free, name)
			socketDials.Unlock()
		})
	}
}

func cancellationDSN(raw, dialName string) (string, time.Duration, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "dm" {
		return "", 0, errors.New("invalid DM connection URL")
	}
	q := u.Query()
	var timeout time.Duration
	for key, values := range q {
		if strings.EqualFold(key, "dialName") {
			if len(values) != 1 || values[0] != "" {
				return "", 0, errors.New("custom DM dialName is incompatible with cancellation-safe connections")
			}
			q.Del(key)
		}
		if strings.EqualFold(key, "socketTimeout") && len(values) == 1 {
			if seconds, e := strconv.Atoi(values[0]); e == nil && seconds > 0 && seconds <= 2147483647 {
				timeout = time.Duration(seconds) * time.Second
			}
		}
	}
	q.Set("dialName", dialName)
	u.RawQuery = q.Encode()
	return u.String(), timeout, nil
}

type connectionSocket struct {
	mu      sync.Mutex
	conn    net.Conn
	invalid atomic.Bool
}

type trackedSocket struct {
	net.Conn
	state     *connectionSocket
	closeOnce sync.Once
	closeErr  error
}

func (s *trackedSocket) Close() error {
	s.state.invalid.Store(true)
	s.closeOnce.Do(func() { s.closeErr = s.Conn.Close() })
	return s.closeErr
}

func (s *connectionSocket) abort() {
	s.invalid.Store(true)
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

type cancellationGuard struct {
	ctx    context.Context
	socket *connectionSocket
	stop   func() bool
	done   chan struct{}
	once   sync.Once
	err    error
}

func (s *connectionSocket) watch(ctx context.Context) (*cancellationGuard, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.invalid.Load() {
		return nil, driver.ErrBadConn
	}
	g := &cancellationGuard{ctx: ctx, socket: s, done: make(chan struct{})}
	g.stop = context.AfterFunc(ctx, func() {
		defer close(g.done)
		s.abort()
	})
	return g, nil
}

func (g *cancellationGuard) end() error {
	g.once.Do(func() {
		if !g.stop() {
			<-g.done
		}
		g.err = g.ctx.Err()
		if g.err != nil {
			g.socket.abort()
		}
	})
	return g.err
}

type cancellationConn struct {
	driver.Conn
	socket      *connectionSocket
	releaseDial func()
}

func connectionCall[T any](ctx context.Context, c *cancellationConn, call func(context.Context) (T, error)) (result T, err error) {
	g, err := c.socket.watch(ctx)
	if err != nil {
		return result, err
	}
	result, err = call(context.WithoutCancel(ctx))
	if canceled := g.end(); canceled != nil {
		var zero T
		return zero, canceled
	}
	return result, err
}

func (c *cancellationConn) Close() error {
	if c.releaseDial != nil {
		defer c.releaseDial()
	}
	c.socket.abort()
	return c.Conn.Close()
}
func (c *cancellationConn) IsValid() bool {
	if c.socket.invalid.Load() {
		return false
	}
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}
func (c *cancellationConn) CheckNamedValue(v *driver.NamedValue) error {
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(v)
	}
	return driver.ErrSkip
}
func (c *cancellationConn) ResetSession(ctx context.Context) error {
	_, err := connectionCall(ctx, c, func(inner context.Context) (struct{}, error) {
		if resetter, ok := c.Conn.(driver.SessionResetter); ok {
			return struct{}{}, resetter.ResetSession(inner)
		}
		return struct{}{}, nil
	})
	return err
}
func (c *cancellationConn) Ping(ctx context.Context) error {
	_, err := connectionCall(ctx, c, func(inner context.Context) (struct{}, error) {
		return struct{}{}, c.Conn.(driver.Pinger).Ping(inner)
	})
	return err
}
func (c *cancellationConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}
func (c *cancellationConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	query = normalizeIdentifiers(query)
	stmt, err := connectionCall(ctx, c, func(inner context.Context) (driver.Stmt, error) {
		return c.Conn.(driver.ConnPrepareContext).PrepareContext(inner, query)
	})
	if err != nil {
		return nil, err
	}
	return &cancellationStmt{Stmt: stmt, conn: c}, nil
}
func (c *cancellationConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *cancellationConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return connectionCall(ctx, c, func(inner context.Context) (driver.Tx, error) {
		return c.Conn.(driver.ConnBeginTx).BeginTx(inner, opts)
	})
}
func (c *cancellationConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	query = normalizeIdentifiers(query)
	return connectionCall(ctx, c, func(inner context.Context) (driver.Result, error) {
		return c.Conn.(driver.ExecerContext).ExecContext(inner, query, args)
	})
}
func (c *cancellationConn) query(ctx context.Context, call func(context.Context) (driver.Rows, error)) (driver.Rows, error) {
	g, err := c.socket.watch(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := call(context.WithoutCancel(ctx))
	if err != nil {
		// The upstream driver returns a typed nil *DmRows on failure.
		if canceled := g.end(); canceled != nil {
			return nil, canceled
		}
		return nil, err
	}
	if ctx.Err() != nil {
		_ = rows.Close()
		return nil, g.end()
	}
	return &cancellationRows{Rows: rows, guard: g}, nil
}
func (c *cancellationConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	query = normalizeIdentifiers(query)
	return c.query(ctx, func(inner context.Context) (driver.Rows, error) {
		return c.Conn.(driver.QueryerContext).QueryContext(inner, query, args)
	})
}

type cancellationStmt struct {
	driver.Stmt
	conn *cancellationConn
}

func (s *cancellationStmt) CheckNamedValue(v *driver.NamedValue) error {
	if checker, ok := s.Stmt.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(v)
	}
	return driver.ErrSkip
}
func (s *cancellationStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return connectionCall(ctx, s.conn, func(inner context.Context) (driver.Result, error) {
		return s.Stmt.(driver.StmtExecContext).ExecContext(inner, args)
	})
}
func (s *cancellationStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.query(ctx, func(inner context.Context) (driver.Rows, error) {
		return s.Stmt.(driver.StmtQueryContext).QueryContext(inner, args)
	})
}

type cancellationRows struct {
	driver.Rows
	guard *cancellationGuard
}

func (r *cancellationRows) Columns() []string {
	names := r.Rows.Columns()
	result := make([]string, len(names))
	for i, name := range names {
		result[i] = strings.ToLower(name)
	}
	return result
}

func (r *cancellationRows) Close() error {
	err := r.Rows.Close()
	if canceled := r.guard.end(); canceled != nil {
		return canceled
	}
	return err
}
func (r *cancellationRows) Next(dest []driver.Value) error {
	if err := r.guard.ctx.Err(); err != nil {
		return err
	}
	err := r.Rows.Next(dest)
	if canceled := r.guard.ctx.Err(); canceled != nil {
		return canceled
	}
	return err
}
func (r *cancellationRows) HasNextResultSet() bool {
	next, ok := r.Rows.(driver.RowsNextResultSet)
	return ok && next.HasNextResultSet()
}
func (r *cancellationRows) NextResultSet() error {
	if err := r.guard.ctx.Err(); err != nil {
		return err
	}
	if next, ok := r.Rows.(driver.RowsNextResultSet); ok {
		return next.NextResultSet()
	}
	return io.EOF
}
func (r *cancellationRows) ColumnTypeDatabaseTypeName(i int) string {
	if x, ok := r.Rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return x.ColumnTypeDatabaseTypeName(i)
	}
	return ""
}
func (r *cancellationRows) ColumnTypeLength(i int) (int64, bool) {
	if x, ok := r.Rows.(driver.RowsColumnTypeLength); ok {
		return x.ColumnTypeLength(i)
	}
	return 0, false
}
func (r *cancellationRows) ColumnTypeNullable(i int) (bool, bool) {
	if x, ok := r.Rows.(driver.RowsColumnTypeNullable); ok {
		return x.ColumnTypeNullable(i)
	}
	return false, false
}
func (r *cancellationRows) ColumnTypePrecisionScale(i int) (int64, int64, bool) {
	if x, ok := r.Rows.(driver.RowsColumnTypePrecisionScale); ok {
		return x.ColumnTypePrecisionScale(i)
	}
	return 0, 0, false
}
func (r *cancellationRows) ColumnTypeScanType(i int) reflect.Type {
	if x, ok := r.Rows.(driver.RowsColumnTypeScanType); ok {
		return x.ColumnTypeScanType(i)
	}
	return reflect.TypeFor[any]()
}
