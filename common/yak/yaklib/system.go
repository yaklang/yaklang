package yaklib

import (
	"github.com/yaklang/yaklang/common/utils/sysproc"
	"net"
	"os"
	"runtime"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/utils/privileged"
)

var SystemExports = map[string]interface{}{
	"IsTCPPortOpen":             IsTCPPortOpen,
	"IsUDPPortOpen":             IsUDPPortOpen,
	"LookupHost":                lookupHost,
	"LookupIP":                  lookupIP,
	"IsTCPPortAvailable":        IsTCPPortAvailable,
	"IsUDPPortAvailable":        IsUDPPortAvailable,
	"GetRandomAvailableTCPPort": GetRandomAvailableTCPPort,
	"GetRandomAvailableUDPPort": GetRandomAvailableUDPPort,
	"IsRemoteTCPPortOpen":       IsRemoteTCPPortOpen,

	"GetMachineID": GetMachineID,

	"Remove":               Remove,
	"RemoveAll":            RemoveAll,
	"Rename":               Rename,
	"TempDir":              TempDir,
	"Getwd":                Getwd,
	"Getpid":               Getpid,
	"Getppid":              Getppid,
	"Getuid":               Getuid,
	"Geteuid":              Geteuid,
	"Getgid":               Getgid,
	"Getegid":              Getegid,
	"Environ":              Environ,
	"GetHomeDir":           GetHomeDir,
	"Hostname":             Hostname,
	"Unsetenv":             Unsetenv,
	"LookupEnv":            LookupEnv,
	"Clearenv":             Clearenv,
	"Setenv":               Setenv,
	"Getenv":               Getenv,
	"Exit":                 Exit,
	"Args":                 cli.OsArgs,
	"Stdout":               Stdout,
	"Stdin":                Stdin,
	"Stderr":               Stderr,
	"Executable":           Executable,
	"ExpandEnv":            ExpandEnv,
	"Pipe":                 Pipe,
	"Chdir":                Chdir,
	"Chmod":                Chmod,
	"Chown":                Chown,
	"OS":                   OS,
	"ARCH":                 ARCH,
	"IsPrivileged":         IsPrivileged,
	"GetDefaultDNSServers": GetDefaultDNSServers,
	"WaitConnect":          WaitConnect,
	"GetLocalAddress":      GetLocalAddress,
	"GetLocalIPv4Address":  GetLocalIPv4Address,
	"GetLocalIPv6Address":  GetLocalIPv6Address,

	"NewConnectionsWatcher": sysproc.NewWatcher,
	"NewProcessWatcher":     sysproc.NewProcessesWatcher,
}

// LookupHost 通过DNS服务器，根据域名查找IP
// 参数:
//   - i: 待查询的域名
//
// 返回值:
//   - 解析得到的 IP 字符串切片
//
// Example:
// ```
// os.LookupHost("www.yaklang.com")
// ```
func lookupHost(i string) []string {
	return netx.LookupAll(i)
}

// LookupIP 通过DNS服务器，根据域名查找IP
// 参数:
//   - i: 待查询的域名
//
// 返回值:
//   - 解析得到的 IP 字符串切片
//
// Example:
// ```
// os.LookupIP("www.yaklang.com")
// ```
func lookupIP(i string) []string {
	return netx.LookupAll(i)
}

// IsTCPPortOpen 检查本地TCP端口是否开放（被占用）
// 参数:
//   - p: 待检查的 TCP 端口号
//
// 返回值:
//   - 端口是否开放（已被监听）
//
// Example:
// ```
// os.IsTCPPortOpen(80)
// ```
func IsTCPPortOpen(p int) bool {
	return !utils.IsTCPPortAvailable(p)
}

// IsUDPPortOpen 检查本地UDP端口是否开放（被占用）
// 参数:
//   - p: 待检查的 UDP 端口号
//
// 返回值:
//   - 端口是否开放（已被占用）
//
// Example:
// ```
// os.IsUDPPortOpen(80)
// ```
func IsUDPPortOpen(p int) bool {
	return !utils.IsUDPPortAvailable(p)
}

// IsTCPPortAvailable 检查本地TCP端口是否可用（未被占用）
// 参数:
//   - p: 待检查的 TCP 端口号
//
// 返回值:
//   - 端口是否可用（可被监听）
//
// Example:
// ```
// os.IsTCPPortAvailable(80)
// ```
func IsTCPPortAvailable(p int) bool {
	return utils.IsTCPPortAvailable(p)
}

// IsUDPPortAvailable 检查本地UDP端口是否可用（未被占用）
// 参数:
//   - p: 待检查的 UDP 端口号
//
// 返回值:
//   - 端口是否可用
//
// Example:
// ```
// os.IsUDPPortAvailable(80)
// ```
func IsUDPPortAvailable(p int) bool {
	return utils.IsUDPPortAvailable(p)
}

