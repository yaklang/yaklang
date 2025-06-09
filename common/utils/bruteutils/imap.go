package bruteutils

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
	"github.com/yaklang/yaklang/common/utils/sasl"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	AUTH_CLEAR = iota
	AUTH_LOGIN
	AUTH_PLAIN
	// AUTH_CRAM_MD5 Challenge Response Authentication Mechanism with MD5
	AUTH_CRAM_MD5
	AUTH_CRAM_SHA1
	AUTH_CRAM_SHA256
	AUTH_DIGEST_MD5
	// AUTH_SCRAM_SHA1 Salted Challenge Response Authentication Mechanism with SHA-1
	AUTH_SCRAM_SHA1
	AUTH_SCRAM_SHA256
	AUTH_NTLM
)

const (
	MAX_WAIT_TIME   = time.Second * 32
	MAX_STABLE_TIME = time.Millisecond * 300
)

// IMAP 响应类型
type IMAPResponse struct {
	Tag     string
	Status  string
	Message string
	Raw     string
}

// IMAP 客户端结构
type IMAPClient struct {
	conn         net.Conn
	reader       *bufio.Reader
	counter      int
	caps         []string
	authStrategy int
	host         string
}

// 创建新的 IMAP 客户端
func NewIMAPClient(conn net.Conn, host string) *IMAPClient {
	return &IMAPClient{
		conn:         conn,
		reader:       bufio.NewReader(conn),
		counter:      1,
		authStrategy: AUTH_CLEAR,
		host:         host,
	}
}

// 读取服务器响应
func (c *IMAPClient) readResponse() (*IMAPResponse, error) {
	lineBytes, err := utils.ReadUntilStable(c.reader, c.conn, MAX_WAIT_TIME, MAX_STABLE_TIME)
	if err != nil {
		return nil, err
	}
	line := string(lineBytes)
	line = strings.TrimSpace(line)
	resp := &IMAPResponse{Raw: line}

	// 解析响应格式: "* OK" 或 "1 OK" 或 "1 NO"
	parts := strings.SplitN(line, " ", 3)
	if len(parts) >= 2 {
		resp.Tag = parts[0]
		resp.Status = parts[1]
		if len(parts) >= 3 {
			resp.Message = parts[2]
		}
	}

	return resp, nil
}

// 发送命令
func (c *IMAPClient) sendCommand(cmd string) error {
	_, err := c.conn.Write([]byte(cmd + "\r\n"))
	return err
}

// 发送带标签的命令
func (c *IMAPClient) sendTaggedCommand(cmd string) error {
	tag := strconv.Itoa(c.counter)
	c.counter++
	fullCmd := tag + " " + cmd
	err := c.sendCommand(fullCmd)
	return err
}

func (c *IMAPClient) isBadRsp(rsp string) bool {
	return strings.Contains(rsp, " NO ") || strings.Contains(rsp, "failed") || strings.Contains(rsp, " BAD ")
}

func (c *IMAPClient) isBadRspGeneral(rsp string) bool {
	return strings.Contains(rsp, " NO ") || strings.Contains(rsp, "failed") || strings.Contains(rsp, " BAD ") || strings.Contains(rsp, "BYE")
}

// 解析服务器能力
func (c *IMAPClient) parseCaps(capLine string) {
	parts := strings.Fields(capLine)
	if len(parts) > 2 {
		c.caps = parts[2:] // 跳过 "* CAPABILITY"
	}
}

func (c *IMAPClient) selectAuthMechanism(cap string) {
	if !strings.Contains(cap, "=LOGIN") {
		if strings.Contains(cap, "=NTLM") {
			c.authStrategy = AUTH_NTLM
		}
		if strings.Contains(cap, "=SCRAM-SHA-1") {
			c.authStrategy = AUTH_SCRAM_SHA1
		}
		if strings.Contains(cap, "=SCRAM-SHA-256") {
			c.authStrategy = AUTH_SCRAM_SHA256
		}
		if strings.Contains(cap, "=DIGEST-MD5") {
			c.authStrategy = AUTH_DIGEST_MD5
		}
		if strings.Contains(cap, "=CRAM-SHA256") {
			c.authStrategy = AUTH_CRAM_SHA256
		}
		if strings.Contains(cap, "=CRAM-SHA1") {
			c.authStrategy = AUTH_CRAM_SHA1
		}
		if strings.Contains(cap, "=CRAM-MD5") {
			c.authStrategy = AUTH_CRAM_MD5
		}
		if strings.Contains(cap, "=PLAIN") {
			c.authStrategy = AUTH_PLAIN
		}
	} else {
		c.authStrategy = AUTH_LOGIN
	}

}

func (c *IMAPClient) IsIMAP() bool {
	if c.conn != nil {
		greetingMsg, err := utils.ReadUntilStable(c.reader, c.conn, MAX_WAIT_TIME, 300*time.Millisecond)
		if err != nil || len(greetingMsg) == 0 || (!bytes.Contains(greetingMsg, []byte("OK")) && greetingMsg[0] != '*') {
			return false
		}
		return true
	}
	return false
}

