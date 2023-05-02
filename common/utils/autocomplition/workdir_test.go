package autocomplition

import (
	"github.com/k0kubun/pp"
	"testing"
	"github.com/yaklang/yaklang/common/log"
)

func TestGetWorkDirSuggestions(t *testing.T) {
	if results := GetWorkDirSuggestions("."); len(results) <= 0 {
		t.FailNow()
	} else {
		log.Info(pp.Sprintln(results))
	}
}
