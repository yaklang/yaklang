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
// Example:
// ```
// os.LookupHost("www.yaklang.com")
// ```
func lookupHost(i string) []string {
	return netx.LookupAll(i)
}

// LookupIP 通过DNS服务器，根据域名查找IP
// Example:
// ```
// os.LookupIP("www.yaklang.com")
// ```
func lookupIP(i string) []string {
	return netx.LookupAll(i)
}

// IsTCPPortOpen 检查TCP端口是否开放
// Example:
// ```
// os.IsTCPPortOpen(80)
// ```
func IsTCPPortOpen(p int) bool {
	return !utils.IsTCPPortAvailable(p)
}

// IsUDPPortOpen 检查UDP端口是否开放
// Example:
// ```
// os.IsUDPPortOpen(80)
// ```
func IsUDPPortOpen(p int) bool {
	return !utils.IsUDPPortAvailable(p)
}

// IsTCPPortAvailable 检查TCP端口是否可用
// Example:
// ```
// os.IsTCPPortAvailable(80)
// ```
func IsTCPPortAvailable(p int) bool {
	return utils.IsTCPPortAvailable(p)
}

// IsUDPPortAvailable 检查UDP端口是否可用
// Example:
// ```
// os.IsUDPPortAvailable(80)
// ```
func IsUDPPortAvailable(p int) bool {
	return utils.IsUDPPortAvailable(p)
}

// GetRandomAvailableTCPPort 获取随机可用的TCP端口
// Example:
// ```
// tcp.Serve("127.0.0.1", os.GetRandomAvailableTCPPort())
// ```
func GetRandomAvailableTCPPort() int {
	return utils.GetRandomAvailableTCPPort()
}

// GetRandomAvailableUDPPort 获取随机可用的UDP端口
// Example:
// ```
// udp.Serve("127.0.0.1", os.GetRandomAvailableTCPPort())
// ```
func GetRandomAvailableUDPPort() int {
	return utils.GetRandomAvailableUDPPort()
}

// IsRemoteTCPPortOpen 检查远程TCP端口是否开放
// Example:
// ```
// os.IsRemoteTCPPortOpen("yaklang.com", 443) // true
// ```
func IsRemoteTCPPortOpen(host string, p int) bool {
	return utils.IsTCPPortOpen(host, p)
}

// GetMachineID 获取每个机器唯一的标识符
// Example:
// ```
// os.GetMachineID()
// ```
func GetMachineID() string {
	return utils.GetMachineCode()
}

// Remove 删除指定的文件或目录
// Example:
// ```
// os.Remove("/tmp/test.txt")
// ```
func Remove(name string) error {
	return os.Remove(name)
}

// RemoveAll 递归删除指定的路径及其子路径
// Example:
// ```
// os.RemoveAll("/tmp")
// ```
func RemoveAll(name string) error {
	return os.RemoveAll(name)
}

// Rename 重命名文件或目录，可以用于移动文件或目录
// Example:
// ```
// os.Rename("/tmp/test.txt", "/tmp/test2.txt")
// os.Rename("/tmp/test", "/root/test")
// ```
func Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// TempDir 获取用于存放临时文件的默认目录路径
// Example:
// ```
// os.TempDir()
// ```
func TempDir() string {
	return os.TempDir()
}

// Getwd 获取当前工作目录路径
// Example:
// ```
// cwd, err = os.Getwd()
// ```
func Getwd() (string, error) {
	return os.Getwd()
}

// Getpid 获取当前进程的进程ID
// Example:
// ```
// os.Getpid()
// ```
func Getpid() int {
	return os.Getpid()
}

// Getppid  获取当前进程的父进程ID
// Example:
// ```
// os.Getppid()
// ```
func Getppid() int {
	return os.Getppid()
}

// Getuid 获取当前进程的用户ID
// Example:
// ```
// os.Getuid()
// ```
func Getuid() int {
	return os.Getuid()
}

// Geteuid 获取当前进程的有效用户ID
// Example:
// ```
// os.Geteuid()
// ```
func Geteuid() int {
	return os.Geteuid()
}

// Getgid 获取当前进程的组ID
// Example:
// ```
// os.Getgid()
// ```
func Getgid() int {
	return os.Getgid()
}

// Getegid 获取当前进程的有效组ID
// Example:
// ```
// os.Getegid()
// ```
func Getegid() int {
	return os.Getegid()
}

// Environ 获取表示环境变量的字符串切片，格式为"key=value"
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
// Example:
// ```
// name, err = os.Hostname()
// ```
func Hostname() (name string, err error) {
	return os.Hostname()
}

// Unsetenv 删除指定的环境变量
// Example:
// ```
// os.Unsetenv("PATH")
// ```
func Unsetenv(key string) error {
	return os.Unsetenv(key)
}

// LookupEnv 获取指定的环境变量的值
// Example:
// ```
// value, ok = os.LookupEnv("PATH")
// ```
func LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

// Clearenv 清空所有环境变量
// Example:
// ```
// os.Clearenv()
// ```
func Clearenv() {
	os.Clearenv()
}

// Setenv 设置指定的环境变量
// Example:
// ```
// os.Setenv("PATH", "/usr/local/bin:/usr/bin:/bin")
// ```
func Setenv(key, value string) error {
	return os.Setenv(key, value)
}

// Getenv 获取指定的环境变量的值，如果不存在则返回空字符串
// Example:
// ```
// value = os.Getenv("PATH")
// ```
func Getenv(key string) string {
	return os.Getenv(key)
}

// Exit 退出当前进程
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
// Example:
// ```
// path, err = os.Executable()
// ```
func Executable() (string, error) {
	return os.Executable()
}

// ExpandEnv  将字符串中的${var}或$var替换为其对应环境变量名的值
// Example:
// ```
// os.ExpandEnv("PATH = $PATH")
// ```
func ExpandEnv(s string) string {
	return os.ExpandEnv(s)
}

// Pipe 创建一个管道，返回一个读取端和一个写入端以及错误
// 它实际是 io.Pipe 的别名
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
// Example:
// ```
// err = os.Chdir("/tmp")
// ```
func Chdir(dir string) error {
	return os.Chdir(dir)
}

// Chmod 改变指定文件或目录的权限
// Example:
// ```
// err = os.Chmod("/tmp/test.txt", 0777)
// ```
func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

// Chown 改变指定文件或目录的所有者和所属组
// Example:
// ```
// err = os.Chown("/var/www/html/test.txt", 1000, 1000)
// ```
func Chown(name string, uid, gid int) error {
	return os.Chown(name, uid, gid)
}

// GetDefaultDNSServers 获取默认的DNS服务器ip对应的字符串切片
// Example:
// ```
// os.GetDefaultDNSServers()
// ```
func GetDefaultDNSServers() []string {
	return netx.NewDefaultReliableDNSConfig().SpecificDNSServers
}

// WaitConnect 等待一个地址的端口开放或指导超时时间，如果超时则返回错误，这通常用于等待并确保一个服务启动
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
