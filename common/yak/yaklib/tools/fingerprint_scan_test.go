package tools

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fp"
	"testing"
)

func Test_scanFingerprint(t *testing.T) {
	//target := "150.129.109.26"
	target := "47.98.176.118"
	//target := "118.171.54.61"
	//target := "192.168.3.113"
	//target := "117.212.17.42"
	target = "37.131.221.151"
	//target = "213.100.240.79"

	//port := "3307"
	//port := "21"
	//port := "80,22,443,8080,3306,161"
	//port := "80,161,U:162,554"
	//port := "554"
	port := "U:162"

	//protoList := []interface{}{"tcp", "udp"}
	//protoList := []interface{}{"udp"}
	protoList := []interface{}{"tcp"}

	pp := func(proto ...interface{}) fp.ConfigOption {
		return fp.WithTransportProtos(fp.ParseStringToProto(proto...)...)
	}

	ch, err := scanFingerprint(target, port, pp(protoList...),
		fp.WithProbeTimeoutHumanRead(5),
		fp.WithProbesMax(5),
	)
	//ch, err := scanFingerprint(target, "162", pp(protoList...), fp.WithProbeTimeoutHumanRead(5))

	if err != nil {
		t.Error(err)
	}

	for v := range ch {
		fmt.Println(v.String())
	}
}
