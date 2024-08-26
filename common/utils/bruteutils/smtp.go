package bruteutils

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
)

func newFakeMail() string {
	return fmt.Sprintf("%s@%s.com", utils.RandStringBytes(16), utils.RandStringBytes(6))
}

// SMTPAuthAndSendMail use netx.Dial instead of net.Dial, and check auth method, so do not use smtp.SendMail
// Manually test with https://mailtrap.io
func SMTPAuthAndSendMail(target, username, password string, needAuth bool) (bool, error) {
	host, _, _ := utils.ParseStringToHostPort(target)
	fakeSenderMail := newFakeMail()
	fakeReceiverMail := newFakeMail()

	conn, err := defaultDialer.DialContext(utils.TimeoutContext(defaultTimeout), "tcp", target)
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
		if err := client.StartTLS(&tls.Config{
			ServerName:         host,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
			Renegotiation:      tls.RenegotiateFreelyAsClient,
		}); err != nil {
			return false, dialError
		}
	}

	if needAuth {
		var auth smtp.Auth
		if ok, ext := client.Extension("AUTH"); ok {
			// use strings.Contains because some smtp server may return "AUTH PLAIN LOGIN", include multiple auth methods
			if strings.Contains(ext, "PLAIN") {
				auth = PlainAuth(utils.RandStringBytes(16), username, password, host)
			} else if strings.Contains(ext, "LOGIN") {
				auth = LoginAuth(username, password)
			} else if strings.Contains(ext, "CRAM-MD5") {
				auth = smtp.CRAMMD5Auth(username, password)
			} else if strings.Contains(ext, "SCRAM") {
				auth, err = ScramAuth(ext, username, password)
				if err != nil {
					return false, err
				}
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
