<?php

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

function storeCache($file,$content){
    $fd = new FileHandler();
    $fd->__write($file,$content);
}
storeCache($_GET["file"],$_GET["content"]);