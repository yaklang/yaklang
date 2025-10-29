package php

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestNamespace(t *testing.T) {
	t.Run("namespace", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("a.php", `<?php

namespace a\d;
use \cb;

class A{
    public function getList($d){
        c::Get($d);
    }
}`)
		fs.AddFile("b.php", `<?php
namespace a\d;
use b\c;
class B{
	public function BB($a){
		c::Get("1");
	} 
}
`)
		fs.AddFile("c.php", `<?php
namespace b;
class c{
	public static function Get($a){
		scandir($a);
	}
}
`)
		ssatest.CheckWithFS(fs, t, func(programs ssaapi.Programs) error {
			res, err := programs.SyntaxFlowWithError(`scandir(* #-> * as $param)`)
			require.NoError(t, err)
			values := res.GetValues("param")
			require.True(t, len(values) == 2)
			values.Show()
			return nil
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}

//todo:
//func TestNamespace33(t *testing.T) {
//	code := `<?php
//
//class ReturnMessage{
//    public function return($msg){
//        exit($msg);
//    }
//}
//class User{
//    public  $name;
//    public  $age;
//    public  $address;
//    public $show;
//    public function __construct($a,$b){
//        $this->show = new ReturnMessage;
//    }
//    public function Show(){
//        $msg =  "hello: ".$this->name;
//        $this->show->return($msg);
//    }
//}
//$user = new User();
//$user->name = $_GET[name];
//$user->Show();`
//	ssatest.CheckSyntaxFlow(t, code, `
//exit(* #-> * as $param)
//println(* #-> * as $sink)
//`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.PHP))
//}
