//go:build hids

package policy

import (
	"net/netip"
	"path/filepath"
	"strings"
)

func AddressScope(value string) string {
	address := strings.Trim(strings.TrimSpace(value), "[]")
	if address == "" {
		return "unknown"
	}

	parsed, err := netip.ParseAddr(address)
	if err != nil {
		return "invalid"
	}
	switch {
	case parsed.IsLoopback():
		return "loopback"
	case parsed.IsPrivate():
		return "private"
	case parsed.IsMulticast(), parsed.IsInterfaceLocalMulticast(), parsed.IsLinkLocalMulticast():
		return "multicast"
	case parsed.IsLinkLocalUnicast():
		return "link_local"
	case parsed.IsUnspecified():
		return "unspecified"
	default:
		return "public"
	}
}

func PortServiceName(protocol string, port int) string {
	if port <= 0 {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "", "tcp", "tcp4", "tcp6", "udp", "udp4", "udp6":
	default:
		return ""
	}

	switch port {
	case 22:
		return "ssh"
	case 23:
		return "telnet"
	case 25, 465, 587:
		return "smtp"
	case 53:
		return "dns"
	case 80, 8080, 8000, 8888:
		return "http"
	case 110, 995:
		return "pop3"
	case 123:
		return "ntp"
	case 143, 993:
		return "imap"
	case 389, 636:
		return "ldap"
	case 443, 8443:
		return "https"
	case 445:
		return "smb"
	case 1080:
		return "socks"
	case 3128:
		return "proxy"
	case 3306:
		return "mysql"
	case 3389:
		return "rdp"
	case 5432:
		return "postgres"
	case 5900, 5901, 5902, 5903, 5904, 5905:
		return "vnc"
	case 5985, 5986:
		return "winrm"
	case 6379:
		return "redis"
	case 11211:
		return "memcached"
	case 6443:
		return "k8s-api"
	case 27017, 27018:
		return "mongodb"
	case 9001:
		return "tor-or"
	case 9050:
		return "tor-socks"
	case 9051:
		return "tor-control"
	default:
		return ""
	}
}

func IsRemoteAdminPort(protocol string, port int) bool {
	switch PortServiceName(protocol, port) {
	case "ssh", "telnet", "rdp", "vnc", "winrm", "smb":
		return true
	default:
		return false
	}
}

func IsProxyOrTorPort(protocol string, port int) bool {
	switch PortServiceName(protocol, port) {
	case "socks", "proxy", "tor-or", "tor-socks", "tor-control":
		return true
	default:
		return false
	}
}

func IsMetadataServiceAddress(value string) bool {
	address := strings.Trim(strings.TrimSpace(value), "[]")
	if address == "" {
		return false
	}
	if address == "169.254.169.254" {
		return true
	}
	parsed, err := netip.ParseAddr(address)
	if err != nil {
		return false
	}
	return parsed.String() == "169.254.169.254"
}

func IsDataServicePort(protocol string, port int) bool {
	switch PortServiceName(protocol, port) {
	case "mysql", "postgres", "redis", "memcached", "mongodb":
		return true
	default:
		return false
	}
}

func IsKubernetesAPIPort(protocol string, port int) bool {
	return PortServiceName(protocol, port) == "k8s-api"
}