// GetRandomAvailableTCPPort 获取一个随机可用的TCP端口
// 返回值:
//   - 一个当前可用的 TCP 端口号
//
// Example:
// ```
// tcp.Serve("127.0.0.1", os.GetRandomAvailableTCPPort())
// ```
func GetRandomAvailableTCPPort() int {
	return utils.GetRandomAvailableTCPPort()
}

// GetRandomAvailableUDPPort 获取一个随机可用的UDP端口
// 返回值:
//   - 一个当前可用的 UDP 端口号
//
// Example:
// ```
// udp.Serve("127.0.0.1", os.GetRandomAvailableTCPPort())
// ```
func GetRandomAvailableUDPPort() int {
	return utils.GetRandomAvailableUDPPort()
}

// IsRemoteTCPPortOpen 检查远程主机的TCP端口是否开放
// 参数:
//   - host: 远程主机地址（域名或 IP）
//   - p: 待检查的 TCP 端口号
//
// 返回值:
//   - 远程端口是否开放
//
// Example:
// ```
// os.IsRemoteTCPPortOpen("yaklang.com", 443) // true
// ```
func IsRemoteTCPPortOpen(host string, p int) bool {
	return utils.IsTCPPortOpen(host, p)
}

// GetMachineID 获取每个机器唯一的标识符
// 返回值:
//   - 当前机器的唯一标识字符串
//
// Example:
// ```
// os.GetMachineID()
// ```
func GetMachineID() string {
	return utils.GetMachineCode()
}

// Remove 删除指定的文件或空目录
// 参数:
//   - name: 待删除的文件或目录路径
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// os.Remove("/tmp/test.txt")
// ```
func Remove(name string) error {
	return os.Remove(name)
}

// RemoveAll 递归删除指定的路径及其包含的所有子路径
// 参数:
//   - name: 待删除的路径
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// os.RemoveAll("/tmp")
// ```
func RemoveAll(name string) error {
	return os.RemoveAll(name)
}

// Rename 重命名文件或目录，可以用于移动文件或目录
// 参数:
//   - oldpath: 原路径
//   - newpath: 目标路径
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// os.Rename("/tmp/test.txt", "/tmp/test2.txt")
// os.Rename("/tmp/test", "/root/test")
// ```
func Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// TempDir 获取用于存放临时文件的默认目录路径
// 返回值:
//   - 系统临时目录路径
//
// Example:
// ```
// dir = os.TempDir()
// assert dir != "", "TempDir should return a non-empty path"
// assert file.IsDir(dir), "TempDir should point to an existing directory"
// ```
func TempDir() string {
	return os.TempDir()
}

// Getwd 获取当前工作目录路径
// 返回值:
//   - 当前工作目录路径
//   - 错误信息
//
// Example:
// ```
// cwd, err = os.Getwd()
// ```
func Getwd() (string, error) {
	return os.Getwd()
}

// Getpid 获取当前进程的进程ID
// 返回值:
//   - 当前进程 ID
//
// Example:
// ```
// os.Getpid()
// ```
func Getpid() int {
	return os.Getpid()
}

// Getppid 获取当前进程的父进程ID
// 返回值:
//   - 当前进程的父进程 ID
//
// Example:
// ```
// os.Getppid()
// ```
func Getppid() int {
	return os.Getppid()
}

// Getuid 获取当前进程的用户ID
// 返回值:
//   - 当前进程的用户 ID
//
// Example:
// ```
// os.Getuid()
// ```
func Getuid() int {
	return os.Getuid()
}

// Geteuid 获取当前进程的有效用户ID
// 返回值:
//   - 当前进程的有效用户 ID
//
// Example:
// ```
// os.Geteuid()
// ```
func Geteuid() int {
	return os.Geteuid()
}

// Getgid 获取当前进程的组ID
// 返回值:
//   - 当前进程的组 ID
//
// Example:
// ```
// os.Getgid()
// ```
func Getgid() int {
	return os.Getgid()
}

// Getegid 获取当前进程的有效组ID
// 返回值:
//   - 当前进程的有效组 ID
//
// Example:
// ```
// os.Getegid()
// ```
func Getegid() int {
	return os.Getegid()
}

// Environ 获取表示环境变量的字符串切片，格式为"key=value"
// 返回值:
//   - 形如 "key=value" 的环境变量字符串切片
//
// Example:
// ```
// for env in os.Environ() {
// value = env.SplitN("=", 2)
// printf("key = %s, value = %v\n", value[0], value[1])
// }
// ```
func Environ() []string {
	return os.Environ()
}

