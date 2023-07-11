package yaklib

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"testing"
	"time"
)

func TestUdpConn_Send(t *testing.T) {
	go func() {
		err := udpServe("127.0.0.1", 55433)
		if err != nil {
			log.Errorf("udp serve: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)
	conn, err := connectUdp("127.0.0.1:55433")
	if err != nil {
		spew.Dump(err)
		return
	}
	println(conn.RemoteAddr().String())
	err = conn.Send("123123123")
	if err != nil {
		log.Errorf("send error: %v", err)
	}
	time.Sleep(1 * time.Minute)
}

func TestDebugMockUDPProtocol(t *testing.T) {
	//matcher, err := fp.NewFingerprintMatcher(nil, nil)
	//if err != nil {
	//	t.Errorf("failed to create matcher: %s", err)
	//	t.FailNow()
	//}
	//_ = matcher
	//banner := []byte("0a\x02\x01\x00\x04\x06public\xa2a\x06\x08+\x06\x01\x02\x01\x01\x05\x00\x04a")
	//r, _ := regexp2.Compile("0.*\\x02\\x01\\x00\\x04\\x06public\\xa2.*\\x06\\x08\\+\\x06\\x01\\x02\\x01\\x01\\x05\\x00\\x04[^\\x00]([^\\x00]+)", 0)
	//a, err := r.FindRunesMatch(utils2.AsciiBytesToRegexpMatchedRunes(banner))
	//if err != nil {
	//	panic(err)
	//}
	//fmt.Println(a)
	fmt.Println("\x08")
	server, port := DebugMockUDPProtocol("snmp")

	fmt.Println(server, port)

	select {}
}
