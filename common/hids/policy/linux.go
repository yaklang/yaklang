//go:build hids

package policy

import (
	"path/filepath"
	"strings"
)

var sensitiveIntegritySystemPaths = []string{
	"/etc/passwd",
	"/etc/shadow",
	"/etc/sudoers",
	"/etc/ld.so.preload",
	"/etc/environment",
	"/etc/profile",
	"/etc/profile.d",
	"/etc/bash.bashrc",
	"/etc/rc.local",
	"/etc/ssh/sshd_config",
	"/etc/crontab",
	"/etc/cron.d",
}

var sensitiveAuditOnlyPaths = []string{
	"/etc/group",
	"/etc/gshadow",
	"/etc/sudoers.d",
	"/etc/pam.d",
	"/etc/security",
	"/var/spool/cron",
	"/etc/systemd/system",
	"/lib/systemd/system",
	"/usr/lib/systemd/system",
}

var authorizedKeysSuffixes = []string{
	"/.ssh/authorized_keys",
	"/.ssh/authorized_keys2",
}

var systemELFArtifactRoots = []string{
	"/bin/",
	"/sbin/",
	"/usr/bin/",
	"/usr/sbin/",
	"/usr/local/bin/",
	"/usr/local/sbin/",
	"/lib/",
	"/lib64/",
	"/usr/lib/",
	"/usr/lib64/",
}

func NormalizePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = filepath.ToSlash(filepath.Clean(value))
	if value == "." {
		return ""
	}
	return value
}

func IsWritableTmpPath(value string) bool {
	normalized := NormalizePath(value)
	return hasAnyPrefix(normalized, "/tmp/", "/var/tmp/", "/dev/shm/")
}

func IsSystemELFArtifactPath(value string) bool {
	normalized := NormalizePath(value)
	return hasAnyPrefix(normalized, systemELFArtifactRoots...)
}

func IsSensitiveSystemPath(filePath string) bool {
	return matchesSensitivePath(NormalizePath(filePath), sensitiveIntegritySystemPaths)
}

