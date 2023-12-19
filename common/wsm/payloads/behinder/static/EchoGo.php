@error_reporting(0);
function main($content)
{
	$result = array();
	$result["status"] = base64_encode("success");
    $result["msg"] = base64_encode($content);
    @session_start();

    echo encrypt(json_encode($result));
}
