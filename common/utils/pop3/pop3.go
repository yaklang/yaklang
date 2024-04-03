// The MIT License (MIT)
// Copyright (c) 2021, Kailash Nadh
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
// Origin: https://github.com/knadh/go-pop3

// Package pop3 is a simple POP3 e-mail client library.
package pop3

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// Client implements a Client e-mail client.
type Client struct {
	opt        Opt
	dialer     Dialer
	serverName string
	tls        bool
}

// Conn is a stateful connection with the POP3 server/
type Conn struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

// Opt represents the client configuration.
type Opt struct {
	Host string `json:"host"`
	Port int    `json:"port"`

	// Default is 3 seconds.
	DialTimeout time.Duration `json:"dial_timeout"`
	Dialer      Dialer        `json:"-"`

	TLSEnabled    bool `json:"tls_enabled"`
	TLSSkipVerify bool `json:"tls_skip_verify"`
}

type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

// MessageID contains the ID and size of an individual message.
type MessageID struct {
	// ID is the numerical index (non-unique) of the message.
	ID   int
	Size int

	// UID is only present if the response is to the UIDL command.
	UID string
}

var (
	lineBreak = []byte("\r\n")

	respChallengeOK     = []byte("+")     // `+` without additional info for SASL challenge
	respChallengeOKInfo = []byte("+ ")    // `+ <info>` for SASL challenge
	respOK              = []byte("+OK")   // `+OK` without additional info
	respOKInfo          = []byte("+OK ")  // `+OK <info>`
	respErr             = []byte("-ERR")  // `-ERR` without additional info
	respErrInfo         = []byte("-ERR ") // `-ERR <info>`
)

// New returns a new client object using an existing connection.
func New(opt Opt) *Client {
	if opt.DialTimeout < time.Millisecond {
		opt.DialTimeout = time.Second * 3
	}

	c := &Client{
		opt:    opt,
		dialer: opt.Dialer,
	}

	if c.dialer == nil {
		c.dialer = &net.Dialer{Timeout: opt.DialTimeout}
	}

	return c
}

// NewConn creates and returns live POP3 server connection.
func (c *Client) NewConn() (*Conn, error) {
	addr := fmt.Sprintf("%s:%d", c.opt.Host, c.opt.Port)
	conn, err := c.dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	// No TLS.
	if c.opt.TLSEnabled {
		// Skip TLS host verification.
		tlsCfg := tls.Config{}
		if c.opt.TLSSkipVerify {
			tlsCfg.InsecureSkipVerify = c.opt.TLSSkipVerify
		} else {
			tlsCfg.ServerName = c.opt.Host
		}

		conn = tls.Client(conn, &tlsCfg)
		c.tls = true
	}

	pCon := &Conn{
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
	}

	// Verify the connection by reading the welcome +OK greeting.
	if _, err := pCon.ReadOne(); err != nil {
		return nil, err
	}

	return pCon, nil
}

// Send sends a POP3 command to the server. The given comand is suffixed with "\r\n".
func (c *Conn) Send(b string) error {
	c.conn.SetWriteDeadline(time.Now().Add(time.Second * 5))
	if _, err := c.w.WriteString(b + "\r\n"); err != nil {
		return err
	}
	return c.w.Flush()
}

func (c *Conn) ChallengeCmd(cmd string, args ...interface{}) (buf *bytes.Buffer, isChallenge bool, err error) {
	var cmdLine string

	// Repeat a %v to format each arg.
	if len(args) > 0 {
		format := " " + strings.TrimRight(strings.Repeat("%v ", len(args)), " ")
		// CMD arg1 argn ...\r\n
		cmdLine = fmt.Sprintf(cmd+format, args...)
	} else {
		cmdLine = cmd
	}

	if err := c.Send(cmdLine); err != nil {
		return nil, false, err
	}

	// Read the first line of response to get the +OK/-ERR status.
	b, isChallenge, err := c.ReadOneWithChallenge()
	return bytes.NewBuffer(b), isChallenge, err
}

// Cmd sends a command to the server. POP3 responses are either single line or multi-line.
// The first line always with -ERR in case of an error or +OK in case of a successful operation.
// OK+ is always followed by a response on the same line which is either the actual response data
// in case of single line responses, or a help message followed by multiple lines of actual response
// data in case of multiline responses.
// See https://www.shellhacks.com/retrieve-email-pop3-server-command-line/ for examples.
func (c *Conn) Cmd(cmd string, isMulti bool, args ...interface{}) (*bytes.Buffer, error) {
	var cmdLine string

	// Repeat a %v to format each arg.
	if len(args) > 0 {
		format := " " + strings.TrimRight(strings.Repeat("%v ", len(args)), " ")

		// CMD arg1 argn ...\r\n
		cmdLine = fmt.Sprintf(cmd+format, args...)
	} else {
		cmdLine = cmd
	}

	if err := c.Send(cmdLine); err != nil {
		return nil, err
	}

	// Read the first line of response to get the +OK/-ERR status.
	b, err := c.ReadOne()
	if err != nil {
		return nil, err
	}

	// Single line response.
	if !isMulti {
		return bytes.NewBuffer(b), err
	}

	buf, err := c.ReadAll()
	return buf, err
}

