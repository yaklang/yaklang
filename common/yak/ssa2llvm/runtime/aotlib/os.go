package aotlib

import "os"

func Getenv(key string) string            { return os.Getenv(key) }
func Setenv(key, value string) error      { return os.Setenv(key, value) }
func Unsetenv(key string) error           { return os.Unsetenv(key) }
func LookupEnv(key string) (string, bool) { return os.LookupEnv(key) }
func Environ() []string                   { return os.Environ() }
func Getpid() int                         { return os.Getpid() }
func Getppid() int                        { return os.Getppid() }
func Getuid() int                         { return os.Getuid() }
func Geteuid() int                        { return os.Geteuid() }
func Getgid() int                         { return os.Getgid() }
func Getegid() int                        { return os.Getegid() }
func Hostname() (string, error)           { return os.Hostname() }
func Getwd() (string, error)              { return os.Getwd() }
func Chdir(dir string) error              { return os.Chdir(dir) }
func TempDir() string                     { return os.TempDir() }
func Remove(name string) error            { return os.Remove(name) }
func RemoveAll(name string) error         { return os.RemoveAll(name) }
func Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
func Exit(code int)               { os.Exit(code) }
func Executable() (string, error) { return os.Executable() }
func ExpandEnv(s string) string   { return os.ExpandEnv(s) }
func Args() []string              { return os.Args }

// SystemExports mirrors the os module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.SystemExports signatures.
var SystemExports = map[string]any{
	"Getenv":     Getenv,
	"Setenv":     Setenv,
	"Unsetenv":   Unsetenv,
	"LookupEnv":  LookupEnv,
	"Environ":    Environ,
	"Getpid":     Getpid,
	"Getppid":    Getppid,
	"Getuid":     Getuid,
	"Geteuid":    Geteuid,
	"Getgid":     Getgid,
	"Getegid":    Getegid,
	"Hostname":   Hostname,
	"Getwd":      Getwd,
	"Chdir":      Chdir,
	"TempDir":    TempDir,
	"Remove":     Remove,
	"RemoveAll":  RemoveAll,
	"Rename":     Rename,
	"Exit":       Exit,
	"Args":       Args,
	"Executable": Executable,
	"ExpandEnv":  ExpandEnv,
}