func (c *IMAPClient) GetCap() error {
	err := c.sendTaggedCommand("CAPABILITY")
	if err != nil {
		return err
	}
	rsp, err := c.readResponse()
	if err != nil {
		return err
	}
	c.parseCaps(rsp.Raw)
	c.selectAuthMechanism(rsp.Raw)
	return nil
}

func (c *IMAPClient) StartIMAP(username string, pwd string) (bool, error) {
	switch c.authStrategy {
	case AUTH_LOGIN:
		err := c.sendTaggedCommand("AUTHENTICATE LOGIN")
		if err != nil {
			return false, err
		}
		rsp, err := c.readResponse()
		if err != nil {
			return false, err
		}
		if c.isBadRsp(rsp.Raw) {
			return false, nil
		}
		b64Username := codec.EncodeBase64(username)
		err = c.sendCommand(b64Username)
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		if c.isBadRsp(rsp.Raw) {
			return false, nil
		}
		b64Pwd := codec.EncodeBase64(pwd)

		err = c.sendCommand(b64Pwd)
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		return !c.isBadRspGeneral(rsp.Raw), nil
	case AUTH_PLAIN:
		err := c.sendTaggedCommand("AUTHENTICATE PLAIN")
		if err != nil {
			return false, err
		}
		rsp, err := c.readResponse()
		if err != nil {
			return false, err
		}
		if c.isBadRsp(rsp.Raw) {
			return false, nil
		}
		_, saslPlain, _ := PlainAuth(username, username, pwd, "").Start(nil)
		err = c.sendCommand(codec.EncodeBase64(saslPlain))
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		return !c.isBadRspGeneral(rsp.Raw), nil
	case AUTH_CRAM_MD5, AUTH_CRAM_SHA1, AUTH_CRAM_SHA256:
		var err error
		var authMsg string
		switch c.authStrategy {
		case AUTH_CRAM_MD5:
			err = c.sendTaggedCommand("AUTHENTICATE CRAM-MD5")
		case AUTH_CRAM_SHA1:

			err = c.sendTaggedCommand("AUTHENTICATE CRAM-SHA1")
		case AUTH_CRAM_SHA256:
			err = c.sendTaggedCommand("AUTHENTICATE CRAM-SHA256")
		default:
			return false, nil
		}
		if err != nil {
			return false, err
		}
		rsp, err := c.readResponse()
		if err != nil {
			return false, err
		}
		// use c.isBadRspGeneral() to follow the same check manner in hydra
		if c.isBadRspGeneral(rsp.Raw) {
			return false, nil
		}
		if len(rsp.Raw) <= 2 {
			return false, errors.New("invalid CRAM server rsp length")
		}
		decodedRsp, err := codec.DecodeBase64(rsp.Raw[2:])
		if err != nil {
			return false, errors.New("invalid CRAM server rsp base64 decode failed")
		}
		switch c.authStrategy {
		case AUTH_CRAM_MD5:
			client := NewCramClient("md5", username, pwd)
			if client == nil {
				return false, errors.New("invalid CRAM client config")
			}
			authBytes, _ := client.Next(decodedRsp)
			authMsg = codec.EncodeBase64(authBytes)
		case AUTH_CRAM_SHA1:
			client := NewCramClient("sha1", username, pwd)
			if client == nil {
				return false, errors.New("invalid CRAM client config")
			}
			authBytes, _ := client.Next(decodedRsp)
			authMsg = codec.EncodeBase64(authBytes)
		case AUTH_CRAM_SHA256:
			client := NewCramClient("sha256", username, pwd)
			if client == nil {
				return false, errors.New("invalid CRAM client config")
			}
			authBytes, _ := client.Next(decodedRsp)
			authMsg = codec.EncodeBase64(authBytes)
		default:
			return false, nil
		}
		if len(authMsg) > 250 {
			authMsg = authMsg[:250]
		}
		err = c.sendCommand(authMsg)
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		return !c.isBadRspGeneral(rsp.Raw), nil
	case AUTH_DIGEST_MD5:
		err := c.sendTaggedCommand("AUTHENTICATE DIGEST-MD5")
		rsp, err := c.readResponse()
		if err != nil {
			return false, err
		}
		// use c.isBadRspGeneral() to follow the same check manner in hydra
		if c.isBadRspGeneral(rsp.Raw) {
			return false, nil
		}
		if len(rsp.Raw) <= 2 {
			return false, errors.New("invalid DIGEST-MD5 server rsp length")
		}
		decodedRsp, err := codec.DecodeBase64(rsp.Raw[2:])
		if err != nil {
			return false, errors.New("invalid digest md5 server rsp base64 decode failed")
		}
		client := NewDigestMD5Mechanism("imap", username, pwd)
		if client == nil {
			return false, errors.New("invalid DIGEST-MD5 client config")
		}
		client.host = c.host
		authBytes, err := client.Step(decodedRsp)
		if err != nil {
			return false, err
		}
		authMsg := codec.EncodeBase64(authBytes)
		if len(authMsg) > 250 {
			authMsg = authMsg[:250]
		}
		err = c.sendCommand(authMsg)
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		return !c.isBadRspGeneral(rsp.Raw), nil
	case AUTH_SCRAM_SHA1, AUTH_SCRAM_SHA256:
		var err error
		var client sasl.Client
		switch c.authStrategy {
		case AUTH_SCRAM_SHA1:
			err = c.sendTaggedCommand("AUTHENTICATE SCRAM-SHA-1")
		case AUTH_SCRAM_SHA256:
			err = c.sendTaggedCommand("AUTHENTICATE SCRAM-SHA-256")
		default:
			return false, nil
		}
		if err != nil {
			return false, err
		}
		rsp, err := c.readResponse()
		if err != nil {
			return false, err
		}
		// use c.isBadRspGeneral() to follow the same check manner in hydra
		if c.isBadRspGeneral(rsp.Raw) {
			return false, nil
		}
		switch c.authStrategy {
		case AUTH_SCRAM_SHA1:
			client, err = NewScramClient("SHA-1", username, pwd)
		case AUTH_SCRAM_SHA256:
			client, err = NewScramClient("SHA-256", username, pwd)
		default:
			return false, nil
		}
		if err != nil {
			return false, err
		}
		_, clientFirstMsg, _ := client.Start()
		b64ClientFirstMsg := codec.EncodeBase64(clientFirstMsg)
		err = c.sendCommand(b64ClientFirstMsg)
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		if c.isBadRspGeneral(rsp.Raw) {
			return false, nil
		}
		if len(rsp.Raw) <= 2 {
			return false, errors.New("invalid SCRAM server rsp length")
		}
		decodedRsp, err := codec.DecodeBase64(rsp.Raw[2:])
		if err != nil {
			return false, errors.New("invalid SCRAM server rsp base64 decode failed")
		}
		nextClientMsg, err := client.Next(decodedRsp)
		if err != nil {
			return false, err
		}
		b64NextClientMsg := codec.EncodeBase64(nextClientMsg)
		err = c.sendCommand(b64NextClientMsg)
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		return !c.isBadRspGeneral(rsp.Raw), nil
	case AUTH_NTLM:
		err := c.sendTaggedCommand("AUTHENTICATE NTLM")
		if err != nil {
			return false, err
		}
		rsp, err := c.readResponse()
		if err != nil {
			return false, err
		}
		// use c.isBadRspGeneral() to follow the same check manner in hydra
		if c.isBadRspGeneral(rsp.Raw) {
			return false, nil
		}
		client := nla.NewNTLMv2("", username, pwd)
		negotiateMsg := client.GetNegotiateMessage().Serialize()
		err = c.sendCommand(codec.EncodeBase64(negotiateMsg))
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		if len(rsp.Raw) < 6 {
			return false, errors.New("invalid NTLM server rsp length")
		}
		serverChallenge, err := codec.DecodeBase64(rsp.Raw[2:])
		if err != nil {
			return false, errors.New("invalid NTLM server rsp base64 decode failed")
		}
		authMsg, _ := client.GetAuthenticateMessage(serverChallenge)
		err = c.sendCommand(codec.EncodeBase64(authMsg))
		if err != nil {
			return false, err
		}
		rsp, err = c.readResponse()
		if err != nil {
			return false, err
		}
		return !c.isBadRspGeneral(rsp.Raw), nil
	default: // AUTH_CLEAR
		if len(username) > 100 {
			username = username[:100]
		}
		if len(pwd) > 100 {
			pwd = pwd[:100]
		}
		authMsg := fmt.Sprintf("LOGIN \"%s\" \"%s\"", username, pwd)
		err := c.sendTaggedCommand(authMsg)
		rsp, err := c.readResponse()
		if err != nil {
			return false, err
		}
		return !c.isBadRspGeneral(rsp.Raw), nil
	}
}

