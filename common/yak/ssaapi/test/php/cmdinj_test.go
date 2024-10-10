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
			"param": {"Function-base64_decode", "Undefined-$a(valid)", "Undefined-_GET"},
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
			map[string][]string{"param": {"Undefined-_GET", "Undefined-_GET.1(valid)"}},
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
		"command": {"Undefined-_GET", "Undefined-_GET.1(valid)"},
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
		map[string][]string{"param": {"Undefined-_GET", "Undefined-_GET.1(valid)"}},
		ssaapi.WithLanguage(ssaapi.PHP))
}
func TestDataflow(t *testing.T) {
	/*
		<include('php-param')> as $start;
		<include('php-filter-function')> as $filter;
		mysql_query(* as $param);
		$param #{
		include: `<self> & $start`,
		exclude: `<self> & $filter`
		}->  as $output
	*/
	code := `<?php
    $llink=addslashes($_GET['1']);
    $query = "SELECT * FROM nav WHERE link='$llink'";
    $result = mysql_query($query) or die('SQL语句有误：'.mysql_error());
    $navs = mysql_fetch_array($result);`
	ssatest.CheckSyntaxFlow(t, code, "<include('php-param')> as $start;\n<include('php-filter-function')> as $filter;\nmysql_query(* as $param);\n$param #{\ninclude: `<self> & $start`,\nexclude: `<self> & $filter`\n}->  as $output", map[string][]string{}, ssaapi.WithLanguage(ssaapi.PHP))
}