// ReadOne reads a single line response from the conn.
func (c *Conn) ReadOne() ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(time.Second * 5))

	b, _, err := c.r.ReadLine()
	if err != nil {
		return nil, err
	}

	r, _, err := parseResp(b)
	return r, err
}

// ReadOne reads a single line response from the conn.
func (c *Conn) ReadOneWithChallenge() ([]byte, bool, error) {
	c.conn.SetReadDeadline(time.Now().Add(time.Second * 5))

	b, _, err := c.r.ReadLine()
	if err != nil {
		return nil, false, err
	}

	return parseResp(b)
}

// ReadAll reads all lines from the connection until the POP3 multiline terminator "." is encountered
// and returns a bytes.Buffer of all the read lines.
func (c *Conn) ReadAll() (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}

	for {
		c.conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		b, _, err := c.r.ReadLine()
		if err != nil {
			return nil, err
		}

		// "." indicates the end of a multi-line response.
		if bytes.Equal(b, []byte(".")) {
			break
		}

		if _, err := buf.Write(b); err != nil {
			return nil, err
		}
		if _, err := buf.Write(lineBreak); err != nil {
			return nil, err
		}
	}

	return buf, nil
}

// SASLAuth authenticates a client using the provided authentication mechanism.
// A failed authentication closes the connection.
// Only servers that advertise the SASL capability can be authenticated using this method.
func (c *Conn) SASLAuth(a smtp.Auth) error {
	mech, resp, err := a.Start(nil)
	if err != nil {
		c.Quit()
		return err
	}
	var (
		b           *bytes.Buffer
		resp64      string
		msg         []byte
		isChallenge bool
	)

	b, isChallenge, err = c.ChallengeCmd(fmt.Sprintf("AUTH %s", mech))
	if err == nil {
		if !isChallenge {
			return errors.New("unexpected response from server")
		}

		// for PLAIN auth
		if len(resp) > 0 {
			resp64 = codec.EncodeBase64(resp)
			b, isChallenge, err = c.ChallengeCmd(resp64)
		}
	}

	for err == nil {
		msg = b.Bytes()

		if isChallenge && len(msg) > 0 {
			msg, err = codec.DecodeBase64(string(msg))
		}

		if err == nil {
			resp, err = a.Next(msg, isChallenge)
		} else {
			c.Quit()
			break
		}

		if resp == nil {
			break
		}

		resp64 = codec.EncodeBase64(resp)
		b, isChallenge, err = c.ChallengeCmd(resp64)
	}
	return err
}

// Auth authenticates the given credentials with the server.
func (c *Conn) Auth(user, password string) error {
	if err := c.User(user); err != nil {
		return err
	}

	if err := c.Pass(password); err != nil {
		return err
	}

	// Issue a NOOP to force the server to respond to the auth.
	// Couresy: github.com/TheCreeper/go-pop3
	return c.Noop()
}

// User sends the username to the server.
func (c *Conn) User(s string) error {
	_, err := c.Cmd("USER", false, s)
	return err
}

// Pass sends the password to the server.
func (c *Conn) Pass(s string) error {
	_, err := c.Cmd("PASS", false, s)
	return err
}

// Stat returns the number of messages and their total size in bytes in the inbox.
func (c *Conn) Stat() (int, int, error) {
	b, err := c.Cmd("STAT", false)
	if err != nil {
		return 0, 0, err
	}

	// count size
	f := bytes.Fields(b.Bytes())

	// Total number of messages.
	count, err := strconv.Atoi(string(f[0]))
	if err != nil {
		return 0, 0, err
	}
	if count == 0 {
		return 0, 0, nil
	}

	// Total size of all messages in bytes.
	size, err := strconv.Atoi(string(f[1]))
	if err != nil {
		return 0, 0, err
	}

	return count, size, nil
}

// List returns a list of (message ID, message Size) pairs.
// If the optional msgID > 0, then only that particular message is listed.
// The message IDs are sequential, 1 to N.
func (c *Conn) List(msgID int) ([]MessageID, error) {
	var (
		buf *bytes.Buffer
		err error
	)

	if msgID <= 0 {
		// Multiline response listing all messages.
		buf, err = c.Cmd("LIST", true)
	} else {
		// Single line response listing one message.
		buf, err = c.Cmd("LIST", false, msgID)
	}
	if err != nil {
		return nil, err
	}

	var (
		out   []MessageID
		lines = bytes.Split(buf.Bytes(), lineBreak)
	)

	for _, l := range lines {
		// id size
		f := bytes.Fields(l)
		if len(f) == 0 {
			break
		}

		id, err := strconv.Atoi(string(f[0]))
		if err != nil {
			return nil, err
		}

		size, err := strconv.Atoi(string(f[1]))
		if err != nil {
			return nil, err
		}

		out = append(out, MessageID{ID: id, Size: size})
	}

	return out, nil
}

