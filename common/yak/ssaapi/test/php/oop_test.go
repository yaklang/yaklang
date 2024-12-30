package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestStatic(t *testing.T) {
	code := `
<?php

class A{
    public static $a =1;
}
println(A::$a);
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"1"},
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
func TestConstructorDataFlow(t *testing.T) {
	t.Run("constructor", func(t *testing.T) {
		code := `<?php
$a = new AA(1);
println($a->a);
`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"Undefined-AA", "Undefined-AA", "1", "Undefined-AA.AA-destructor", "make(any)"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("have constructor", func(t *testing.T) {
		code := `<?php
class A{
	public function __construct(){}
}
$a = new A();
$a->bb();
`
		ssatest.CheckSyntaxFlow(t, code, `
A() as $output
$output -> as $sink
`, map[string][]string{
			"output": {"Function-A(Undefined-A)"},
			"sink":   {"Undefined-$a.bb(Function-A(Undefined-A))", "Undefined-A.A-destructor(Function-A(Undefined-A))"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("no constructor", func(t *testing.T) {
		code := `<?php
$a = new A();
$a->bb();
`
		ssatest.CheckSyntaxFlow(t, code, `
A() as $output
$output -> as $sink
`, map[string][]string{
			"output": {"Undefined-A(Undefined-A)"},
			"sink":   {"Undefined-$a.bb(Undefined-A(Undefined-A))", "Undefined-A.A-destructor(Undefined-A(Undefined-A))"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}

func TestFulLTypename(t *testing.T) {
	t.Run("test no package,blueprint packageName", func(t *testing.T) {
		code := `<?php
class A{}
$a = new A();
`
		ssatest.CheckSyntaxFlow(t, code, `A() as $start;  $start<fullTypeName><show> as $end`, map[string][]string{
			"end": {`"main.A"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test package blueprint packageName", func(t *testing.T) {
		code := `<?php
namespace B\A\C{
class A{}
}
namespace{
	use B\A\C\A;
	$a = new A();
}
`
		ssatest.CheckSyntaxFlow(t, code, `
A() as $start;
$start<fullTypeName><show> as $end;
`, map[string][]string{
			"end": {`"B.A.C.A"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test package blueprint member", func(t *testing.T) {
		code := `<?php

class B{

}
class A{
    public B $a;
}
$a = new A();
println($a->a);
`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $start);$start<fullTypeName><show>  as $end`, map[string][]string{
			"end": {`"main.B"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test package bluePrint member not import", func(t *testing.T) {
		code := `<?php

namespace A\B\C{
    use B\C\D\B;
    class A{
        public B $a;
    }
}
namespace {
	use \A\B\C\A;
    $a = new A();
    println($a->a);
}
`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $param);$param<fullTypeName><show> as $end`, map[string][]string{
			"end": {`"B.C.D.B"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test package blueprint", func(t *testing.T) {
		code := `<?php
namespace {
	use B\C\D;
	class A{
		public D $a;
	}
}
$a = new A();
println($a->a);
`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $param);$param<fullTypeName><show> as $end`, map[string][]string{
			"end": {`"B.C.D"`, `"main.A"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})

	//todo: fix fullTypename member
	t.Run("test no import", func(t *testing.T) {
		code := `<?php
namespace A\B\C{
    class A{
        public B $a;
    }    
}
namespace {
    $a = new A();
    println($a->a);
}`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $param);$param<fullTypeName><show> as $end`, map[string][]string{
			"end": {`"main.A"`},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("parent class", func(t *testing.T) {
		code := `<?php

namespace B\C\D{
    class A{}
}
namespace A\B\C{
    use B\C\D\A;
    class BB extends A{}
}
namespace{
    use A\B\C\BB;
    $a = new BB;
    println($a);
}
`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $param);$param<fullTypeName><show> as $end;`,
			map[string][]string{
				"end": {`"A.B.C.BB"`, `"B.C.D.A"`},
			},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("anymous-class with parent2", func(t *testing.T) {
		code := `<?php


class A extends B{
}

$c= 1;
$a = new class($c) extends A{
	public function __construct($c){
        echo $c;
	}
};
println($a->AA());

class B{
    public function AA(){
        return 1;
    }
}`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": []string{"1"},
		}, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
