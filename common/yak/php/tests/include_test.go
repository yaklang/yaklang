package tests

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestInclude(t *testing.T) {
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
	include("1.php");
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
	ssatest.CheckSyntaxFlowWithFS(t, fs, `<include('php-param')> as $params`, map[string][]string{
		"params": {"Undefined-$password(valid)", "Undefined-$user2(valid)", "Undefined-$username(valid)"},
	}, false, ssaapi.WithLanguage(ssaapi.PHP))
}
