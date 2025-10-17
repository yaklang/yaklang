package ssaapi_test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_CrossProcess(t *testing.T) {
	t.Run("avoid recursive mechanism should not be triggered ", func(t *testing.T) {
		code := `
	func A(num){
		return num
	}	

	func foo(){
		m := {"a":A(1),"b":A(2)}
		print(m)
	}
		`
		/*
			以上代码会进行两次跨过程分析，不会触发防递归机制
			m->
			  -> FreeValue-A(1)
				-> Function-A
				  -> Parameter-num
					-> 1
			  -> FreeValue-vA(2)
				-> Function-A
			      -> Parameter-num
					-> 2
		*/
		ssatest.CheckSyntaxFlowContain(t, code, `print(* #{
include:<<<INCLUDE
	* ?{opcode:const}
INCLUDE
}-> as $res)`, map[string][]string{
			"res": {"1", "2"},
		})
	})

	t.Run("intra and cross process avoid recursive mechanism should not be triggered ", func(t *testing.T) {
		code := `
	f1 = (a1)=>{
		return a1
	}
	
	f0 = (a0) =>{
		value = f1(a0)
		if c{
			value = f1(a0)+1
		}	
		return value
	}
	print(f0(11))
		`
		ssatest.CheckSyntaxFlow(t, code, `print(* #-> as $res)`, map[string][]string{
			"res": {"1", "11", "FreeValue-c"},
		})
	})

	t.Run("avoid recursive mechanism should be triggered ", func(t *testing.T) {
		code := `
f4 = (a4) => {
    return a4;
}

f3 = (a3) => {
    return f4(a3) ;
}

f2 = (a2) => {
    return f3(a2) ;
}

f1 = (a1) => {
    resultB = f2(a1);
    resultC = f3(a1);
    return resultB + resultC;
}

result = f1(10);
`

		ssatest.CheckSyntaxFlowContain(t, code, `a4?{opcode:param} --> as $ret`, map[string][]string{
			"ret": {"Function-f1(10)"},
		})
	})

	t.Run("rollback process", func(t *testing.T) {
		code := `
f1 = (a1) => {
	return a1
}
f2 = (a2) =>{
	return f1(a2)
}

a = f1(7)
b = f2(8)
`

		ssatest.CheckSyntaxFlow(t, code, `a #-> * as $target1;b #-> * as $target2`, map[string][]string{
			"target1": {"7"},
			"target2": {"8"},
		})

	})

	t.Run("multiple  rollback process ", func(t *testing.T) {
		code := `
f0 = (a0) =>{
	return a0
}

f1 = (a1) => {
	return f0(a1) 
}
f2 = (a2) =>{
	return f1(a2)
}

a = f1(7)
b = f2(8)
`

		ssatest.CheckSyntaxFlow(t, code, `a #-> * as $target1;b #-> * as $target2`, map[string][]string{
			"target1": {"7"},
			"target2": {"8"},
		})
	})

	t.Run("rollback and cross process", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f1 = (a1)  => {
  			return a1
		}
		f2 = (a2) => {
			return f1(a2)
		}
		f1(f2(11))
		`, `
		a2?{opcode: param} #-> * as $target`, map[string][]string{
			"target": {"11"},
		})
	})

}

func Test_IntraProcess(t *testing.T) {
	t.Run("Test_IntraProcess_Analysis", func(t *testing.T) {
		code := `
	m = {
		"foo":r.FormValue("name"),
		"bar":template.HTML(r.FormValue("id")), 
	}
	print(m)
	`
		/*
				strict digraph {
			    rankdir = "BT";
			    n1 [label="r"]
			    n10 [label="t19: #7.FormValue(t18)"]
			    n11 [label="t18: \"id\""]
			    n15 [label="t20: m.bar=#14.HTML(t19)"]
			    n18 [label="template"]

			    n2 [label="#7.FormValue"]
			    n3 [label="t11: m.foo=#7.FormValue(t10)"]
			    n4 [label="t10: \"name\""]
			    n16 [label="#14.HTML"]

			    n3 -> n4 [label=""]
			    n10 -> n11 [label=""]
			    n10 -> n2 [label=""]
			    n3 -> n4 [label=""]
			    n3 -> n2 [label=""]
			    n15 -> n16 [label=""]
			    n15 -> n10 [label=""]
			    n1 -> n2 [label=""]
			    n3 -> n2 [label=""]
			    n1 -> n2 [label=""]
			    n10 -> n11 [label=""]
			    n10 -> n2 [label=""]
			    n15 -> n16 [label=""]
			    n18 -> n16 [label=""]
			    n18 -> n16 [label=""]
			    n15 -> n10 [label=""]
			}
		*/
		rule := ` 
m.foo as $foo;
m.bar as $bar;
print(* as $m)

$m#{
exclude:<<<EXCLUDE
	* & $foo	
EXCLUDE
}-> as $res1;

$m#{
exclude:<<<EXCLUDE
	* & $bar
EXCLUDE
}-> as $res2;
`
		ssatest.CheckSyntaxFlowContain(t, code, rule, map[string][]string{
			"res1": {"id"},
			"res2": {"name"},
		})
	})

	t.Run("test object", func(t *testing.T) {
		code := `
	m = {
		"foo":foo(a.b),
		"bar":bar(a.b),
	}
	print(m)
`
		/*
			strict digraph {
				rankdir = "BT";
				n1 [label="foo"]
				n2 [label="t1224023: m.foo=foo(#1224020.b)"]
				n4 [label="#1224020.b"]
				n6 [label="a"]
				n9 [label="t1224028: m.bar=bar(#1224020.b)"]
				n10 [label="bar"]
				n2 -> n1 [label=""]
				n2 -> n4 [label=""]
				n9 -> n10 [label=""]
				n9 -> n4 [label=""]
				n2 -> n4 [label=""]
				n6 -> n4 [label=""]
				n6 -> n4 [label=""]
				n9 -> n10 [label=""]
				n9 -> n4 [label=""]
				n2 -> n1 [label=""]
			}
		*/
		rule := ` 
m.foo as $foo;
m.bar as $bar;
print(* as $m)

$m#{
exclude:<<<EXCLUDE
	* & $foo	
EXCLUDE
}-> as $res1;

$m#{
exclude:<<<EXCLUDE
	* & $bar
EXCLUDE
}-> as $res2;
`
		ssatest.CheckSyntaxFlowContain(t, code, rule, map[string][]string{
			"res1": {"bar"},
			"res2": {"foo"},
		})
	})
}
