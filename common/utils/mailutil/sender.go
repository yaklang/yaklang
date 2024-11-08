package mailutil

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/netx"
	"gopkg.in/gomail.v2"
	"net"
	"net/smtp"
	"time"
)

type SMTPConfig struct {
	Server     string
	Port       int
	ConnectSSL bool

	// empty is for common case
	Identify string

	Username string
	Password string

	From string
}

func (c *SMTPConfig) GetSMTPMailSender() *SMTPMailSender {
	return &SMTPMailSender{
		config: c,
	}
}

type SMTPMailSender struct {
	config *SMTPConfig
}

func NewSMTPMailSender(c *SMTPConfig) (*SMTPMailSender, error) {
	if c == nil {
		return nil, errors.New("empty config is not allowed")
	}
	sender := &SMTPMailSender{config: c}
	return sender, nil
}

func (s *SMTPMailSender) SendWithContext(ctx context.Context, toWho string, msg *gomail.Message, cc ...string) error {
	if toWho == "" {
		return errors.New("toWho cannot be empty")
	}

	var (
		err error
	)

	conn, client, err := s.GetAuthClient(ctx)
	if err != nil {
		return errors.Errorf("failed to auth: %s", err)
	}
	defer func() {
		_ = client.Close()
		_ = conn.Close()
	}()

	var (
		from string = s.config.From
		to   string = toWho
	)

	if s.config.From == "" {
		from = fmt.Sprintf("%v", s.config.Username)
	}

	err = client.Mail(from)
	if err != nil {
		return errors.Errorf("send from who[%s] [MAIL] failed: %s", from, err)
	}

	err = client.Rcpt(to)
	if err != nil {
		return errors.Errorf("send to [%s] [RCPT] failed: %s", to, err)
	}

	for _, c := range cc {
		err = client.Rcpt(c)
		if err != nil {
			return errors.Errorf("CC Rcpt failed：%v", c)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return errors.Errorf("get transfer writer failed; %s", err)
	}
	defer func() {
		_ = writer.Close()
	}()

	_, err = msg.WriteTo(writer)
	if err != nil {
		return errors.Errorf("write msg failed: %s", err)
	}

	return nil
}

func (s *SMTPMailSender) GetAuthClient(ctx context.Context) (net.Conn, *smtp.Client, error) {
	var (
		conn net.Conn
		err  error
		addr = fmt.Sprintf("%s:%v", s.config.Server, s.config.Port)
	)
	if s.config.ConnectSSL {
		conn, err = netx.DialTLSTimeout(10*time.Second, addr, &gmtls.Config{InsecureSkipVerify: true,
			ServerName: s.config.Server})
		if err != nil {
			return nil, nil, errors.Errorf("tls dial failed:  %s", err)
		}
	} else {
		conn, err = netx.DialTCPTimeout(10*time.Second, addr)
		if err != nil {
			return nil, nil, errors.Errorf("dial failed: %s", err)
		}
	}

	client, err := smtp.NewClient(conn, s.config.Server)
	if err != nil {
		return nil, nil, errors.Errorf("create smtp client failed: %s", err)
	}

	err = client.Auth(smtp.PlainAuth(s.config.Identify, s.config.Username, s.config.Password, s.config.Server))
	if err != nil {
		return nil, nil, errors.Errorf("auth failed: %s", err)
	}

	return conn, client, nil
}

func (s *SMTPMailSender) IsAvailable(ctx context.Context) (bool, string) {
	conn, client, err := s.GetAuthClient(ctx)
	if err != nil {
		return false, fmt.Sprintf("auth failed: %v", err)
	}

	defer func() {
		_ = client.Close()
		_ = conn.Close()
	}()

	return true, ""
}
