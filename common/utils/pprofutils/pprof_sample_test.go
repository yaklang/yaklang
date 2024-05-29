package pprofutils

import (
	_ "embed"
	"github.com/google/pprof/profile"
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

//go:embed cpu_sample.pprof.sample
var cpuSample []byte

func parseCPUSample(raw []byte) error {
	data, err := profile.ParseData(raw)
	if err != nil {
		return err
	}
	for _, i := range data.SampleType {
		log.Infof("sample type: %v %v", i.Type, i.Unit)
	}
	return nil
}

func TestParseCPUSample(t *testing.T) {
	err := parseCPUSample(cpuSample)
	if err != nil {
		t.Fatal(err)
	}
}