func (c *IMAPClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// IMAPAuth use netx.Dial instead of net.Dial, and check auth method
// Manually test with https://app.mailslurp.com/dashboard/
func IMAPAuth(target, username, password string) (bool, error) {
	conn, err := defaultDialer.Dial("TCP", target)
	if err != nil {
		return false, dialError
	}
	client := NewIMAPClient(conn, utils.ExtractHost(target))
	defer client.Close()
	if !client.IsIMAP() || client.GetCap() != nil {
		return false, errors.New("not an imap or service shutdown")
	}
	return client.StartIMAP(username, password)
}

var imapAuth = &DefaultServiceAuthInfo{
	ServiceName:      "imap",
	DefaultPorts:     "143",
	DefaultUsernames: CommonUsernames,
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		target := fixToTarget(i.Target, 143)
		res := i.Result()
		ok, err := IMAPAuth(target, "", "")
		if err != nil && errors.Is(err, dialError) {
			res.Finished = true
		}
		res.Ok = ok
		return res
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		target := fixToTarget(i.Target, 143)
		res := i.Result()
		ok, err := IMAPAuth(target, i.Username, i.Password)
		if err != nil && errors.Is(err, dialError) {
			res.Finished = true
			return res
		}
		res.Ok = ok
		return res
	},
}
