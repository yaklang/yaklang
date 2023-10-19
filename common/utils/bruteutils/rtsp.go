package bruteutils

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type RTSPAuthMethod uint8

const (
	RTSPAuthMethod_Origin RTSPAuthMethod = 0
	RTSPAuthMethod_Basic                 = 1
	RTSPAuthMethod_Digest                = 2
)

func genDESCRIBLE(url string, seq int) string {
	msgRet := "DESCRIBE " + url + " RTSP/1.0\r\n"
	msgRet += "CSeq: " + strconv.Itoa(seq) + "\r\n"
	msgRet += "Accept: application/sdp\r\n"
	msgRet += "\r\n"
	return msgRet
}

func genDESCRIBLEWithAuth(url string, seq int, username, password string, authMethod RTSPAuthMethod, authHeader string) string {

	switch authMethod {
	case RTSPAuthMethod_Digest, RTSPAuthMethod_Origin:
		msgRet := "DESCRIBE " + url + " RTSP/1.0\r\n"
		msgRet += "CSeq: " + strconv.Itoa(seq) + "\r\n"
		msgRet += "Authorization: " + authHeader + "\r\n"
		msgRet += "Accept: application/sdp\r\n"
		msgRet += "\r\n"
		return msgRet
	case RTSPAuthMethod_Basic:
		auth := fmt.Sprintf("%s:%s", username, password)
		auth = codec.EncodeBase64(auth)
		msgRet := "DESCRIBE " + url + " RTSP/1.0\r\n"
		msgRet += "CSeq: " + strconv.Itoa(seq) + "\r\n"
		msgRet += "Authorization: Basic " + auth + "\r\n"
		msgRet += "Accept: application/sdp\r\n"
		msgRet += "\r\n"
		return msgRet
	default:
		msgRet := "DESCRIBE " + url + " RTSP/1.0\r\n"
		msgRet += "CSeq: " + strconv.Itoa(seq) + "\r\n"
		msgRet += "Accept: application/sdp\r\n"
		msgRet += "\r\n"
		return msgRet
	}
}

var rtspAuth = &DefaultServiceAuthInfo{
	ServiceName:  "ssh",
	DefaultPorts: "554,1554",
	DefaultUsernames: []string{
		"admin", "root",
	},
	DefaultPasswords: []string{
		"12345", "admin", "1234", "123456", "123", "1111",
	},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		res := i.Result()
		u := i.Target

		if !strings.Contains(i.Target, "://") {
			target := fixToTarget(i.Target, 554)
			i.Target = target
			u = "rtsp://" + target
		} else {
			parsed, err := url.Parse(u)
			if err != nil {
				log.Errorf("rtsp: %v parse failed: %s", i.Target, err)
				return i.Result()
			}
			parsed.Host = fixToTarget(parsed.Host, 554)
			i.Target = parsed.Host
			u = parsed.String()
		}

		conn, err := netx.DialTCPTimeout(10*time.Second, i.Target)
		if err != nil {
			log.Errorf("rtsp: %v conn failed: %s", i.Target, err)
			res.Finished = true
			return res
		}
		defer conn.Close()

		_, err = conn.Write(utils.UnsafeStringToBytes(genDESCRIBLE(u, 2)))
		if err != nil {
			log.Errorf("rtsp: %v write failed: %s", i.Target, err)
			res.Finished = true
			return res
		}

		raw, err := utils.ReadConnWithTimeout(conn, 2*time.Second)
		if err != nil {
			log.Errorf("rtsp: %v read failed: %s", i.Target, err)
			res.Finished = true
			return res
		}

		sc := lowhttp.GetStatusCodeFromResponse(raw)

		if sc == 200 {
			res.Ok = true
			return res
		}

		return res
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		res := i.Result()
		u := i.Target
		target := fixToTarget(i.Target, 554)
		if !strings.Contains(i.Target, "://") {
			u = "rtsp://" + target
		}

		_ = u
		conn, err := netx.DialTCPTimeout(10*time.Second, target)
		if err != nil {
			log.Errorf("rtsp: %v conn failed: %s", target, err)
			res.Finished = true
			return res
		}
		defer conn.Close()

		_, err = conn.Write(utils.UnsafeStringToBytes(genDESCRIBLE(u, 2)))
		if err != nil {
			log.Errorf("rtsp: %v write failed: %s", i.Target, err)
			res.Finished = true
			return res
		}

		raw, err := utils.ReadConnWithTimeout(conn, 2*time.Second)
		if err != nil {
			log.Errorf("rtsp: %v read failed: %s", i.Target, err)
			res.Finished = true
			return res
		}

		sc := lowhttp.GetStatusCodeFromResponse(raw)

		authMethod := RTSPAuthMethod_Origin
		authHeader := lowhttp.GetHTTPPacketHeader(raw, "WWW-Authenticate")
		authResponseHeader := ""
		if sc == 401 {
			if strings.HasPrefix(authHeader, "Digest") {
				authMethod = RTSPAuthMethod_Digest
				_, ah, err := lowhttp.GetDigestAuthorizationFromRequestEx("DESCRIBE", u, "", authHeader, i.Username, i.Password, true)
				if err == nil {
					authResponseHeader = ah.String()
				}
			} else if strings.HasPrefix(authHeader, "Basic") {
				authMethod = RTSPAuthMethod_Basic
			}
		}

		_, err = conn.Write(utils.UnsafeStringToBytes(genDESCRIBLEWithAuth(u, 2, i.Username, i.Password, authMethod, authResponseHeader)))
		if err != nil {
			log.Errorf("rtsp: %v write failed: %s", i.Target, err)
			res.Finished = true
			return res
		}

		raw, _ = utils.ReadConnWithTimeout(conn, 2*time.Second)

		sc = lowhttp.GetStatusCodeFromResponse(raw)
		if sc == 200 {
			res.Ok = true
			res.Finished = true
		}

		return res
	},
}
