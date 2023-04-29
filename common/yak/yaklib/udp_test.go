package yaklib

import (
	"github.com/davecgh/go-spew/spew"
	"yaklang/common/log"
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
