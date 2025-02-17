package bruteutils

import (
	"errors"
	"strings"

	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
)

// IMAPAuth use netx.Dial instead of net.Dial, and check auth method
// Manually test with https://app.mailslurp.com/dashboard/
func IMAPAuth(target, username, password string, needAuth bool) (bool, error) {
	conn, err := defaultDialer.Dial("TCP", target)
	if err != nil {
		return false, dialError
	}
	c := imapclient.New(conn, &imapclient.Options{})

	// ! no need to handle StartTLS because go-imap will handle it
	defer c.Close()
	if needAuth {
		// check if server support SASL capability
		authMechanisms := c.Caps().AuthMechanisms()
		if len(authMechanisms) > 0 {
			// use sasl.Client instead of smtp.Auth
			var authClient sasl.Client
			for _, ext := range authMechanisms {
				switch ext {
				case "CRAM-MD5":
					authClient = NewCramMD5Client(username, password)
				case "LOGIN":
					authClient = sasl.NewLoginClient(username, password)
				case "PLAIN":
					// use empty identity instead of random string because maybe some server will occur error
					authClient = sasl.NewPlainClient("", username, password)
				}
				if strings.Contains(ext, "SCRAM") {
					authClient, err = NewScramClient(ext, username, password)
					if err != nil {
						return false, err
					}
				}

				if authClient != nil {
					break
				}
			}
			if authClient != nil {
				if err := c.Authenticate(authClient); err != nil {
					return false, err
				}
			}
		} else {
			// use imap Login command to auth
			if err := c.Login(username, password).Wait(); err != nil {
				return false, err
			}
			defer c.Logout().Wait()
		}
	}

	if err := c.List("", "%", nil).Wait(); err != nil {
		return false, err
	}
	return true, nil
}

var imapAuth = &DefaultServiceAuthInfo{
	ServiceName:      "imap",
	DefaultPorts:     "143",
	DefaultUsernames: CommonUsernames,
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		target := fixToTarget(i.Target, 143)
		res := i.Result()
		ok, err := IMAPAuth(target, i.Username, i.Password, false)
		if err != nil && errors.Is(err, dialError) {
			res.Finished = true
		}
		res.Ok = ok
		return res
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		target := fixToTarget(i.Target, 143)
		res := i.Result()
		ok, err := IMAPAuth(target, i.Username, i.Password, true)
		if err != nil && errors.Is(err, dialError) {
			res.Finished = true
			return res
		}
		res.Ok = ok
		return res
	},
}
