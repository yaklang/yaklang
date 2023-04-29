package yaklib

import (
	"github.com/davecgh/go-spew/spew"
	"yaklang/common/log"
	"testing"
	"time"
)

func TestYakitServer_Addr(t *testing.T) {
	s := NewYakitServer(2335, SetYakitServer_ProgressHandler(func(id string, progress float64) {
		log.Infof("progress: %v  - percent float: %v", id, progress)
	}))
	go func() {
		s.Start()
	}()
	time.Sleep(time.Second * 1)

	spew.Dump(s.Addr())
	c := NewYakitClient(s.Addr())
	err := c.SetProgress("test", 0.5)
	err = c.SetProgress("test", 0.6)
	err = c.SetProgress("test", 0.7)
	err = c.SetProgress("test", 0.8)
	err = c.SetProgress("test", 0.99)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	time.Sleep(time.Second)
}
