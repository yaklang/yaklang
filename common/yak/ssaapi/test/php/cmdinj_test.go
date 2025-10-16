package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPHP_CMDInj(t *testing.T) {
	code := `<?php
$command = 'ping -c 1 '.$_GET['ip'];
system($command); //system函数特性 执行结果会自动打印
?>`

	ssatest.CheckSyntaxFlow(t, code,
		`system( * as $command)`,
		map[string][]string{
			"command": {"add(\"ping -c 1 \", Undefined-_GET.ip(valid))"},
		},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}

func TestPHP_CMDInjNilPointer(t *testing.T) {
	code := `<?php
$a = $_GET[1];



$b= base64_decode($a);

$c= base64_decode($b);



system($c);`

	ssatest.CheckSyntaxFlow(t, code,
		`system(*  #-> * as $param)`,
		map[string][]string{
			"param": {"Function-base64_decode", "Undefined-_GET"},
		},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}

func TestPHP_OOP(t *testing.T) {
	t.Run("no impl __construct", func(t *testing.T) {
		code := `<?php
class b{
public $a;
public function __construct($a){
$this->a = $a;
}
}

class ob extends b{
}

$ob = new ob($_GET[1]);
eval($ob->a);
`
		ssatest.CheckSyntaxFlow(t, code,
			`eval(* #-> * as $param)`,
			map[string][]string{"param": {"Undefined-_GET"}},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}

func TestPHP_CMD(t *testing.T) {
	code := `<?php
$a = $_GET[1];
eval($a);
`
	ssatest.CheckSyntaxFlow(t, code,
		`eval(* #-> * as $param)`,
		map[string][]string{
			"param": {"Undefined-_GET"},
		},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}
func TestPHP_EVALGetTop(t *testing.T) {
	code := `<?php

function test($a){
  eval($a);
}

test($_GET[1]);`
	ssatest.CheckSyntaxFlow(t, code, `eval(* #-> * as $command)`, map[string][]string{
		"command": {"Undefined-_GET"},
	},
		ssaapi.WithLanguage(ssaapi.PHP))
}

func TestPhpEval(t *testing.T) {
	code := `<?php
function test($a){
  system($a);
}
test($_GET[1]);`
	ssatest.CheckSyntaxFlow(t, code,
		`system(* #->  as $param)`,
		map[string][]string{"param": {"Undefined-_GET"}},
		ssaapi.WithLanguage(ssaapi.PHP))
}
func TestDataflow(t *testing.T) {
	code := `<?php
$a = $_GET[1];

if($c){
    $a = filter($a);
}else{
    $a = unsafe($a);
}
eval($a);`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		ssatest.CheckSyntaxFlow(t, code, `
_POST.* as $start
_GET.* as $start
_REQUEST.* as $start
_COOKIE.* as $start


/^(htmlspecialchars|strip_tags|mysql_real_escape_string|addslashes|filter|is_numeric|str_replace|ereg|strpos|preg_replace|trim)$/ as $filter;


eval(* as $param);
$param#{
include: <<<CODE
<self> & $start
CODE,
exclude: <<<CODE
<self> & $filter
CODE
}-> as $output`, map[string][]string{"output": {"Undefined-$a(valid)", "Undefined-_GET"}}, ssaapi.WithLanguage(ssaapi.PHP))
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
