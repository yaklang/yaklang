package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestInclude(t *testing.T) {
	t.Run("include no php", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("var/www/html/1.txt", `<?php
$a = 1;
`)
		fs.AddFile("var/www/html/1.php", `<?php
include("1.txt");
echo $a;
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `echo(* #-> * as $param)`, map[string][]string{
			"param": {"1"},
		}, false, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test 2 include", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("/var/www/html/1.txt", `<?php
$a = 1;
`)
		fs.AddFile("/var/www/html/1.php", `<?php
include("1.txt");
echo $a;
`)
		fs.AddFile("/var/www/html/2.php", `<?php
include("1.txt");
$a = 2;
echo $a;
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `echo(* #-> * as $param)`, map[string][]string{
			"param": {"1", "2"},
		}, false, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test include lazyBuild", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("/var/www/html/a.txt", `<?php
function a($a){
	echo($a);
}
`)
		fs.AddFile("/var/www/html/b.php", `<?php
include("a.txt");
a(1);
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `echo(* #-> as $param)`, map[string][]string{
			"param": {"1"},
		}, true, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("test include", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("/var/www/html/1.txt", `<?php
class A{
	public function TT($a){
		echo $a;
	}
}
`)
		fs.AddFile("/var/www/html/1.php", `<?php
include("1.txt");
$a = new A();
$a->TT(1);
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `echo(* #-> * as $param)`, map[string][]string{}, false, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("custom include", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("var/www/html/1.php", `<?php
$a = 1;
$b = $a.$f;
`)
		fs.AddFile("var/www/html/2.php", `<?php
include("1.php");
println($a);
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"1"}},
			false,
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("include return", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("var/www/html/1.php", `<?php
$a = 1;
$b = $a.$f;
function test(){
	$a = 123;
	return $a;
}
return 1;
`)
		fs.AddFile("var/www/html/2.php", `<?php
include("1.php");
println($a);
$a = test();
println($a);
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"1", "123"}},
			false,
			ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("include file and include", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("var/www/html/1.php", `<?php
			$a = 1;
			return;
		`)
		fs.AddFile("var/www/html/2.php", `<?php
	$a = 2;
	function test(){
		$a = 123;
		return $a;
	}
`)
		fs.AddFile("var/www/html/3.php", `<?php
	include("2.php");
	println($a);
 	$a = test();
	println($a);
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs,
			`println(* #-> * as $param)`,
			map[string][]string{"param": {"2", "123"}},
			false,
			ssaapi.WithLanguage(ssaapi.PHP))
	})
}

func TestNativeCall_Include(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("list3.php", `<?php
	
	$ldapconn = ldap_connect("localhost");
	
	if($ldapconn){
	 $user2 = $_GET["user2"];
	
	 $filter = "(&(objectClass=user)(uid=" . $user2. "))";
	 $dn = "dc=example,dc=org";
	
	 ldap_list($ldapconn, $dn, $filter); // Noncompliant
	}`)
	fs.AddFile("list2.php", `<?php
	$username = $_POST['username'];
	$password = $_POST['password'];
	// without_pass
	$escaped_username = pass($username, '', LDAP_ESCAPE_FILTER);
	$dn = "cn={$escaped_username},ou=users,dc=example,dc=com";
	$is_valid = ldap_compare($ldap_conn, $dn, "userPassword", $password);`)
	ssatest.CheckSyntaxFlowWithFS(t, fs, `
_POST.* as $params
_GET.* as $params
_REQUEST.* as $params
_COOKIE.* as $params
	`, map[string][]string{
		"params": {
			"Undefined-$password(valid)", "Undefined-$user2(valid)",
			"Undefined-$username(valid)",
		},
	}, false, ssaapi.WithLanguage(ssaapi.PHP))
}

func TestNamespace2(t *testing.T) {
	t.Run("namespace", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("a.php", `<?php

namespace lemo\helper;
class FileHelper
{
    public static function getFileList($path,$type='')
    {
		scan($path);
}
}`)
		fs.AddFile("b.php", `<?php
namespace app\admin\controller\sys;
use lemo\helper\FileHelper;

class Uploads extends Backend{
    public function getList(){
        $path = input('path','uploads');
        $paths = app()->getRootPath().'public/storage/'.$path;
        $type = input('type','image');
        $list = FileHelper::getFileList($paths,$type);
        $data = ['state'=>'SUCCESS','start'=>0,'total'=>count($list),'list'=>[]];
        if($list){
            foreach ($list[0] as $k=>$v) {
                $data['list'][$k]['url'] = str_replace( app()->getRootPath().'public','',$v);
                $data['list'][$k]['mtime'] = mime_content_type($v);
            }
        }
        return json($data);
    }

}`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `scandir(* as $param)

scan(* as $param)
input() as $source
$param#{include: <<<CODE
* & $source
CODE}-> as $sink`, map[string][]string{"sink": {"input"}}, true, ssaapi.WithLanguage(ssaapi.PHP))
	})
	t.Run("same namespace name", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("a.php", `<?php

namespace b\c\a;

class A{
    public static function getA($a){
        scandir($a);
    }
}
`)
		fs.AddFile("b.php", `<?php

namespace b\c\a;

class B{
    public static function getB($a){
    }
}
`)
		fs.AddFile("c.php", `<?php

namespace d\b\c;
use b\c\a\A;

class AA{
    public function A(){
        $a = input("a");
        A::getA($a);
    }
}
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `scandir(* #->* as $param)`, map[string][]string{
			"param": {"input"},
		}, true, ssaapi.WithLanguage(ssaapi.PHP))
	})
}
