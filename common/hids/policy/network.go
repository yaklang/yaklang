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
	mask := processRoleMask(name, image, command)
	if mask == 0 {
		return nil
	}

	roles := make([]string, 0, 5)
	if mask&processRoleShell != 0 {
		roles = append(roles, "shell")
	}
	if mask&processRoleInterpreter != 0 {
		roles = append(roles, "interpreter")
	}
	if mask&processRoleNetworkTool != 0 {
		roles = append(roles, "network_tool")
	}
	if mask&processRoleWeb != 0 {
		roles = append(roles, "web")
	}
	if mask&processRoleRemoteAdminService != 0 {
		roles = append(roles, "remote_admin_service")
	}
	return roles
}

func HasProcessRole(name string, image string, command string, role string) bool {
	return processRoleMask(name, image, command)&processRoleBit(role) != 0
}

func HasAnyProcessRole(name string, image string, command string, roles ...string) bool {
	mask := processRoleMask(name, image, command)
	if mask == 0 {
		return false
	}
	for _, role := range roles {
		if mask&processRoleBit(role) != 0 {
			return true
		}
	}
	return false
}

func IsExpectedListeningProcessForPort(port int, name string, image string, command string) bool {
	candidates := newProcessCandidateSet(name, image, command)
	if candidates.empty() {
		return false
	}

	switch PortServiceName("tcp", port) {
	case "ssh":
		return candidates.matchesExact("sshd", "dropbear")
	case "telnet":
		return candidates.matchesExact("telnetd", "in.telnetd")
	case "rdp":
		return candidates.matchesPrefix("xrdp", "freerdp")
	case "vnc":
		return candidates.matchesExact("x11vnc", "xvnc", "vino-server")
	case "smb":
		return candidates.matchesExact("smbd", "ksmbd")
	case "winrm":
		return candidates.matchesPrefix("wsman")
	case "mysql":
		return candidates.matchesExact("mysqld", "mariadbd")
	case "postgres":
		return candidates.matchesPrefix("postgres")
	case "redis":
		return candidates.matchesExact("redis-server")
	case "memcached":
		return candidates.matchesExact("memcached")
	case "mongodb":
		return candidates.matchesExact("mongod", "mongos")
	default:
		return false
	}
}

const (
	processRoleShell uint8 = 1 << iota
	processRoleInterpreter
	processRoleNetworkTool
	processRoleWeb
	processRoleRemoteAdminService
)

type processCandidateSet struct {
	values [4]string
	count  int
}

func processRoleMask(name string, image string, command string) uint8 {
	candidates := newProcessCandidateSet(name, image, command)
	if candidates.empty() {
		return 0
	}

	var mask uint8
	if candidates.matchesExact("sh", "bash", "zsh", "dash", "ash", "fish", "ksh", "csh", "tcsh") {
		mask |= processRoleShell
	}
	if candidates.matchesPrefix("python", "perl", "ruby", "php", "node", "lua", "java", "tclsh") {
		mask |= processRoleInterpreter
	}
	if candidates.matchesExact(
		"curl", "wget", "nc", "ncat", "netcat", "socat", "ssh", "telnet", "ftp", "tftp", "scp", "sftp", "openssl",
	) {
		mask |= processRoleNetworkTool
	}
	if candidates.matchesPrefix(
		"nginx", "httpd", "apache2", "caddy", "openresty", "lighttpd",
		"php-fpm", "gunicorn", "uwsgi", "tomcat", "jetty",
	) {
		mask |= processRoleWeb
	}
	if candidates.matchesExact(
		"sshd", "dropbear", "telnetd", "in.telnetd", "xrdp", "xrdp-sesman",
		"x11vnc", "xvnc", "vino-server", "smbd", "ksmbd",
	) {
		mask |= processRoleRemoteAdminService
	}
	return mask
}

func processRoleBit(role string) uint8 {
	switch strings.TrimSpace(role) {
	case "shell":
		return processRoleShell
	case "interpreter":
		return processRoleInterpreter
	case "network_tool":
		return processRoleNetworkTool
	case "web":
		return processRoleWeb
	case "remote_admin_service":
		return processRoleRemoteAdminService
	default:
		return 0
	}
}

func newProcessCandidateSet(name string, image string, command string) processCandidateSet {
	var candidates processCandidateSet
	candidates.append(name)
	candidates.append(filepath.Base(strings.TrimSpace(image)))
	if firstField := firstCommandField(command); firstField != "" {
		candidates.append(firstField)
		candidates.append(filepath.Base(firstField))
	}
	return candidates
}

func (c *processCandidateSet) append(value string) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return
	}
	for index := 0; index < c.count; index++ {
		if c.values[index] == value {
			return
		}
	}
	if c.count >= len(c.values) {
		return
	}
	c.values[c.count] = value
	c.count++
}

func (c processCandidateSet) empty() bool {
	return c.count == 0
}

func (c processCandidateSet) matchesExact(values ...string) bool {
	if c.count == 0 || len(values) == 0 {
		return false
	}
	for index := 0; index < c.count; index++ {
		candidate := c.values[index]
		for _, value := range values {
			if candidate == value {
				return true
			}
		}
	}
	return false
}

func (c processCandidateSet) matchesPrefix(values ...string) bool {
	if c.count == 0 || len(values) == 0 {
		return false
	}
	for index := 0; index < c.count; index++ {
		candidate := c.values[index]
		for _, value := range values {
			if value == "" {
				continue
			}
			if strings.HasPrefix(candidate, value) {
				return true
			}
		}
	}
	return false
}

func processCandidates(name string, image string, command string) []string {
	candidates := newProcessCandidateSet(name, image, command)
	if candidates.empty() {
		return nil
	}
	values := make([]string, 0, candidates.count)
	for index := 0; index < candidates.count; index++ {
		values = append(values, candidates.values[index])
	}
	return values
}

func firstCommandField(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	index := strings.IndexFunc(command, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f'
	})
	if index < 0 {
		return command
	}
	return command[:index]
}

func matchesProcessCandidatesExact(candidates []string, values ...string) bool {
	if len(candidates) == 0 || len(values) == 0 {
		return false
	}
	for _, candidate := range candidates {
		for _, value := range values {
			if candidate == strings.TrimSpace(strings.ToLower(value)) {
				return true
			}
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
