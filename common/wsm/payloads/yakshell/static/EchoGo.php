%v
function ping(){
    return json_encode(array("status"=>"ok","msg"=>base64_encode("ok")));
}

echo function_exists("encrypt") ? encrypt(ping(),$pass) : "";