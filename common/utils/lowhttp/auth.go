package lowhttp

import (
	"bufio"
	"encoding/base64"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
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
	AuthInfo string
}

func (da *DigestAuthentication) Authenticate(conn net.Conn, config *LowhttpExecConfig) ([]byte, error) {
	method, uri, _ := GetHTTPPacketFirstLine(config.Packet)
	body := GetHTTPPacketBody(config.Packet)
	url := GetHTTPPacketHeader(config.Packet, "Host") + uri
	if config.Https {
		url += "https://"
	} else {
		url += "http://"
	}
	_, ah, err := GetDigestAuthorizationFromRequestEx(method, url, string(body), da.AuthInfo, da.Username, da.Password, true)
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

type CustomAuthClient struct {
	handler func([]byte) ([]byte, error) //自定义认证处理函数
}

func (ca *CustomAuthClient) Authenticate(conn net.Conn, config *LowhttpExecConfig) ([]byte, error) {
	return ca.handler(config.Packet)
}

func GetAuth(authHeader string, username string, password string) Authentication {
	authResp := strings.SplitN(authHeader, " ", 2)
	switch strings.ToLower(authResp[0]) {
	case "negotiate", "ntlm":
		domain := ""
		if strings.Contains(username, "\\") {
			domainAndUsername := strings.SplitN(username, "\\", 2)
			domain, username = domainAndUsername[0], domainAndUsername[1]
		}
		if len(authResp) > 1 { // 连接复用的情况 可能会跳过协商阶段 直接质询
			handler := func(packet []byte) ([]byte, error) {
				ntv2 := nla.NewNTLMv2(domain, username, password)
				challenge, err := codec.DecodeBase64(authResp[1])
				if err != nil {
					log.Errorf("decode challenge failed")
					return []byte{}, err
				}
				authMessage, _ := ntv2.GetAuthenticateMessage(challenge)
				authReq := ReplaceHTTPPacketHeader(packet, "Authorization", authResp[0]+" "+codec.EncodeBase64(authMessage.Serialize()))
				return authReq, nil
			}
			return &CustomAuthClient{handler: handler}
		}
		return &NtlmAuthentication{username, password, domain}
	case "basic":
		return &BasicAuthentication{username, password}
	case "digest":
		return &DigestAuthentication{username, password, authHeader}
	}
	return nil
}

func GetHttpAuth(authHeader string, opt *LowhttpExecConfig) Authentication {
	authResp := strings.SplitN(authHeader, " ", 2)
	authType := authResp[0]
	host := GetHTTPPacketHeader(opt.Packet, "Host")
	if opt.Username != "" || opt.Password != "" {
		return GetAuth(authHeader, opt.Username, opt.Password)
	}
	authInfo := consts.GetGlobalHTTPAuthInfo(host, authType)
	if authInfo == nil {
		return nil
	}
	return GetAuth(authHeader, authInfo.AuthUsername, authInfo.AuthPassword)
}
