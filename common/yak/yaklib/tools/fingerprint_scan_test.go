package tools

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/fp"
	"testing"
)

func Test_scanFingerprint(t *testing.T) {
	target := "150.129.109.26"
	//target := "47.98.176.118"

	protoList := []interface{}{"tcp", "udp"}

	pp := func(proto ...interface{}) fp.ConfigOption {
		return fp.WithTransportProtos(fp.ParseStringToProto(proto...)...)
	}

	//ch, err := scanFingerprint(target, "80,22,443,8080,3306,161", pp(protoList...), fp.WithProbeTimeoutHumanRead(5))
	ch, err := scanFingerprint(target, "161", pp(protoList...), fp.WithProbeTimeoutHumanRead(5))

	if err != nil {
		t.Error(err)
	}

	for v := range ch {
		spew.Dump(v)
	}
}
