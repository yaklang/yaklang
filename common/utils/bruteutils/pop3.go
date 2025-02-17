package bruteutils

import (
	"crypto/tls"
	"errors"
	"net/smtp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pop3"
)

// Manually test with https://mailtrap.io
func POP3Auth(target, username, password string, needAuth bool) (bool, error) {
	host, port, _ := utils.ParseStringToHostPort(target)
	p := pop3.New(pop3.Opt{
		Host:   host,
		Port:   port,
		Dialer: defaultDialer,
	})

	c, err := p.NewConn()
	if err != nil {
		return false, dialError
	}
	defer c.Quit()
	caps, err := c.CAPA()
	if _, ok := caps["STLS"]; ok {
		if err := c.StartTLS(&tls.Config{
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
		// use smtp.Auth interface, because some pop3 server may use sasl auth
		var auth smtp.Auth
		// check if server support SASL capability
		if ext, ok := caps["SASL"]; ok {
			// use strings.Contains because some pop3 server may return "AUTH PLAIN LOGIN", include multiple auth methods
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
			if auth != nil {
				if err = c.SASLAuth(auth); err != nil {
					return false, err
				}
			}
		} else {
			// use pop3 USER PASS command to auth
			if err := c.Auth(username, password); err != nil {
				return false, err
			}
		}
	}
	_, _, err = c.Stat()
	if err != nil {
		return false, err
	}
	return true, nil
}

var pop3Auth = &DefaultServiceAuthInfo{
	ServiceName:      "pop3",
	DefaultPorts:     "110",
	DefaultUsernames: CommonUsernames,
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		target := fixToTarget(i.Target, 110)
		res := i.Result()
		ok, err := POP3Auth(target, i.Username, i.Password, false)
		if err != nil && errors.Is(err, dialError) {
			res.Finished = true
		}
		res.Ok = ok
		return res
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		target := fixToTarget(i.Target, 110)
		res := i.Result()
		ok, err := POP3Auth(target, i.Username, i.Password, true)
		if err != nil && errors.Is(err, dialError) {
			res.Finished = true
			return res
		}
		res.Ok = ok
		return res
	},
}
