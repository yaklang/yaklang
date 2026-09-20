// Package oracleprobe contains only the password-login portion of go-ora.
// Protocol/crypto code is derived from v3.0.1 (025c51529284177330cde75c2e60c137c5f30fe1),
// with bounded I/O and no database/sql registration, SQL execution, or value codecs.
package oracleprobe

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Dialer must honor cancellation and deadlines supplied through its context.
type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// MaxTimeout bounds the entire login exchange, including redirects and TLS.
const MaxTimeout = 20 * time.Second

// EncryptionPolicy controls Oracle native encryption independently of TLS.
type EncryptionPolicy byte

const (
	EncryptionAccepted EncryptionPolicy = iota
	EncryptionRejected
	EncryptionRequested
	EncryptionRequired
)

type Options struct {
	Address, Service, Username, Password string
	SID, SysDBA                          bool
	// Timeout defaults to 10 seconds and is capped at MaxTimeout.
	Timeout time.Duration
	// TLS uses the caller's verification policy. Nil selects ordinary TNS/TCP.
	TLS        *tls.Config
	Encryption EncryptionPolicy
}

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string             { return fmt.Sprintf("ORA-%05d: %s", e.Code, e.Message) }
func (e *Error) ServiceUnknown() bool      { return e.Code == 12505 || e.Code == 12514 }
func (e *Error) CredentialsRejected() bool { return e.Code == 1017 }

type clientInfo struct {
	HostName, ProgramName, OSUserName, DriverName string
	PID                                           int
}
type loginOptions struct {
	UserID, Password, descriptor string
	ClientInfo                   clientInfo
}

func (o *loginOptions) ConnectionData() string { return o.descriptor }

type LogonMode int

const (
	NoNewPass   LogonMode = 1
	UserAndPass LogonMode = 0x100
)

type Connection struct {
	ctx               context.Context
	connOption        *loginOptions
	details           *Details
	session           *session
	tcpNego           *TCPNego
	dataNego          *DataTypeNego
	LogonMode         LogonMode
	SessionProperties map[string]string
}

// Probe returns nil only after the server completes password authentication.
// It never issues a SQL query. Every connection is closed before returning.
func Probe(ctx context.Context, dialer Dialer, o Options) (err error) {
	_, err = ProbeDetailed(ctx, dialer, o)
	return err
}

// ProbeDetailed preserves Probe's authentication contract and adds bounded,
// non-secret evidence for callers that need to distinguish failure stages.
func ProbeDetailed(ctx context.Context, dialer Dialer, o Options) (details Details, err error) {
	details.Stage, details.Transport = "options", "unknown"
	err = probe(ctx, dialer, o, &details)
	if err != nil {
		err = &Failure{Stage: details.Stage, Err: err}
	}
	return
}

func probe(ctx context.Context, dialer Dialer, o Options, details *Details) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	if o.Timeout > MaxTimeout {
		o.Timeout = MaxTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	if o.Encryption > EncryptionRequired {
		return errors.New("oracle: invalid encryption policy")
	}
	host, port, e := net.SplitHostPort(o.Address)
	if e != nil {
		return e
	}
	p, e := strconv.Atoi(port)
	if e != nil || p < 1 || p > 65535 {
		return errors.New("oracle: invalid port")
	}
	if o.Service == "" {
		return errors.New("oracle: service is required")
	}
	for _, s := range []string{host, o.Service} {
		if len(s) > 1024 || strings.ContainsAny(s, "()\x00\r\n") {
			return errors.New("oracle: invalid connect descriptor value")
		}
	}
	if o.Username == "" || len(o.Username) > 1024 || len(o.Password) > 4096 {
		return ErrInvalidCredentials
	}
	serviceKind := "SERVICE_NAME"
	if o.SID {
		serviceKind = "SID"
	}
	conf := &loginOptions{UserID: o.Username, Password: o.Password, ClientInfo: clientInfo{HostName: "yak", ProgramName: "yak-oracle-probe", OSUserName: "yak", DriverName: "yak-oracle-probe"}}
	conf.descriptor = fmt.Sprintf("(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=%s)(PORT=%d))(CONNECT_DATA=(%s=%s)(CID=(PROGRAM=yak-oracle-probe)(HOST=yak)(USER=yak))))", host, p, serviceKind, o.Service)
	details.Stage = "connect"
	s, e := connect(ctx, dialer, o, conf.descriptor, details)
	if e != nil {
		return e
	}
	defer s.transport.Close()
	// A cancellation closes the owned connection, interrupting all reads/writes.
	stop := context.AfterFunc(ctx, func() { s.transport.Close() })
	defer stop()
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	c := &Connection{ctx: ctx, connOption: conf, session: s, LogonMode: NoNewPass, details: details}
	if o.SysDBA {
		c.LogonMode |= 0x20
	}
	details.Stage = "network-negotiation"
	if o.Encryption == EncryptionRequired && !s.advanced {
		return fmt.Errorf("%w: required but unavailable", ErrEncryptionPolicy)
	}
	if s.advanced {
		if e = s.negotiateAdvanced(); e != nil {
			return fmt.Errorf("oracle network negotiation: %w", e)
		}
	}
	details.Encryption, details.Integrity = s.encryptionName, s.integrityName
	if details.Encryption == "" {
		details.Encryption = "none"
	}
	if details.Integrity == "" {
		details.Integrity = "none"
	}
	details.Stage = "protocol-negotiation"
	if e = c.protocolNegotiation(); e != nil {
		return fmt.Errorf("oracle protocol negotiation: %w", e)
	}
	details.Stage = "datatype-negotiation"
	if e = c.dataTypeNegotiation(); e != nil {
		return fmt.Errorf("oracle datatype negotiation: %w", e)
	}
	return c.doAuth()
}
func (c *Connection) protocolNegotiation() error {
	c.tcpNego = &TCPNego{conn: c}
	if err := c.tcpNego.write(); err != nil {
		return err
	}
	if err := c.tcpNego.readMessage(); err != nil {
		return err
	}
	c.tcpNego.ServerFlags |= 2
	return c.session.err
}
func (c *Connection) dataTypeNegotiation() error {
	c.dataNego = buildTypeNego(c.tcpNego, c)
	if e := c.dataNego.write(); e != nil {
		return e
	}
	_, e := c.dataNego.read()
	return e
}
