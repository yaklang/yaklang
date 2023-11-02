package lowhttp

import (
	"bufio"
	"encoding/base64"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strings"
)

type Authentication interface {
	Authenticate(conn net.Conn, req []byte) ([]byte, error)
}

type BasicAuthentication struct {
	Username string
	Password string
}

func (ba *BasicAuthentication) Authenticate(conn net.Conn, req []byte) ([]byte, error) {
	return ReplaceHTTPPacketHeader(req, "Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(ba.Username+":"+ba.Password))), nil
}

type DigestAuthentication struct {
	// DigestAuthentication的相关属性
}

func (da *DigestAuthentication) Authenticate(conn net.Conn, req []byte) ([]byte, error) {
	// 实现摘要认证逻辑
	return req, nil
}

type NtlmAuthentication struct {
	Username string
	Password string
	Domain   string
}

func (na *NtlmAuthentication) Authenticate(conn net.Conn, req []byte) ([]byte, error) {
	ntv2 := nla.NewNTLMv2(na.Domain, na.Username, na.Password)
	negotiation := ntv2.GetNegotiateMessage()
	negotiationReq := ReplaceHTTPPacketHeader(req, "Authorization", "NTLM "+codec.EncodeBase64(negotiation.Serialize()))
	_, err := conn.Write(negotiationReq)
	if err != nil {
		return nil, errors.Wrap(err, "write negotiation request failed")
	}
	httpResponseReader := bufio.NewReader(conn)
	_, err = httpResponseReader.Peek(1)
	if err != nil {
		return nil, errors.Wrap(err, "peek http response failed")
	}
	negotiationResponse, err := utils.ReadHTTPResponseFromBufioReader(httpResponseReader, nil)
	if err != nil {
		return nil, errors.Wrap(err, "read http response failed")
	}

	negotiationResponseByte, err := utils.DumpHTTPResponse(negotiationResponse, false)
	if err != nil {
		return nil, errors.Wrap(err, "dump http response failed")
	}
	challengeHeader := GetHTTPPacketHeader(negotiationResponseByte, "WWW-Authenticate")
	if !(len(challengeHeader) > 5 && strings.HasPrefix(challengeHeader, "NTLM ")) {
		return nil, errors.Wrap(err, "Authenticate header non-standard ")
	}
	challenge, err := codec.DecodeBase64(challengeHeader[5:])
	if err != nil {
		return nil, errors.Wrap(err, "decode challenge failed")
	}
	authMessage, _ := ntv2.GetAuthenticateMessage(challenge)
	authReq := ReplaceHTTPPacketHeader(req, "Authorization", "NTLM "+codec.EncodeBase64(authMessage.Serialize()))
	return authReq, nil
}

func GetNTLMAuth(username string, password string, domain string) Authentication {
	return &NtlmAuthentication{
		Username: username,
		Password: password,
		Domain:   domain,
	}
}
