package tools

import (
	"testing"
)

func Test__yakitBruterNew(t *testing.T) {
	addr := "192.168.3.52"
	addr = "192.168.3.113"
	got, err := _yakitBruterNew("mysql",
		yakBruteOpt_userlist("root", "aaa", "bbb"),
		yakBruteOpt_passlist("paaa", "pbbb", "root"),
		yakBruteOpt_OkToStop(true),
		yakBruteOpt_concurrent(1),
		//yakBruteOpt_minDelay(3),
	)
	res, err := got.Start(addr)
	if err != nil {
		t.FailNow()
	}
	for rt := range res {
		t.Log(rt.String())
	}
}
