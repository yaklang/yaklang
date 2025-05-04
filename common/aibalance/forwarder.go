package aibalance

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	CONST_AIBALANCE_CA_KEY  = "72cb89f4d7f94ff9027eb19f1529a5b0"
	CONST_AIBALANCE_CA_CERT = "6336b42bd3ee9914044e636099ed80d3"
)

type RegisterForwarderRequest struct {
	Domain    string `json:"domain"`
	EnableTLS string `json:"enable_tls"`
	Target    string `json:"target"`
}

func (c *ServerConfig) serveForwarder(conn net.Conn, requestRaw []byte) {
	// generate
	ca := yakit.GetKey(GetDB(), CONST_AIBALANCE_CA_KEY)
	caKey := yakit.GetKey(GetDB(), CONST_AIBALANCE_CA_CERT)
	if ca == "" || caKey == "" {
		rd := utils.RandStringBytes(10) + ".com"
		rd = strings.ToLower(rd)
		caRaw, keyRaw, err := tlsutils.GenerateSelfSignedCertKeyWithCommonNameEx(
			rd, rd, rd, nil, nil, nil, true,
		)
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(err.Error()), err.Error())))
			log.Errorf("Failed to generate self signed certificate for AIBALANCE server: %s", err)
			return
		}
		yakit.SetKey(GetDB(), CONST_AIBALANCE_CA_KEY, caRaw)
		yakit.SetKey(GetDB(), CONST_AIBALANCE_CA_CERT, keyRaw)
		ca = string(caRaw)
		caKey = string(keyRaw)
	}

	path := lowhttp.GetHTTPRequestPath(requestRaw)
	switch {
	case strings.HasPrefix(path, "/forwarder/register"):
		body := lowhttp.GetHTTPPacketBody(requestRaw)
		if body == nil {
			conn.Write([]byte(fmt.Sprintf("HTTP/1.1 400 Bad Request\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len("body is nil"), "body is nil")))
			return
		}

		scrt, skey, err := tlsutils.SignServerCrtNKeyWithoutAuth([]byte(ca), []byte(caKey))
		if err != nil {
			log.Errorf("Failed to sign server certificate: %s", err)
			conn.Write([]byte(fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(err.Error()), err.Error())))
			return
		}
		result := map[string]any{
			"ca":  string(ca),
			"crt": string(scrt),
			"key": string(skey),
		}
		raw, err := json.Marshal(result)
		if err != nil {
			log.Errorf("Failed to marshal result: %s", err)
			conn.Write([]byte(fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(err.Error()), err.Error())))
			return
		}
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(raw), string(raw))))
	}
}