// GetHomeDir 获取当前用户的家目录
// 返回值:
//   - 当前用户的家目录路径
//
// Example:
// ```
// os.GetHomeDir() // "/Users/yaklang"
// ```
func GetHomeDir() string {
	// key: HOME, USERPROFILE, HOMEDRIVE + HOMEPATH
	// macOS: HOME, USERPROFILE
	// Windows: USERPROFILE, HOMEDRIVE + HOMEPATH
	// Linux: HOME
	home := os.Getenv("HOME")
	if home != "" {
		return home
	}

	// Windows 环境
	userProfile := os.Getenv("USERPROFILE")
	if userProfile != "" {
		return userProfile
	}

	// Windows 环境的另一种情况
	homeDrive := os.Getenv("HOMEDRIVE")
	homePath := os.Getenv("HOMEPATH")
	if homeDrive != "" && homePath != "" {
		return homeDrive + homePath
	}

	// 如果都获取不到,返回当前目录
	pwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return pwd
}

// Hostname 获取主机名
// 返回值:
//   - 主机名
//   - 错误信息
//
// Example:
// ```
// name, err = os.Hostname()
// ```
func Hostname() (name string, err error) {
	return os.Hostname()
}

// Unsetenv 删除指定的环境变量
// 参数:
//   - key: 待删除的环境变量名
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// os.Setenv("YAK_DOC_UNSET", "v")
// os.Unsetenv("YAK_DOC_UNSET")
// assert os.Getenv("YAK_DOC_UNSET") == "", "Unsetenv should remove the environment variable"
// ```
func Unsetenv(key string) error {
	return os.Unsetenv(key)
}

// LookupEnv 获取指定的环境变量的值，并返回该变量是否存在
// 参数:
//   - key: 环境变量名
//
// 返回值:
//   - 环境变量的值
//   - 该环境变量是否存在
//
// Example:
// ```
// os.Setenv("YAK_DOC_LOOKUP", "hello")
// value, ok = os.LookupEnv("YAK_DOC_LOOKUP")
// println(value)   // OUT: hello
// assert ok, "LookupEnv should report the variable exists"
// assert value == "hello", "LookupEnv should return the variable value"
// ```
func LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

// Clearenv 清空当前进程的所有环境变量
// Example:
// ```
// os.Clearenv()
// ```
func Clearenv() {
	os.Clearenv()
}

// Setenv 设置指定的环境变量
// 参数:
//   - key: 环境变量名
//   - value: 环境变量值
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// os.Setenv("YAK_DOC_SET", "yaklang")
// println(os.Getenv("YAK_DOC_SET"))   // OUT: yaklang
// assert os.Getenv("YAK_DOC_SET") == "yaklang", "Setenv then Getenv should round-trip"
// ```
func Setenv(key, value string) error {
	return os.Setenv(key, value)
}

// Getenv 获取指定的环境变量的值，如果不存在则返回空字符串
// 参数:
//   - key: 环境变量名
//
// 返回值:
//   - 环境变量的值，不存在时为空字符串
//
// Example:
// ```
// os.Setenv("YAK_DOC_GET", "world")
// value = os.Getenv("YAK_DOC_GET")
// println(value)   // OUT: world
// assert value == "world", "Getenv should return the value set by Setenv"
// ```
func Getenv(key string) string {
	return os.Getenv(key)
}

// Exit 以指定状态码退出当前进程
// 参数:
//   - code: 进程退出状态码（0 表示成功）
//
// Example:
// ```
// os.Exit(0)
// ```
func Exit(code int) {
	os.Exit(code)
}

// Args 获取命令行参数
// Example:
// ```
// for arg in os.Args {
// println(arg)
// }
// ```
func osArgs() []string {
	return os.Args
}

// Executable 获取当前可执行文件的路径
// 返回值:
//   - 当前可执行文件的绝对路径
//   - 错误信息
//
// Example:
// ```
// path, err = os.Executable()
// ```
func Executable() (string, error) {
	return os.Executable()
}

// ExpandEnv 将字符串中的 ${var} 或 $var 替换为其对应环境变量的值
// 参数:
//   - s: 含有环境变量引用的字符串
//
// 返回值:
//   - 替换后的字符串
//
// Example:
// ```
// os.Setenv("YAK_DOC_EXPAND", "yak")
// result = os.ExpandEnv("hello $YAK_DOC_EXPAND")
// println(result)   // OUT: hello yak
// assert result == "hello yak", "ExpandEnv should substitute the variable value"
// ```
func ExpandEnv(s string) string {
	return os.ExpandEnv(s)
}

