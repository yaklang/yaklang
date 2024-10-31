<?php

function loadTemplate($template,$msg){
    if(file_exists($template)){
        include($template);
    }else{
        return;
    }
}
class load{
    public $msg;
    public function __construct(){

    }
    public function setMsg($msg){
        $this->msg = $msg;
        return $this;
    }
    public function __load($path){
        return loadTemplate($path,$this->msg);
    }
}

$ld = new load();
$ld->setMsg("data")->__load($_GET[1]);