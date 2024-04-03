package bruteutils

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

type smtpPlainAuth struct {
	identity, username, password string
	host                         string
}

// SMTPPlainAuth like smtp.SMTPPlainAuth but remove Start check
func SMTPPlainAuth(identity, username, password, host string) smtp.Auth {
	return &smtpPlainAuth{identity, username, password, host}
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}

func (a *smtpPlainAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *smtpPlainAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected server challenge")
	}
	return nil, nil
}

// loginAuth
type loginAuth struct {
	username, password string
}

func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("Unknown fromServer")
		}
	}
	return nil, nil
}

func newFakeMail() string {
	return fmt.Sprintf("%s@%s.com", utils.RandStringBytes(16), utils.RandStringBytes(6))
}

// SMTPAuthAndSendMail use netx.Dial instead of net.Dial, and check auth method, so do not use smtp.SendMail
// Manually test with https://mailtrap.io
func SMTPAuthAndSendMail(target, username, password string, needAuth bool) (bool, error) {
	host, _, _ := utils.ParseStringToHostPort(target)
	fakeSenderMail := newFakeMail()
	fakeReceiverMail := newFakeMail()

	conn, err := netx.DialTimeout(defaultTimeout, target)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return false, dialError
	}

	// tls
	ok, _ := client.Extension("STARTTLS")
	if ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return false, dialError
		}
	}

	if needAuth {
		var auth smtp.Auth
		if ok, ext := client.Extension("AUTH"); ok {
			// use strings.Contains because some smtp server may return "AUTH PLAIN LOGIN", include multiple auth methods
			if strings.Contains(ext, "PLAIN") {
				auth = SMTPPlainAuth(utils.RandStringBytes(16), username, password, host)
			} else if strings.Contains(ext, "LOGIN") {
				auth = LoginAuth(username, password)
			} else if strings.Contains(ext, "CRAM-MD5") {
				auth = smtp.CRAMMD5Auth(username, password)
			}
		}
		if auth != nil {
			if err = client.Auth(auth); err != nil {
				return false, err
			}
		}
	}

	if err = client.Mail(fakeSenderMail); err != nil {
		return false, err
	}
	if err = client.Rcpt(fakeReceiverMail); err != nil {
		return false, err
	}
	w, err := client.Data()
	if err != nil {
		return false, err
	}
	if _, err = w.Write([]byte(utils.RandStringBytes(50))); err != nil {
		return false, err
	}
	if err = client.Close(); err != nil {
		return false, err
	}

	return true, nil
}

var smtpAuth = &DefaultServiceAuthInfo{
	ServiceName:      "smtp",
	DefaultPorts:     "25",
	DefaultUsernames: CommonUsernames,
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		target := fixToTarget(i.Target, 25)
		res := i.Result()
		ok, err := SMTPAuthAndSendMail(target, i.Username, i.Password, false)
		if err != nil && errors.Is(err, dialError) {
			res.Finished = true
		}
		res.Ok = ok
		return res
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		target := fixToTarget(i.Target, 25)
		res := i.Result()
		ok, err := SMTPAuthAndSendMail(target, i.Username, i.Password, true)
		if err != nil {
			res.Finished = true
			return res
		}
		res.Ok = ok
		return res
	},
}
