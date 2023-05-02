package bruteutils

import (
	"testing"
	"yaklang.io/yaklang/common/log"
)

func TestBruteItem_TOMCAT(t *testing.T) {
	err := runTest(tomcat, "https://etcapi.****i.net/manager/html")
	if err != nil {
		log.Error(err)
		t.FailNow()
		return
	}
}