func ProcessRoles(name string, image string, command string) []string {
	candidates := processCandidates(name, image, command)
	if len(candidates) == 0 {
		return nil
	}

	roles := make([]string, 0, 5)
	appendRole := func(role string) {
		for _, existing := range roles {
			if existing == role {
				return
			}
		}
		roles = append(roles, role)
	}

	if matchesProcessCandidatesExact(candidates,
		"sh", "bash", "zsh", "dash", "ash", "fish", "ksh", "csh", "tcsh",
	) {
		appendRole("shell")
	}
	if matchesProcessCandidatesPrefix(candidates,
		"python", "perl", "ruby", "php", "node", "lua", "java", "tclsh",
	) {
		appendRole("interpreter")
	}
	if matchesProcessCandidatesExact(candidates,
		"curl", "wget", "nc", "ncat", "netcat", "socat", "ssh", "telnet", "ftp", "tftp", "scp", "sftp", "openssl",
	) {
		appendRole("network_tool")
	}
	if matchesProcessCandidatesPrefix(candidates,
		"nginx", "httpd", "apache2", "caddy", "openresty", "lighttpd",
		"php-fpm", "gunicorn", "uwsgi", "uWSGI", "tomcat", "jetty",
	) {
		appendRole("web")
	}
	if matchesProcessCandidatesExact(candidates,
		"sshd", "dropbear", "telnetd", "in.telnetd", "xrdp", "xrdp-sesman",
		"x11vnc", "xvnc", "vino-server", "smbd", "ksmbd",
	) {
		appendRole("remote_admin_service")
	}

	if len(roles) == 0 {
		return nil
	}
	return roles
}

func HasProcessRole(name string, image string, command string, role string) bool {
	role = strings.TrimSpace(role)
	if role == "" {
		return false
	}
	for _, candidate := range ProcessRoles(name, image, command) {
		if candidate == role {
			return true
		}
	}
	return false
}

func HasAnyProcessRole(name string, image string, command string, roles ...string) bool {
	for _, role := range roles {
		if HasProcessRole(name, image, command, role) {
			return true
		}
	}
	return false
}

func IsExpectedListeningProcessForPort(port int, name string, image string, command string) bool {
	candidates := processCandidates(name, image, command)
	if len(candidates) == 0 {
		return false
	}

	switch PortServiceName("tcp", port) {
	case "ssh":
		return matchesProcessCandidatesExact(candidates, "sshd", "dropbear")
	case "telnet":
		return matchesProcessCandidatesExact(candidates, "telnetd", "in.telnetd")
	case "rdp":
		return matchesProcessCandidatesPrefix(candidates, "xrdp", "freerdp")
	case "vnc":
		return matchesProcessCandidatesExact(candidates, "x11vnc", "xvnc", "vino-server")
	case "smb":
		return matchesProcessCandidatesExact(candidates, "smbd", "ksmbd")
	case "winrm":
		return matchesProcessCandidatesPrefix(candidates, "wsman")
	case "mysql":
		return matchesProcessCandidatesExact(candidates, "mysqld", "mariadbd")
	case "postgres":
		return matchesProcessCandidatesPrefix(candidates, "postgres")
	case "redis":
		return matchesProcessCandidatesExact(candidates, "redis-server")
	case "memcached":
		return matchesProcessCandidatesExact(candidates, "memcached")
	case "mongodb":
		return matchesProcessCandidatesExact(candidates, "mongod", "mongos")
	default:
		return false
	}
}

func processCandidates(name string, image string, command string) []string {
	seen := map[string]struct{}{}
	values := make([]string, 0, 6)
	appendCandidate := func(value string) {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}

	appendCandidate(name)
	appendCandidate(filepath.Base(strings.TrimSpace(image)))

	if firstField := firstCommandField(command); firstField != "" {
		appendCandidate(firstField)
		appendCandidate(filepath.Base(firstField))
	}
	return values
}

func firstCommandField(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func matchesProcessCandidatesExact(candidates []string, values ...string) bool {
	if len(candidates) == 0 || len(values) == 0 {
		return false
	}
	exact := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(strings.ToLower(value))
		if trimmed == "" {
			continue
		}
		exact[trimmed] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, exists := exact[candidate]; exists {
			return true
		}
	}
	return false
}

func matchesProcessCandidatesPrefix(candidates []string, values ...string) bool {
	if len(candidates) == 0 || len(values) == 0 {
		return false
	}
	for _, candidate := range candidates {
		for _, value := range values {
			prefix := strings.TrimSpace(strings.ToLower(value))
			if prefix == "" {
				continue
			}
			if strings.HasPrefix(candidate, prefix) {
				return true
			}
		}
	}
	return false
}
