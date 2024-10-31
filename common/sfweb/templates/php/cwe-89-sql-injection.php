<?php

class loader{
    public function execsql($sql){
        return mysqli_query($sql);
    }
}
class user{
    public function login(){
        $user = $_POST["user"];
        $pass = $_POST["pass"];
        $sql = "select * from users where username = ".$user." and pass = ".md5($pass);
        $ld = new Loader();
        $this->getLoader()->execsql($sql);
    }
    public function getLoader(){
        return new loader;
    }
}