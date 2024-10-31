package php

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
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
func TestConstructor(t *testing.T) {
	code := `<?php
$a = new AA(1);
println($a->a);
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"Undefined-AA-constructor", "Undefined-AA", "1"},
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
func TestCode(t *testing.T) {
	code :=
	code := `<?php

namespace lib\common\fileOperator;
class FileHandler{
    public $file;
    public $path = "C:\\temp\\store\\";
    public function __construct(){

    }
    public function __read($file){
        $this->file = $this->path.$file;
        $this->file = $this->filter($this->file);
        $fd = fopen($this->file);
        $content = fread($fd);
        fclose($fd);
        return $content;
    }
    public function __write($file,$content){
        $file = $this->filter($file);
        file_put_contents($file,$content);
    }
    public function filter($filename){
        return str_ireplace("../","",$filename);
    }
}

namespace base\user\Constroller;

use lib\common\fileOperator\FileHandler;

class UserController extends Controller{
    private $user= "";
    private $pass = "";
    public function Login(){
        if(isset($_POST["user"])){
            $this->user = $_POST["user"];
        }
        if(isset($_POST["pass"])){
            $this->pass = $_POST["pass"];
        }
        $result = $mysql->query($this->user,$this->pass);
        if($result){
            $this->Cache();
        }
    }
    public function Cache(){
        $file = $this->user.$this->pass.".php";
        $fd = new FileHandler();
        $content = $fd->__read($file);
        if($content==""){
            $loginfo[0]=$this->user;
            $loginfo[1]=$this->pass;
            $fd->__write($file,json_encode($this->user));
        }
    }
}`
	ssatest.CheckSyntaxFlow(t, code, ``, map[string][]string{}, ssaapi.WithLanguage(ssaapi.PHP))
}