// Uidl returns a list of (message ID, message UID) pairs. If the optional msgID
// is > 0, then only that particular message is listed. It works like Top() but only works on
// servers that support the UIDL command. Messages size field is not available in the UIDL response.
func (c *Conn) Uidl(msgID int) ([]MessageID, error) {
	var (
		buf *bytes.Buffer
		err error
	)

	if msgID <= 0 {
		// Multiline response listing all messages.
		buf, err = c.Cmd("UIDL", true)
	} else {
		// Single line response listing one message.
		buf, err = c.Cmd("UIDL", false, msgID)
	}
	if err != nil {
		return nil, err
	}

	var (
		out   []MessageID
		lines = bytes.Split(buf.Bytes(), lineBreak)
	)

	for _, l := range lines {
		// id size
		f := bytes.Fields(l)
		if len(f) == 0 {
			break
		}

		id, err := strconv.Atoi(string(f[0]))
		if err != nil {
			return nil, err
		}

		out = append(out, MessageID{ID: id, UID: string(f[1])})
	}

	return out, nil
}

// Retr downloads a message by the given msgID, parses it and returns it as a
// emersion/go-message.message.Entity object.
func (c *Conn) Retr(msgID int) (*message.Entity, error) {
	b, err := c.Cmd("RETR", true, msgID)
	if err != nil {
		return nil, err
	}

	m, err := message.Read(b)
	if err != nil {
		if !message.IsUnknownCharset(err) {
			return nil, err
		}
	}

	return m, nil
}

// RetrRaw downloads a message by the given msgID and returns the raw []byte
// of the entire message.
func (c *Conn) RetrRaw(msgID int) (*bytes.Buffer, error) {
	b, err := c.Cmd("RETR", true, msgID)
	return b, err
}

// Top retrieves a message by its ID with full headers and numLines lines of the body.
func (c *Conn) Top(msgID int, numLines int) (*message.Entity, error) {
	b, err := c.Cmd("TOP", true, msgID, numLines)
	if err != nil {
		return nil, err
	}

	m, err := message.Read(b)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Dele deletes one or more messages. The server only executes the
// deletions after a successful Quit().
func (c *Conn) Dele(msgID ...int) error {
	for _, id := range msgID {
		_, err := c.Cmd("DELE", false, id)
		if err != nil {
			return err
		}
	}
	return nil
}

// Rset clears the messages marked for deletion in the current session.
func (c *Conn) Rset() error {
	_, err := c.Cmd("RSET", false)
	return err
}

// Noop issues a do-nothing NOOP command to the server. This is useful for
// prolonging open connections.
func (c *Conn) Noop() error {
	_, err := c.Cmd("NOOP", false)
	return err
}

// Quit sends the QUIT command to server and gracefully closes the connection.
// Message deletions (DELE command) are only excuted by the server on a graceful
// quit and close.
func (c *Conn) Quit() error {
	defer c.conn.Close()

	if _, err := c.Cmd("QUIT", false); err != nil {
		return err
	}

	return nil
}

// CAPA returns the capabilities of the server.
func (c *Conn) CAPA() (map[string]string, error) {
	b, err := c.Cmd("CAPA", true)
	if err != nil {
		return nil, err
	}

	var (
		caps  = make(map[string]string)
		lines = strings.Split(b.String(), "\r\n")
	)

	for _, l := range lines {
		if l == "" {
			break
		}
		cap, value, _ := strings.Cut(l, " ")
		caps[cap] = value
	}

	return caps, nil
}

func (c *Conn) StartTLS(cfg *tls.Config) error {
	b, err := c.Cmd("STLS", false)
	_ = b
	if err != nil {
		return err
	}

	// Upgrade the connection to TLS.
	c.conn = tls.Client(c.conn, cfg)
	c.r = bufio.NewReader(c.conn)
	c.w = bufio.NewWriter(c.conn)

	return nil
}

// parseResp checks if the response is an error that starts with `-ERR`
// and returns an error with the message that succeeds the error indicator.
// For success `+OK` messages, it returns the remaining response bytes.
// For challenge-response SASL auth, it returns the challenge bytes.
func parseResp(b []byte) (msg []byte, isChallenge bool, err error) {
	if len(b) == 0 {
		return nil, false, nil
	}

	if bytes.Equal(b, respChallengeOK) {
		return nil, true, nil
	} else if bytes.Equal(b, respOK) {
		return nil, false, nil
	} else if bytes.HasPrefix(b, respChallengeOKInfo) {
		return bytes.TrimPrefix(b, respChallengeOKInfo), true, nil
	} else if bytes.HasPrefix(b, respOKInfo) {
		return bytes.TrimPrefix(b, respOKInfo), false, nil
	} else if bytes.Equal(b, respErr) {
		return nil, false, errors.New("unknown error (no info specified in response)")
	} else if bytes.HasPrefix(b, respErrInfo) {
		return nil, false, errors.New(string(bytes.TrimPrefix(b, respErrInfo)))
	} else {
		return nil, false, fmt.Errorf("unknown response: %s. Neither -ERR, nor +OK", string(b))
	}
}
