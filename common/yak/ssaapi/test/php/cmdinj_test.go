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

	// TODO: handler extern-function
	ssatest.CheckSyntaxFlow(t, code,
		`system( * as $command)`,
		map[string][]string{
			"command": {"add(\"ping -c 1 \", Undefined-$_GET.'ip')"},
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
		`system(*  #-> * as $command)`,
		map[string][]string{
			"command": {"Function-base64_decode", "Undefined-$_GET"},
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
			map[string][]string{"param": {"Undefined-$_GET", "Undefined-$_GET.1"}},
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}

func TestPHP_CMD(t *testing.T) {
	code := `<?php
$a = $_GET[1];
eval($a);
`
	ssatest.CheckSyntaxFlow(t, code,
		`eval(* #-> * as $command)`,
		map[string][]string{
			"command": {"Undefined-$_GET"},
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
		"command": {"Undefined-$_GET", "Undefined-$_GET.1"},
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
		`*_GET[*] --> * as $sink`,
		map[string][]string{"sink": {"Parameter-$a"}},
		ssaapi.WithLanguage(ssaapi.PHP))
}
