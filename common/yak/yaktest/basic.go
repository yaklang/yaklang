package yaktest

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yak/yaklang"

	_ "github.com/yaklang/yaklang/common/yak"
)

type YakTestCase struct {
	Name string
	Src  string
}

func exec(raw string) error {
	return yaklang.New().SafeEval(context.Background(), raw)
}

func analyze(raw string) []*result.StaticAnalyzeResult {
	return yak.StaticAnalyze(raw)
}

func Run(verbose string, t *testing.T, cases ...YakTestCase) {
	testcase := assert.New(t)
	fmt.Println("Start to run TestCase Group:", verbose)
	for _, _case := range cases {
		err := exec(_case.Src)
		if err != nil {
			testcase.FailNow(fmt.Sprintf(`"TestCase[%v] Failed: %v"`, _case.Name, err))
		}
	}
}

func StaticAnalyze(verbose string, t *testing.T, cases ...YakTestCase) {
	testcase := assert.New(t)
	println("Start to run TestCase StaticAnalyze Group:", verbose)
	for _, _case := range cases {
		suggestion := analyze(_case.Src)
		if len(suggestion) <= 0 {
			testcase.FailNow(fmt.Sprintf("{%v} 语言静态建议失败", _case.Name))
		}
	}
}
