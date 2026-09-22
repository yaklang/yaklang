//go:build ignore

// Positive controls for the observer, not a production SCA capability.
package main

import (
 "fmt"
 "net"
 "os"
 "os/exec"
 "syscall"
)
func main() {
 _,e:=os.ReadFile("/etc/hosts");if e!=nil{panic(e)}
 _,e=os.Stat("/etc/hosts");if e!=nil{panic(e)}
 _,e=os.Lstat("/etc/hosts");if e!=nil{panic(e)}
 _,e=os.Readlink("/etc/hosts");fmt.Println("readlink",e)
 if f,e:=os.Create("/tmp/sca-audit-forbidden-file");e==nil{f.Close();panic("write unexpectedly allowed")}
 if e:=os.Mkdir("/tmp/sca-audit-forbidden-directory",0700);e==nil{panic("mkdir unexpectedly allowed")}
 if c,e:=net.Dial("tcp","127.0.0.1:1");e==nil{c.Close();panic("network unexpectedly allowed")}
 // Invalid descriptors/paths force failure without opening network or
 // replacing the control process; they still traverse the observed boundary.
 fmt.Println("connect",syscall.Connect(-1,&syscall.SockaddrInet4{Port:1}))
 fmt.Println("execve",syscall.Exec("/sca-audit-no-such-executable",[]string{"control"},os.Environ()))
 cmd:=exec.Command("/usr/bin/true");cmd.Stdin=os.Stdin;cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr
 if e:=cmd.Run();e==nil{panic("fork unexpectedly allowed")}
 fmt.Println("CONTROLS_PASSED")
}
