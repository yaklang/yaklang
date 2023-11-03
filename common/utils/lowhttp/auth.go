package lowhttp

import (
	"bufio"
	"encoding/base64"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strings"
)

type Authentication interface {
	Authenticate(conn net.Conn, config *LowhttpExecConfig) ([]byte, error)
}

type BasicAuthentication struct {
	Username string
	Password string
}

func (ba *BasicAuthentication) Authenticate(conn net.Conn, config *LowhttpExecConfig) ([]byte, error) {
	return ReplaceHTTPPacketHeader(config.Packet, "Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(ba.Username+":"+ba.Password))), nil
}

type DigestAuthentication struct {
	// DigestAuthentication的相关属性
	Username string
	Password string
}

func (da *DigestAuthentication) Authenticate(conn net.Conn, config *LowhttpExecConfig) ([]byte, error) {
	method, uri, _ := GetHTTPPacketFirstLine(config.Packet)
	body := GetHTTPPacketBody(config.Packet)
	authInfo := GetHTTPPacketHeader(config.Packet, "WWW-Authenticate")
	url := GetHTTPPacketHeader(config.Packet, "Host") + uri
	if config.Https {
		url += "https://"
	} else {
		url += "http://"
	}
	_, ah, err := GetDigestAuthorizationFromRequestEx(method, url, string(body), authInfo, da.Username, da.Password, true)
	if err != nil {
		return nil, utils.Wrap(err, "get digest authorization failed")
	}
	authResponseHeader := ah.String()
	authReq := ReplaceHTTPPacketHeader(config.Packet, "Authorization", authResponseHeader)
	return authReq, nil
}

type NtlmAuthentication struct {
	Username string
	Password string
	Domain   string
}

func (na *NtlmAuthentication) Authenticate(conn net.Conn, config *LowhttpExecConfig) ([]byte, error) {
	ntv2 := nla.NewNTLMv2(na.Domain, na.Username, na.Password)
	negotiation := ntv2.GetNegotiateMessage()
	negotiationReq := ReplaceHTTPPacketHeader(config.Packet, "Authorization", "NTLM "+codec.EncodeBase64(negotiation.Serialize()))
	_, err := conn.Write(negotiationReq)
	if err != nil {
		return nil, utils.Wrap(err, "write negotiation request failed")
	}
	httpResponseReader := bufio.NewReader(conn)
	_, err = httpResponseReader.Peek(1)
	if err != nil {
		return nil, utils.Wrap(err, "peek http response failed")
	}
	negotiationResponse, err := utils.ReadHTTPResponseFromBufioReader(httpResponseReader, nil)
	if err != nil {
		return nil, utils.Wrap(err, "read http response failed")
	}

	negotiationResponseByte, err := utils.DumpHTTPResponse(negotiationResponse, false)
	if err != nil {
		return nil, utils.Wrap(err, "dump http response failed")
	}
	challengeHeader := GetHTTPPacketHeader(negotiationResponseByte, "WWW-Authenticate")
	if !(len(challengeHeader) > 5 && strings.HasPrefix(challengeHeader, "NTLM ")) {
		return nil, utils.Wrap(err, "Authenticate header non-standard ")
	}
	challenge, err := codec.DecodeBase64(challengeHeader[5:])
	if err != nil {
		return nil, utils.Wrap(err, "decode challenge failed")
	}
	authMessage, _ := ntv2.GetAuthenticateMessage(challenge)
	authReq := ReplaceHTTPPacketHeader(config.Packet, "Authorization", "NTLM "+codec.EncodeBase64(authMessage.Serialize()))
	return authReq, nil
}

func GetAuth(auth string, username string, password string) Authentication {
	switch strings.ToLower(auth) {
	case "ntlm":
		domain := ""
		if strings.Contains(username, "\\") {
			domainAndUsername := strings.SplitN(username, "\\", 2)
			domain, username = domainAndUsername[0], domainAndUsername[1]
		}
		return &NtlmAuthentication{username, password, domain}
	case "basic":
		return &BasicAuthentication{username, password}
	case "digest":
		return &DigestAuthentication{username, password}
	}
	return nil
}
