package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestUse(t *testing.T) {
	t.Run("only use pkgName, function", func(t *testing.T) {
		code := `<?php

namespace a\b\c{
    const a = 1;
    function A(){
		return 1;
	}
}
namespace a\b\c\d{
    const a= 2;
}
namespace a{
    const a = 3;
}
namespace{
    use a\b\c;
    println(c\A());
}`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("only use pkgName,use blueprint static", func(t *testing.T) {
		code := `<?php

namespace a\b\c{
    const a = 1;
    function A(){
		return 1;
	}
    class ClassA{
        public static $a = 1;
    }
}
namespace a\b\c\d{
    const a= 2;
}
namespace a{
    const a = 3;
}
namespace{
    use a\b\c;
    println(c\ClassA::$a);
}`
		ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"1"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("only use pkgName,use blueprint keyword", func(t *testing.T) {
		code := `<?php

namespace a\b\c{
    const a = 1;
    function A(){
		return 1;
	}
    class ClassA{
        public $a = 1;
    }
}
namespace a\b\c\d{
    const a= 2;
}
namespace a{
    const a = 3;
}
namespace{
    use a\b\c;
    $a = new c\ClassA;
    println($a->a);
}`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{})
	})
	t.Run("use blueprint", func(t *testing.T) {
		code := `<?php

namespace a\b\c{
    const a = 1;
    function A(){
		return 1;
	}
    class ClassA{
        public $a = 1;
    }
}
namespace a\b\c\d{
    const a= 2;
}
namespace a{
    const a = 3;
}
namespace{
    use a\b\c\ClassA;
    $a = new ClassA;
    println($a->a);
}`
		ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"1"})
	})
}
