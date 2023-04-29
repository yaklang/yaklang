package yakgrpc

import (
	"crypto/tls"
	"yaklang/common/utils"
	"testing"
	"time"
)

func TestNetConn(t *testing.T) {
	conn, err := utils.GetProxyConn("chat.openai.com", "http://127.0.0.1:7890", 10*time.Second)
	if err != nil {
		panic(err)
	}
	println("Connect Finished")
	defer conn.Close()

	tlsConn := tls.Client(conn, utils.NewDefaultTLSConfig())
	if err != nil {
		panic(err)
	}
	defer tlsConn.Close()

	err = tlsConn.Handshake()
	if err != nil {
		panic(err)
	}
	_ = tlsConn
}
