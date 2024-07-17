package tools

import (
	_ "net/http/pprof"
	"testing"
)

func Test__scanx(t *testing.T) {
	res, err := _scanx(
		"192.168.3.1/24",
		//"47.52.100.35/24",
		"80",
	)
	if err != nil {
		t.Fatal(err)
	}
	for re := range res {
		t.Log(re.String())
	}
}
