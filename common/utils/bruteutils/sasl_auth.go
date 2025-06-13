package bruteutils

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/xdg-go/stringprep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/sasl"
	"hash"
	"math/rand"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/xdg-go/scram"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	_ smtp.Auth   = (*plainAuth)(nil)
	_ smtp.Auth   = (*loginAuth)(nil)
	_ smtp.Auth   = (*scramAuth)(nil)
	_ sasl.Client = (*cramSASLClient)(nil)
	_ sasl.Client = (*scramSASLCLient)(nil)
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

type plainAuth struct {
	identity, username, password string
	host                         string
}

// PlainAuth like smtp.PlainAuth but remove Start check
func PlainAuth(identity, username, password, host string) smtp.Auth {
	var userprep, pwdprep string
	var err error
	if userprep, err = stringprep.SASLprep.Prepare(username); err != nil {
		log.Errorf("Error SASLprepping username '%s': %v", username, err)
		return &plainAuth{identity, username, password, host}
	}
	if pwdprep, err = stringprep.SASLprep.Prepare(password); err != nil {
		log.Errorf("Error SASLprepping password '%s': %v", password, err)
		return &plainAuth{identity, username, password, host}
	}
	if username == identity {
		return &plainAuth{userprep, userprep, pwdprep, host}
	}
	return &plainAuth{identity, userprep, pwdprep, host}
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

type cramSASLClient struct {
	Username string
	Secret   string
	mech     string
	hashFunc func() hash.Hash
}

var _ sasl.Client = &cramSASLClient{}

func (c *cramSASLClient) Start() (mech string, ir []byte, err error) {
	return
}

func (c *cramSASLClient) Next(challenge []byte) (response []byte, err error) {
	d := hmac.New(c.hashFunc, []byte(c.Secret))
	d.Write(challenge)
	s := make([]byte, 0, d.Size())
	return []byte(fmt.Sprintf("%s %x", c.Username, d.Sum(s))), nil
}

func NewCramClient(mech, username, secret string) sasl.Client {
	var hashFunc func() hash.Hash
	switch strings.ToLower(mech) {
	case "md5":
		hashFunc = func() hash.Hash { return md5.New() }
	case "sha1":
		hashFunc = func() hash.Hash { return sha1.New() }
	case "sha256":
		hashFunc = func() hash.Hash { return sha256.New() }
	default:
		return nil
	}
	var userprep, secretprep string
	var err error
	if userprep, err = stringprep.SASLprep.Prepare(username); err != nil {
		log.Errorf("Error SASLprepping username '%s': %v", username, err)
		return nil
	}
	if secretprep, err = stringprep.SASLprep.Prepare(secret); err != nil {
		log.Errorf("Error SASLprepping password '%s': %v", secret, err)
		return nil
	}
	return &cramSASLClient{userprep, secretprep, mech, hashFunc}
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

// DigestMD5Mechanism corresponds to PLAIN SASL mechanism
type DigestMD5Mechanism struct {
	service    string
	identity   string
	username   string
	password   string
	host       string
	nonceCount int
	cnonce     string
	nonce      string
	keyHash    string
	auth       string
}

// NewDigestMD5Mechanism returns a new PlainMechanism
func NewDigestMD5Mechanism(service string, username string, password string) *DigestMD5Mechanism {
	var userprep, passprep string
	var err error
	if userprep, err = stringprep.SASLprep.Prepare(username); err != nil {
		log.Errorf("Error SASLprepping username '%s': %v", username, err)
		return nil
	}
	if passprep, err = stringprep.SASLprep.Prepare(password); err != nil {
		log.Errorf("Error SASLprepping password '%s': %v", password, err)
		return nil
	}
	return &DigestMD5Mechanism{
		service:  service,
		username: userprep,
		password: passprep,
	}
}

func (m *DigestMD5Mechanism) start() ([]byte, error) {
	return m.Step(nil)
}

func (m *DigestMD5Mechanism) randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// parseChallenge turns the challenge string into a map
func (m *DigestMD5Mechanism) parseChallenge(challenge []byte) map[string]string {
	s := string(challenge)

	c := make(map[string]string)

	for len(s) > 0 {
		eq := strings.Index(s, "=")
		key := s[:eq]
		s = s[eq+1:]
		isQuoted := false
		search := ","
		if s[0:1] == "\"" {
			isQuoted = true
			search = "\""
			s = s[1:]
		}
		co := strings.Index(s, search)
		if co == -1 {
			co = len(s)
		}
		val := s[:co]
		if isQuoted && len(s) > len(val)+1 {
			s = s[co+2:]
		} else if co < len(s) {
			s = s[co+1:]
		} else {
			s = ""
		}
		c[key] = val
	}

	return c
}

func (m *DigestMD5Mechanism) authenticate(digestUri string, challengeMap map[string]string) error {
	a2String := ":" + digestUri

	if m.auth != "auth" {
		a2String += ":00000000000000000000000000000000"
	}

	if m.getHash(digestUri, a2String, challengeMap) != challengeMap["rspauth"] {
		return fmt.Errorf("authenticate failed")
	}
	return nil
}

func (m *DigestMD5Mechanism) getHash(digestUri string, a2String string, c map[string]string) string {
	// Create a1: HEX(H(H(username:realm:password):nonce:cnonce:authid))
	if m.keyHash == "" {
		x := m.username + ":" + c["realm"] + ":" + m.password
		byteKeyHash := md5.Sum([]byte(x))
		m.keyHash = string(byteKeyHash[:])
	}
	a1String := []string{
		m.keyHash,
		m.nonce,
		m.cnonce,
	}

	h1 := md5.Sum([]byte(strings.Join(a1String, ":")))
	a1 := hex.EncodeToString(h1[:])

	h2 := md5.Sum([]byte(a2String))
	a2 := hex.EncodeToString(h2[:])

	// Set nonce count nc
	nc := fmt.Sprintf("%08x", m.nonceCount)

	// Create response: H(a1:nonce:nc:cnonce:qop:a2)
	r := strings.ToLower(a1) + ":" + m.nonce + ":" + nc + ":" + m.cnonce + ":" + m.auth + ":" + strings.ToLower(a2)
	hr := md5.Sum([]byte(r))

	// Convert response to hex
	response := strings.ToLower(hex.EncodeToString(hr[:]))
	return string(response)

}

func (m *DigestMD5Mechanism) Step(challenge []byte) ([]byte, error) {
	if challenge == nil {
		return nil, nil
	}

	// Create map of challenge
	c := m.parseChallenge(challenge)
	digestUri := m.service + "/" + m.host

	// Prepare response variables
	m.nonce = c["nonce"]
	m.auth = c["qop"]
	if m.nonceCount == 0 {
		m.cnonce = m.randSeq(14)
	}
	m.nonceCount++

	// Create a2: HEX(H(AUTHENTICATE:digest-uri-value:00000000000000000000000000000000))
	a2String := "AUTHENTICATE:" + digestUri

	maxBuf := ""
	if c["qop"] != "auth" {
		a2String += ":00000000000000000000000000000000"
		maxBuf = ",maxbuf=16777215"
	}
	// Set nonce count nc
	nc := fmt.Sprintf("%08x", m.nonceCount)
	// Create final response sent to server
	resp := "qop=" + c["qop"] + ",realm=" + strconv.Quote(c["realm"]) + ",username=" + strconv.Quote(m.username) + ",nonce=" + strconv.Quote(m.nonce) +
		",cnonce=" + strconv.Quote(m.cnonce) + ",nc=" + nc + ",digest-uri=" + strconv.Quote(digestUri) + ",response=" + m.getHash(digestUri, a2String, c) + maxBuf

	return []byte(resp), nil
}