// Pipe 创建一个管道，返回一个读取端和一个写入端以及错误
// 它实际是 io.Pipe 的别名
// 返回值:
//   - 管道的读取端文件对象
//   - 管道的写入端文件对象
//   - 错误信息
//
// Example:
// ```
// r, w, err = os.Pipe()
// die(err)
//
//	go func {
//	    w.WriteString("hello yak")
//	    w.Close()
//	}
//
// bytes, err = io.ReadAll(r)
// die(err)
// dump(bytes)
// ```
func Pipe() (r *os.File, w *os.File, err error) {
	return os.Pipe()
}

// Chdir 改变当前工作目录
// 参数:
//   - dir: 目标工作目录路径
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// err = os.Chdir("/tmp")
// ```
func Chdir(dir string) error {
	return os.Chdir(dir)
}

// Chmod 改变指定文件或目录的权限
// 参数:
//   - name: 文件或目录路径
//   - mode: 目标权限（如 0o777）
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// err = os.Chmod("/tmp/test.txt", 0777)
// ```
func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

// Chown 改变指定文件或目录的所有者和所属组
// 参数:
//   - name: 文件或目录路径
//   - uid: 新的所有者用户 ID
//   - gid: 新的所属组 ID
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// err = os.Chown("/var/www/html/test.txt", 1000, 1000)
// ```
func Chown(name string, uid, gid int) error {
	return os.Chown(name, uid, gid)
}

// GetDefaultDNSServers 获取默认的DNS服务器ip对应的字符串切片
// 返回值:
//   - 默认 DNS 服务器 IP 字符串切片
//
// Example:
// ```
// os.GetDefaultDNSServers()
// ```
func GetDefaultDNSServers() []string {
	return netx.NewDefaultReliableDNSConfig().SpecificDNSServers
}

// WaitConnect 等待一个地址的端口开放，直到超时，如果超时则返回错误，这通常用于等待并确保一个服务启动
// 参数:
//   - addr: 目标地址（host:port）
//   - timeout: 最长等待时间（秒）
//
// 返回值:
//   - 错误信息（超时或连接失败时非空）
//
// Example:
// ```
// timeout, _ = time.ParseDuration("1m")
// ctx, cancel = context.WithTimeout(context.New(), timeout)
//
//	go func() {
//	    err = tcp.Serve("127.0.0.1", 8888, tcp.serverCallback(func (conn) {
//	    conn.Send("hello world")
//	    conn.Close()
//	}), tcp.serverContext(ctx))
//
//	    die(err)
//	}()
//
// os.WaitConnect("127.0.0.1:8888", 5)~ // 等待tcp服务器启动
// conn = tcp.Connect("127.0.0.1", 8888)~
// bytes = conn.Recv()~
// println(string(bytes))
// ```
func WaitConnect(addr string, timeout float64) error {
	return utils.WaitConnect(addr, timeout)
}

// Stdin 标准输入
var Stdin = os.Stdin

// Stdout 标准输出
var Stdout = os.Stdout

// Stderr 标准错误
var Stderr = os.Stderr

// OS 当前操作系统名
var OS = runtime.GOOS

// ARCH 当前操作系统的运行架构：它的值可能是386、amd64、arm、s390x等
var ARCH = runtime.GOARCH

// IsPrivileged 当前是否是特权模式
var IsPrivileged = privileged.GetIsPrivileged()

// GetLocalAddress 获取本地IP地址
// 返回值:
//   - 本地网卡 IP 地址字符串切片
//
// Example:
// ```
// os.GetLocalAddress() // ["192.168.1.103", "fe80::605a:5ff:fefb:5405"]
// ```
func GetLocalAddress() []string {
	ret, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	results := make([]string, len(ret))
	for i, a := range ret {
		if r, ok := a.(*net.IPNet); ok {
			results[i] = r.IP.String()
		}
	}
	return results
}

// GetLocalIPv4Address 获取本地IPv4地址
// 返回值:
//   - 本地非回环 IPv4 地址字符串切片
//
// Example:
// ```
// os.GetLocalIPv4Address() // ["192.168.3.103"]
// ```
func GetLocalIPv4Address() []string {
	var r []string
	for _, result := range GetLocalAddress() {
		if utils.IsLoopback(result) {
			continue
		}
		if utils.IsIPv4(result) {
			r = append(r, result)
		}
	}
	return r
}

// GetLocalIPv6Address 获取本地IPv6地址
// 返回值:
//   - 本地非回环 IPv6 地址字符串切片
//
// Example:
// ```
// os.GetLocalIPv6Address() // ["fe80::605a:5ff:fefb:5405"]
// ```
func GetLocalIPv6Address() []string {
	var r []string
	for _, result := range GetLocalAddress() {
		if utils.IsLoopback(result) {
			continue
		}
		if utils.IsIPv6(result) {
			r = append(r, result)
		}
	}
	return r
}
