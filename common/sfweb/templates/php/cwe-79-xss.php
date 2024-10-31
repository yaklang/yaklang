<?php

class ReturnMessage{
    public function return($msg){
        exit($msg);
    }
}
class User{
    public  $name;
    public  $age;
    public  $address;
    public $show;
    public function __construct(){
        $this->show = new ReturnMessage;
    }
    public function Show(){
        $msg =  "hello: ".$this->name;
        $this->show->return($msg);
    }
}
$user = new User();
$user->name = $_GET["name"];
$user->Show();