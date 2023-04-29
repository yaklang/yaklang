package autocomplition

import (
	"github.com/k0kubun/pp"
	"yaklang/common/log"
	"testing"
)

func TestGetWorkDirSuggestions(t *testing.T) {
	if results := GetWorkDirSuggestions("."); len(results) <= 0 {
		t.FailNow()
	} else {
		log.Info(pp.Sprintln(results))
	}
}