func IsAuthorizedKeysPath(filePath string) bool {
	normalized := NormalizePath(filePath)
	for _, suffix := range authorizedKeysSuffixes {
		if normalized == suffix || strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

func IsSensitiveIntegrityPath(filePath string) bool {
	return IsSensitiveSystemPath(filePath) || IsAuthorizedKeysPath(filePath)
}

func IsSensitiveAuditPath(filePath string) bool {
	normalized := NormalizePath(filePath)
	return matchesSensitivePath(normalized, sensitiveIntegritySystemPaths) ||
		matchesSensitivePath(normalized, sensitiveAuditOnlyPaths) ||
		IsAuthorizedKeysPath(normalized)
}

func SensitiveAuditSeedPaths() []string {
	paths := make([]string, 0, len(sensitiveIntegritySystemPaths)+len(sensitiveAuditOnlyPaths))
	paths = append(paths, sensitiveIntegritySystemPaths...)
	paths = append(paths, sensitiveAuditOnlyPaths...)
	return append([]string(nil), paths...)
}

func IsSecurityControlTamperCommand(command string) bool {
	normalized := strings.ToLower(strings.TrimSpace(command))
	if normalized == "" {
		return false
	}

	for _, phrase := range []string{
		"auditctl -d",
		"auditctl -D",
		"auditctl -e 0",
		"systemctl stop auditd",
		"systemctl disable auditd",
		"systemctl mask auditd",
		"service auditd stop",
		"pkill auditd",
		"killall auditd",
		"setenforce 0",
		"iptables -f",
		"iptables --flush",
		"nft flush ruleset",
		"ufw disable",
	} {
		if strings.Contains(normalized, strings.ToLower(phrase)) {
			return true
		}
	}
	return false
}

func IsDownloadPipeShellCommand(command string) bool {
	normalized := normalizeCommandText(command)
	if normalized == "" {
		return false
	}
	if !containsAny(normalized, "curl ", "wget ", "fetch ") {
		return false
	}
	return containsAny(
		normalized,
		"| sh",
		"| bash",
		"| dash",
		"| ash",
		"| zsh",
		"| /bin/sh",
		"| /bin/bash",
		"| /usr/bin/sh",
		"| /usr/bin/bash",
	)
}

func IsSetuidSetgidBitCommand(command string) bool {
	normalized := normalizeCommandText(command)
	if normalized == "" || !strings.Contains(normalized, "chmod ") {
		return false
	}

	for _, token := range strings.Fields(normalized) {
		token = strings.Trim(token, `"'`+"`;:,")
		switch {
		case token == "+s":
			return true
		case strings.Contains(token, "u+s"), strings.Contains(token, "g+s"), strings.Contains(token, "a+s"):
			return true
		case isSpecialPermissionMode(token):
			return true
		}
	}
	return false
}

func IsPersistenceCommand(command string) bool {
	normalized := normalizeCommandText(command)
	if normalized == "" {
		return false
	}

	if containsAny(
		normalized,
		"systemctl enable ",
		"systemctl reenable ",
		"systemctl link ",
		"systemctl add-wants ",
		"systemctl add-requires ",
	) {
		return true
	}
	if strings.Contains(normalized, "crontab ") && !containsAny(normalized, "crontab -l", "crontab --list") {
		return true
	}
	if !containsAny(
		normalized,
		"/etc/ld.so.preload",
		"/etc/environment",
		"/etc/profile",
		"/etc/profile.d",
		"/etc/bash.bashrc",
		"/etc/rc.local",
		"/etc/cron.d",
		"/etc/crontab",
		"/var/spool/cron",
		"/etc/systemd/system",
		"/lib/systemd/system",
		"/usr/lib/systemd/system",
	) {
		return false
	}
	return containsAny(
		normalized,
		">",
		"tee ",
		"cp ",
		"mv ",
		"install ",
		"ln ",
		"sed -i",
		"chmod ",
	)
}

func IsReverseShellCommand(command string) bool {
	normalized := normalizeCommandText(command)
	if normalized == "" {
		return false
	}

	if containsAny(normalized, "/dev/tcp/", "/dev/udp/") &&
		containsAny(
			normalized,
			"bash -i",
			"sh -i",
			"dash -i",
			"ash -i",
			"zsh -i",
			"/bin/bash -i",
			"/bin/sh -i",
		) {
		return true
	}
	if containsAny(
		normalized,
		"nc -e ",
		"nc -c ",
		"ncat -e ",
		"ncat -c ",
		"netcat -e ",
		"netcat -c ",
	) {
		return true
	}
	if strings.Contains(normalized, "mkfifo ") &&
		containsAny(normalized, "|nc ", "| ncat ", "| netcat ", ";nc ", "; ncat ", "; netcat ", " nc ", " ncat ", " netcat ") &&
		containsAny(normalized, " sh", " bash", "/bin/sh", "/bin/bash") {
		return true
	}
	if strings.Contains(normalized, "socat ") &&
		containsAny(normalized, "tcp:", "tcp4:", "tcp6:") &&
		containsAny(normalized, "exec:", "system:") {
		return true
	}
	return false
}

func IsAccountManagementCommand(command string) bool {
	normalized := normalizeCommandText(command)
	if normalized == "" {
		return false
	}
	return containsAny(
		normalized,
		"useradd ",
		"usermod ",
		"userdel ",
		"groupadd ",
		"groupmod ",
		"groupdel ",
		"passwd ",
		"chpasswd ",
		"gpasswd ",
	)
}

func IsAuditMutationAction(action string) bool {
	normalized := strings.ToLower(strings.TrimSpace(action))
	if normalized == "" {
		return false
	}
	for _, keyword := range []string{
		"chmod",
		"chown",
		"write",
		"truncate",
		"create",
		"rename",
		"remove",
		"unlink",
		"delete",
		"mkdir",
		"rmdir",
	} {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	return false
}

func IsAuditReadAction(action string) bool {
	normalized := strings.ToLower(strings.TrimSpace(action))
	if normalized == "" || IsAuditMutationAction(normalized) {
		return false
	}
	for _, keyword := range []string{"open", "access", "read", "cat"} {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func normalizeCommandText(command string) string {
	normalized := strings.ToLower(strings.TrimSpace(command))
	if normalized == "" {
		return ""
	}
	return strings.Join(strings.Fields(normalized), " ")
}

func containsAny(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(value, strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func matchesSensitivePath(normalized string, suffixes []string) bool {
	if normalized == "" {
		return false
	}
	for _, suffix := range suffixes {
		if normalized == suffix || strings.HasSuffix(normalized, suffix) || strings.Contains(normalized, suffix+"/") {
			return true
		}
	}
	return false
}

func isSpecialPermissionMode(token string) bool {
	token = strings.TrimSpace(token)
	if len(token) == 5 && token[0] == '0' {
		token = token[1:]
	}
	if len(token) != 4 {
		return false
	}
	if !strings.ContainsRune("2467", rune(token[0])) {
		return false
	}
	for _, char := range token {
		if char < '0' || char > '7' {
			return false
		}
	}
	return true
}
