package fingerprint

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/yak/cartesian"
	"testing"
)

var rsp = []byte(`HTTP/1.1 200 OK
Tag: --- VIDEO WEB SERVER ---
Tag1: ---aexeaaaa

<!doctype html>
<html>
/AV732E/setup.exe
</html>
`)

func TestExpressionOpCode(t *testing.T) {
	trueExp := `header = "VIDEO WEB SERVER"`
	falseExp := `header = "aa"`
	testTypes, err := cartesian.Product[bool]([][]bool{{true, false}, {true, false}, {true, false}, {true, false}})
	if err != nil {
		t.Fatal(err)
	}
	testCases := [][]any{}
	for _, testCase := range testTypes {
		var exp1, exp2 string
		if testCase[0] {
			exp1 = trueExp
		} else {
			exp1 = falseExp
		}
		if testCase[1] {
			exp2 = trueExp
		} else {
			exp2 = falseExp
		}
		testCases = append(testCases, []any{exp1 + "&&" + exp2, testCase[0] && testCase[1]})
		testCases = append(testCases, []any{exp1 + "||" + exp2, testCase[0] || testCase[1]})
	}
	for _, testCase := range testCases {
		exp := testCase[0].(string)
		expect := testCase[1].(bool)
		r, err := parsers.ParseExpRule([][2]string{{exp, "ok"}})
		if err != nil {
			t.Fatal(err)
		}
		info, err := rule.Execute(rsp, r[0].ToOpCodes())
		if err != nil {
			t.Fatal(err)
		}
		if !expect {
			assert.Nil(t, info)
		} else {
			assert.Equal(t, "ok", info.Info)
		}
	}
}

// TestYamlOpCode test regexp、condition、http_header、extract cpe by regexp
func TestYamlOpCode(t *testing.T) {
	for _, testCase := range []struct {
		rule   string
		expect bool
	}{
		{
			`- methods:
    - headers:
        - key: Tag1
          value:
            product_index: 1
            regexp: a(e.e)a`, true,
		}, {
			`- methods:
    - headers:
        - key: Tag
          value:
            product: exe
            regexp: WEB`, true,
		}, {
			`- methods:
    - headers:
        - key: Tag
          value:
            product: exe
            regexp: WEB1`, false,
		},
		{
			`- methods:
    - keywords:
        - product: exe
          regexp: .*\.exe`, true,
		},
		{
			`- methods:
    - condition: and
      keywords:
        - product: exe
          regexp: .*\.exe
        - product: exe
          regexp: .*\.aexe`, false,
		},
		{
			`- methods:
    - condition: or
      keywords:
        - product: exe
          regexp: .*\.exe
        - product: exe
          regexp: .*\.aexe`, true,
		},
	} {
		yamlRule := testCase.rule
		expect := testCase.expect
		r, err := parsers.ParseYamlRule(yamlRule)
		if err != nil {
			t.Fatal(err)
		}
		info, err := rule.Execute(rsp, r[0].ToOpCodes())
		if err != nil {
			t.Fatal(err)
		}
		if !expect {
			assert.Nil(t, info)
		} else {
			assert.Equal(t, "exe", info.CPE.Product)
		}
	}
}
