package test

import (
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type TestCase struct {
	code   string
	ref    string
	target []string
	users  bool
	fuzz   bool
}

func check(t *testing.T, tc TestCase) {
	prog, err := ssaapi.Parse(tc.code, ssaapi.WithLanguage(ssaconfig.JS))
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
	value := prog.Ref(tc.ref)
	var ret []string
	if tc.users {
		value = value.GetUsers()
	}
	for _, v := range value {
		ret = append(ret, v.String())
	}

	// fmt.Println(ret)
	sort.Strings(ret)
	sort.Strings(tc.target)

	if len(tc.target) != len(ret) {
		t.Fatalf("check %s Count Number err, expect %d, got %d", tc.ref, len(tc.target), len(ret))
	}

	if tc.fuzz {
		for i, v := range tc.target {
			if !strings.Contains(ret[i], v) {
				t.Fatalf("check %s Test err, expect %s, got %s", tc.ref, v, ret[i])
			}
		}
	} else {
		for i, v := range tc.target {
			if v != ret[i] {
				t.Fatalf("check %s Test err, expect %s, got %s", tc.ref, v, ret[i])
			}
		}
	}
}

func TestFuncCall(t *testing.T) {
	t.Run("funcCall", func(t *testing.T) {
		refValue := []string{
			"Function-test(1,2)",
			"Function-test(2,3)"}
		check(t, TestCase{
			`function test(a, b){
				return a + b;
			}
			sum = test(1,2);
			sum = test(2,3);`,
			"sum",
			refValue,
			false,
			false,
		})
	})

	t.Run("funcCallBool", func(t *testing.T) {
		refValue := []string{"Function-tof(true,false)", "Parameter-b"}
		check(t, TestCase{
			`
			function tof(a, b){}
			b = tof(true, false);
			print(b)
			`,
			"b",
			refValue,
			false,
			false,
		})
	})
}

func TestAssign(t *testing.T) {
	t.Run("test let", func(t *testing.T) {
		retValue := []string{"Undefined-print(Undefined-fetch(Undefined-url))"}
		check(t, TestCase{
			`
			let response = fetch(url);
			print(response);
			`,
			`print`,
			retValue,
			true,
			false,
		})
	})
}

func TestCompute(t *testing.T) {
	// t.Run("test and", func(t *testing.T) {
	// 	retValue := []string{"lt(phi(a)[1,add(a, 1)], 11)", "add(phi(a)[1,add(a, 1)], 1)", "add(phi(s)[1,add(s, 1)], phi(a)[1,add(a, 1)])", "add(phi(a)[1,add(a, 1)], 1)", "phi(a)[1,add(a, 1)]", "phi(a)[1,add(a, 1)]"}
	// 	check(t, TestCase{
	// 		`
	// 		for(a=1,s=1;a<11&&s<20;a++,s++){
	// 			a+1,s+a;
	// 		}
	// 		`,
	// 		`a`,
	// 		retValue,
	// 		true,
	// 		false,
	// 	})
	// })

	t.Run("test bit UnOp", func(t *testing.T) {
		retValue := []string{"-2", "1"}
		check(t, TestCase{
			`
			Unvalue = ~0b1
			print(Unvalue)
			Unvalue = -(-(1))
			print(Unvalue)
			`,
			`Unvalue`,
			retValue,
			false,
			false,
		})
	})

	t.Run("test Scientific notation", func(t *testing.T) {
		retValue := []string{"lt(Undefined-a, 1e-06)"}
		check(t, TestCase{
			`
			a < 1e-6 ? 1 : 2
			`,
			`a`,
			retValue,
			true,
			false,
		})
	})
}

func TestExpr(t *testing.T) {
	t.Run("test expr", func(t *testing.T) {
		retValue := []string{"print(phi"}
		check(t, TestCase{
			`
			o = function() {o = 1}
			c = a && o()
			print(c)
			`,
			`c`,
			retValue,
			true,
			true,
		})
	})
}
