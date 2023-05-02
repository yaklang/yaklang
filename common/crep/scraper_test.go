package crep

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/log"
)

func TestSnapshot(t *testing.T) {
	raw, res, err := Snapshot("https://baidu.com", 2000*time.Millisecond)
	if err != nil {
		log.Error(err)
		t.Fail()
		return
	}

	spew.Dump(res)
	spew.Dump(raw)

	//err = ioutil.WriteFile("test.png", raw, 0777)
	//if err != nil {
	//	log.Error(err)
	//	t.Fail()
	//	return
	//}
}
