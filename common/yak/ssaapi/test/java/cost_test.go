package java

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
	"time"
)

//go:embed sample/fastjson/ParserConfig.java
var bigJavaFile string

func TestBigJavaFile(t *testing.T) {
	var astCost time.Duration
	var memCost time.Duration
	var dbCost time.Duration
	ssatest.ProfileJavaCheck(t, bigJavaFile, func(mem bool, prog *ssaapi.Program, start time.Time) error {
		if prog == nil {
			astCost = time.Since(start)
			return nil
		}
		if mem {
			memCost = time.Since(start)
		} else {
			dbCost = time.Since(start)
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
	ssa.ShowDatabaseCacheCost()
	log.Info("ast cost: ", astCost)
	log.Info("mem cost: ", memCost)
	log.Info(" db cost: ", dbCost)
}
