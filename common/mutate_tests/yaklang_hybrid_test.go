package mutate_tests

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"testing"
)

type HybridTestCase struct {
	Code  string
	Debug bool
}

func TestYaklangHybridFuzz(t *testing.T) {
	initDB()

	cases := []*HybridTestCase{
		{Code: `
packet = ` + "`" + `POST /admin/test.php?abc=123&&c={"abc":12,"b":["12",{"efg":4,"ag":"123"}],"c":1} HTTP/1.1
Host: www.baidu.com
Cookie: jsonBase64Param=eyJhYmMiOjEyLCJiIjpbIjEyIix7ImVmZyI6NCwiYWciOiIxMjMifV0sImMiOjF9; jsonUrlParam=%7B%22abc%22%3A12%2C%22b%22%3A%5B%2212%22%2C%7B%22efg%22%3A4%2C%22ag%22%3A%22123%22%7D%5D%2C%22c%22%3A1%7D

{"abc":12,"b":["12",{"efg":4,"ag":"123"}],"c":1}` + "`\n" + `
fuzz.HTTPRequest(packet)~.GetAllParams()
`, Debug: true},
	}

	var debugCases []*HybridTestCase
	var normalCases []*HybridTestCase
	for _, c := range cases {
		if c.Debug {
			debugCases = append(debugCases, c)
		} else {
			normalCases = append(normalCases, c)
		}
	}

	handleCase := func(data *HybridTestCase) {
		err := yaklang.New().Eval(context.Background(), data.Code)
		if err != nil {
			fmt.Println()
			fmt.Println(data.Code)
			panic(err)
		}
	}

	for _, c := range debugCases {
		handleCase(c)
	}
	for _, c := range normalCases {
		handleCase(c)
	}
}
