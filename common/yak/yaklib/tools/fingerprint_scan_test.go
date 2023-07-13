package tools

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fp"
	"testing"
)

func Test_scanFingerprint(t *testing.T) {

	target := "127.0.0.1"

	port := "55072"

	protoList := []interface{}{"tcp", "udp"}

	pp := func(proto ...interface{}) fp.ConfigOption {
		return fp.WithTransportProtos(fp.ParseStringToProto(proto...)...)
	}

	ch, err := scanFingerprint(target, port, pp(protoList...),
		fp.WithProbeTimeoutHumanRead(5),
		fp.WithProbesMax(100),
	)
	//ch, err := scanFingerprint(target, "162", pp(protoList...), fp.WithProbeTimeoutHumanRead(5))

	if err != nil {
		t.Error(err)
	}

	for v := range ch {
		fmt.Println(v.String())
	}
}
