package bruteutils

import (
	"crypto/hmac"
	"crypto/md5"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/emersion/go-sasl"
	"github.com/pkg/errors"
	"github.com/xdg-go/scram"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	_ smtp.Auth   = (*plainAuth)(nil)
	_ smtp.Auth   = (*loginAuth)(nil)
	_ smtp.Auth   = (*scramAuth)(nil)
	_ sasl.Client = (*cramMD5SASLClient)(nil)
	_ sasl.Client = (*scramSASLCLient)(nil)
)

type plainAuth struct {
	identity, username, password string
	host                         string
}

// PlainAuth like smtp.PlainAuth but remove Start check
func PlainAuth(identity, username, password, host string) smtp.Auth {
	return &plainAuth{identity, username, password, host}
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}

func (a *plainAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *plainAuth) Next(fromServer []byte, more bool) ([]byte, error) {
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

// scramAuth

type scramAuth struct {
	ID string
	*scram.ClientConversation
}

// PlainAuth like smtp.PlainAuth but remove Start check
func ScramAuth(hashID, username, password string) (smtp.Auth, error) {
	var (
		fcn scram.HashGeneratorFcn
		id  string
	)
	if strings.Contains(hashID, "SHA-1") {
		id = "SHA-1"
		fcn = scram.SHA1
	} else if strings.Contains(hashID, "SHA-256") {
		id = "SHA-256"
		fcn = scram.SHA256
	} else if strings.Contains(hashID, "SHA-512") {
		id = "SHA-512"
		fcn = scram.SHA512
	} else {
		return nil, errors.New("Unknown hashID")
	}

	client, err := fcn.NewClient(username, password, "")
	if err != nil {
		return nil, err
	}
	conv := client.NewConversation()
	return &scramAuth{
		ID:                 id,
		ClientConversation: conv,
	}, nil
}

func (a *scramAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return fmt.Sprintf("SCRAM-%s", a.ID), []byte{}, nil
}

func (a *scramAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if a.ClientConversation.Done() {
		return nil, nil
	}
	msg, err := a.ClientConversation.Step(string(fromServer))
	return []byte(msg), err
}

// SASL Client for IMAP

type cramMD5SASLClient struct {
	Username string
	Secret   string
}

var _ sasl.Client = &cramMD5SASLClient{}

func (c *cramMD5SASLClient) Start() (mech string, ir []byte, err error) {
	mech = "CRAM-MD5"
	return
}

func (c *cramMD5SASLClient) Next(challenge []byte) (response []byte, err error) {
	d := hmac.New(md5.New, []byte(c.Secret))
	d.Write(challenge)
	s := make([]byte, 0, d.Size())
	return []byte(fmt.Sprintf("%s %x", c.Username, d.Sum(s))), nil
}

// NewCramMD5Client implements the CRAM-MD5 authentication mechanism, as
// described in RFC 2195.
// The returned Client uses the given username and secret to authenticate to the
// server using the challenge-response mechanism.
func NewCramMD5Client(username, secret string) sasl.Client {
	return &cramMD5SASLClient{username, secret}
}

type scramSASLCLient struct {
	ID string
	*scram.ClientConversation
}

func NewScramClient(hashID, username, password string) (sasl.Client, error) {
	var (
		fcn scram.HashGeneratorFcn
		id  string
	)
	if strings.Contains(hashID, "SHA-1") {
		id = "SHA-1"
		fcn = scram.SHA1
	} else if strings.Contains(hashID, "SHA-256") {
		id = "SHA-256"
		fcn = scram.SHA256
	} else if strings.Contains(hashID, "SHA-512") {
		id = "SHA-512"
		fcn = scram.SHA512
	} else {
		return nil, errors.New("Unknown hashID")
	}

	client, err := fcn.NewClient(username, password, "")
	if err != nil {
		return nil, err
	}
	conv := client.NewConversation()
	return &scramSASLCLient{
		ID:                 id,
		ClientConversation: conv,
	}, nil
}

func (c *scramSASLCLient) Start() (mech string, ir []byte, err error) {
	resp, err := c.ClientConversation.Step("")
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("SCRAM-%s", c.ID), []byte(resp), nil
}

func (c *scramSASLCLient) Next(challenge []byte) (response []byte, err error) {
	if c.ClientConversation.Done() {
		return nil, nil
	}
	msg, err := c.ClientConversation.Step(string(challenge))
	if c.ClientConversation.Valid() {
		// return random base64 message because continued-message want valid base64 message to finish authentication, go-imap with change empty string to "=" which is invalid base64 message in some server
		msg = codec.EncodeBase64(utils.RandStringBytes(10))
	}
	return []byte(msg), err
}
