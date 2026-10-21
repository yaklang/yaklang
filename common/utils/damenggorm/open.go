package damenggorm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"regexp"
	"strings"

	dm "gitee.com/chunanyong/dm"
)

var connectionGate = make(chan struct{}, 1)

func enterConnectionGate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case connectionGate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			leaveConnectionGate()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func leaveConnectionGate() { <-connectionGate }

// OpenDB gives every physical connection a private upstream connector. The
// pinned DM driver writes negotiated compression fields into DmConnector while
// connecting, and active connections read those fields without that lock.
// database/sql otherwise reuses one connector across the whole pool.
// Handshakes are serialized because the pinned driver also mutates global
// logging settings and DH parameters during connection establishment. Existing
// query execution remains concurrent; callers can cancel while waiting.
// Driver identity is retained for IsDB/VectorStore.
func OpenDB(dsn string) (*sql.DB, error) { return OpenDBContext(context.Background(), dsn) }
func OpenDBContext(ctx context.Context, dsn string) (*sql.DB, error) {
	u, parseErr := url.Parse(dsn)
	if parseErr != nil || u.Scheme != Dialect || !regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`).MatchString(u.Query().Get("schema")) {
		return nil, errors.New("DM SSA requires an explicit uppercase schema")
	}
	if err := enterConnectionGate(ctx); err != nil {
		return nil, err
	}
	gateHeld := true
	defer func() {
		if gateHeld {
			leaveConnectionGate()
		}
	}()
	probe, err := sql.Open(Dialect, dsn)
	if err != nil {
		return nil, err
	}
	base := probe.Driver()
	_ = probe.Close()
	source, ok := base.(driver.DriverContext)
	if !ok {
		return nil, errors.New("DM driver does not support independent connectors")
	}
	db := sql.OpenDB(&privateConnector{dsn: dsn, base: base, source: source})
	leaveConnectionGate()
	gateHeld = false
	// Verify schema selection before any engine migration or business write.
	var actual string
	if err := db.QueryRowContext(ctx, `SELECT SYS_CONTEXT('USERENV','CURRENT_SCHEMA') FROM DUAL`).Scan(&actual); err != nil {
		_ = db.Close()
		return nil, errors.New("DM SSA schema validation failed")
	}
	if !strings.EqualFold(actual, u.Query().Get("schema")) {
		_ = db.Close()
		return nil, errors.New("DM SSA connection selected a different schema")
	}
	return db, nil
}

type privateConnector struct {
	dsn    string
	base   driver.Driver
	source driver.DriverContext
}

func (c *privateConnector) Driver() driver.Driver { return c.base }
func (c *privateConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := enterConnectionGate(ctx); err != nil {
		return nil, err
	}
	defer leaveConnectionGate()
	dsn := c.dsn
	var socket *connectionSocket
	var request *dialRequest
	var releaseDial func()
	keepDial := false
	connectContext := ctx
	if _, ok := c.base.(*dm.DmDriver); ok {
		socket = &connectionSocket{}
		request = &dialRequest{context: ctx, socket: socket}
		name, release := registerSocketDial(request)
		releaseDial = release
		defer func() {
			request.connecting.Store(false)
			if !keepDial {
				release()
			}
		}()
		adapted, timeout, err := cancellationDSN(dsn, name)
		if err != nil {
			return nil, err
		}
		dsn, request.timeout = adapted, timeout
		connectContext = context.WithoutCancel(ctx)
	}
	connector, err := c.source.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	if socket == nil {
		return connector.Connect(ctx)
	}
	guard, err := socket.watch(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := connector.Connect(connectContext)
	if canceled := guard.end(); canceled != nil {
		// The upstream returns a typed nil *DmConnection on handshake error.
		if err == nil && conn != nil {
			_ = conn.Close()
		}
		return nil, canceled
	}
	if err != nil {
		socket.abort()
		return nil, err
	}
	keepDial = true
	return &cancellationConn{Conn: conn, socket: socket, releaseDial: releaseDial}, nil
}
