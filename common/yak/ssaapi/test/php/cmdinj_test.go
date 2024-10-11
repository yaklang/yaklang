package php

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
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
func TestPhp_SQLI_With_Filter(t *testing.T) {
	rule := `
<include('php-param')> as $start;
<include('php-filter-function')> as $filterFunc;
mysql_query(* as $param);

$param #{
	include: <<<INCLUDE
		<self> & $start
INCLUDE}-> as $tmp;

$param #{
	include: <<<INCLUDE
		<self> & $start
INCLUDE,
	exclude: <<<EXCLUDE
		<self> & $filterFunc
EXCLUDE,
}->  as $output`
	t.Run("test filter func is undefined", func(t *testing.T) {
		code := `<?php
    $llink=addslashes($_GET['1']);
    $query = "SELECT * FROM nav WHERE link='$llink'";
    $result = mysql_query($query) or die('SQL语句有误：'.mysql_error());
    $navs = mysql_fetch_array($result);`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			require.Contains(t, res.GetValues("filterFunc").String(), "Undefined-addslashes(Undefined-_GET.1(valid))")
			require.Contains(t, res.GetValues("tmp").String(), "Undefined-_GET")
			require.Equal(t, 0, res.GetValues("output").Len())
			return nil
		}, ssaapi.WithLanguage(consts.PHP))
	})
	t.Run("test existed  filter  func ", func(t *testing.T) {
		code := `<?php
	function addslashes($a){
		return $a;
	}
    $llink=addslashes($_GET['1']);
    $query = "SELECT * FROM nav WHERE link='$llink'";
    $result = mysql_query($query) or die('SQL语句有误：'.mysql_error());
    $navs = mysql_fetch_array($result);`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		res, err := prog.SyntaxFlowWithError(rule,sfvm.WithEnableDebug(true))
		require.NoError(t, err)
		require.Contains(t, res.GetValues("filterFunc").String(), "Function-addslashes")
		require.Contains(t, res.GetValues("tmp").String(), "Undefined-_GET")
		require.Equal(t, 0, res.GetValues("output").Len())
		return nil
	}, ssaapi.WithLanguage(consts.PHP))
	})
}

