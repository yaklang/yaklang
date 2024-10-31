<?php
namespace lib\utils\session;

class Session{
    public $prefix = "sess_";
    public $path = "C:\\temp\\php_session\\";
    public function storeSession($a){
        $path = $this->path.md5($a).".php";
        $b = serialize($a);
        file_put_contents($path,$b);
    }
    public function recoverSession($session){
        $path = $this->path.md5($session).".php";
        $content = file_get_contents($path);
        $b = unserialize($content);
        return $b;
    }
}
namespace base\user\Controller;

use lib\utils\session\Session;
class User extends baseController{
    private $sessionManager;
    public function  DoLogin(){
        $this->sessionManager = new Session;
        $user = $_POST["user"];
        $pass = $_POST["pass"];
        $csrf_token = $_POST["token"];
        if(!checkToken($csrf_token)){
            exit("csrf token check fail");
        }
        $result = $mysql->query($user,$pass);
        if($result){
            $this->sessionManager->storeSession($pass);
            return "success";
        }
        $success = $this->sessionManager->recoverSession($pass);
        if($success!=""){
            return "success";
        }
    }
}