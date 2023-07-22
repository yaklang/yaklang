package fuzztagx

import (
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestExecuteExpTag(t *testing.T) {

	for _, testCase := range [][2]string{
		{
			"asd{{{ int(1)  }}}}}{}",
			"1",
		},
	} {
		res, err := ExecuteWithStringHandler(testCase[0], nil)
		if err != nil {
			panic(utils.Errorf("test data [%v] error: %v", testCase[0], err))
		}
		if len(res) == 0 {
			panic("generate error")
		}
		if res[0] != testCase[1] {
			panic(utils.Errorf("test data [%v] failed", testCase[0]))
		}
	}
}
