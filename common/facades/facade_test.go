package facades

import (
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net"
	"testing"
	"time"
)

func TestNewDNSServer(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:4443")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				t.Error(err)
				t.FailNow()
				return
			}

			isTls := utils.NewBool(false)
		WRAPPER:
			peekableConn := utils.NewPeekableNetConn(conn)
			raw, err := peekableConn.Peek(1)
			if err != nil {
				utils.Errorf("")
				return
			}
			switch raw[0] {
			case 0x16: // https
				tlsConn := tlsutils.NewDefaultTLSServer(peekableConn)
				log.Error("https conn is recv... start to handshake")
				err := tlsConn.Handshake()
				if err != nil {
					conn.Close()
					log.Errorf("handle shake failed: %s", err)
					return
				}
				log.Infof("handshake finished for %v", conn.RemoteAddr())
				conn = tlsConn
				isTls.Set()
				goto WRAPPER
			case 'J': // 4a524d49 (JRMI)
				jrmiMagic, _ := peekableConn.Peek(4)
				if bytes.Equal(jrmiMagic, []byte("JRMI")) {
					log.Info("handle for JRMI")
					//err := rmiShakeHands(peekableConn)
					//if err != nil {
					//	log.Errorf("rmi handshak failed: %s", err)
					//}
					peekableConn.Close()
					return
				}
			}

			log.Infof("start to fallback http handlers for: %s", conn.RemoteAddr())
			//err = GetHTTPHandler(isTls.IsSet())(peekableConn)
			//if err != nil {
			//	log.Errorf("handle http failed: %s", err)
			//	return
			//}
		}
	}()

	time.Sleep(1 * time.Second)
	rsp, err := utils.NewDefaultHTTPClient().Get("https://127.0.0.1:4443")
	if err != nil {
		t.Error(err)
		return
	}

	utils.HttpShow(rsp)
}
