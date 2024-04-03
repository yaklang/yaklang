package bruteutils

import (
	"crypto/hmac"
	"crypto/md5"
	"fmt"

	"github.com/emersion/go-sasl"
)

// The CRAM-MD5 mechanism name.
const CramMD5 = "CRAM-MD5"

type cramMD5Client struct {
	Username string
	Secret   string
}

var _ sasl.Client = &cramMD5Client{}

func (a *cramMD5Client) Start() (mech string, ir []byte, err error) {
	mech = CramMD5
	return
}

func (a *cramMD5Client) Next(challenge []byte) (response []byte, err error) {
	d := hmac.New(md5.New, []byte(a.Secret))
	d.Write(challenge)
	s := make([]byte, 0, d.Size())
	return []byte(fmt.Sprintf("%s %x", a.Username, d.Sum(s))), nil
}

// NewCramMD5Client implements the CRAM-MD5 authentication mechanism, as
// described in RFC 2195.
// The returned Client uses the given username and secret to authenticate to the
// server using the challenge-response mechanism.
func NewCramMD5Client(username, secret string) sasl.Client {
	return &cramMD5Client{username, secret}
}