package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/sftp"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	reSSHHostLine          = regexp.MustCompile(`(?im)^\s*ssh(?:[_\s-]host)\s*[:：=]\s*(.+?)\s*$`)
	reSSHUserLine          = regexp.MustCompile(`(?im)^\s*ssh(?:[_\s-]user(?:name)?)\s*[:：=]\s*(.+?)\s*$`)
	reSSHPasswordLine      = regexp.MustCompile(`(?im)^\s*ssh(?:[_\s-]pass(?:word)?)\s*[:：=]\s*(.+?)\s*$`)
	reSSHKeyLine           = regexp.MustCompile(`(?im)^\s*ssh(?:[_\s-]?key(?:[_\s-]?path)?)\s*[:：=]\s*(.+?)\s*$`)
	reSSHKeyPassphraseLine = regexp.MustCompile(`(?im)^\s*ssh(?:[_\s-]?key(?:[_\s-]?pass(?:phrase)?))\s*[:：=]\s*(.+?)\s*$`)
	reSSHConnectLine       = regexp.MustCompile(`(?im)^\s*ssh\s*[:：=]\s*(.+?)\s*$`)
	reRemoteCodePathLine   = regexp.MustCompile(`(?im)^\s*(?:remote(?:[_\s-]?code(?:[_\s-]?path)?)|remote[_\s-]?source|ssh[_\s-]?code(?:[_\s-]?path)?|源码(?:路径)?)\s*[:：=]\s*(.+?)\s*$`)
)

// SSHRemoteSourceConfigured reports whether user asked to pull code over SSH.
func SSHRemoteSourceConfigured(parsed *ParsedUserInput) bool {
	if parsed == nil {
		return false
	}
	if strings.TrimSpace(parsed.RemoteCodePath) == "" {
		return false
	}
	host := strings.TrimSpace(parsed.SSHHost)
	user := strings.TrimSpace(parsed.SSHUsername)
	if host == "" || user == "" {
		return false
	}
	if strings.TrimSpace(parsed.SSHPassword) == "" && strings.TrimSpace(parsed.SSHPrivateKey) == "" {
		return false
	}
	return true
}

func applySSHFieldsFromUserText(out *ParsedUserInput, userText string) {
	if out == nil {
		return
	}
	for _, line := range strings.Split(userText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case reSSHConnectLine.MatchString(line):
			parseSSHConnectSpec(out, firstSubmatch(reSSHConnectLine, line))
		case reSSHKeyPassphraseLine.MatchString(line):
			out.SSHKeyPassphrase = strings.TrimSpace(firstSubmatch(reSSHKeyPassphraseLine, line))
		case reSSHKeyLine.MatchString(line):
			out.SSHPrivateKey = NormalizePathFromUserInput(firstSubmatch(reSSHKeyLine, line))
		case reSSHHostLine.MatchString(line):
			out.SSHHost = parseSSHHostValue(firstSubmatch(reSSHHostLine, line))
		case reSSHUserLine.MatchString(line):
			out.SSHUsername = NormalizePathFromUserInput(firstSubmatch(reSSHUserLine, line))
		case reSSHPasswordLine.MatchString(line):
			out.SSHPassword = strings.TrimSpace(firstSubmatch(reSSHPasswordLine, line))
		case reRemoteCodePathLine.MatchString(line):
			setRemoteCodePath(out, firstSubmatch(reRemoteCodePathLine, line))
		}
	}
	if out.SSHUsername == "" && strings.Contains(out.SSHHost, "@") {
		var tmp ParsedUserInput
		parseSSHConnectSpec(&tmp, out.SSHHost)
		if tmp.SSHUsername != "" {
			out.SSHUsername = tmp.SSHUsername
			out.SSHHost = tmp.SSHHost
		}
	}
}

func firstSubmatch(re *regexp.Regexp, line string) string {
	m := re.FindStringSubmatch(line)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func setRemoteCodePath(out *ParsedUserInput, raw string) {
	raw = NormalizePathFromUserInput(raw)
	if raw == "" {
		return
	}
	if strings.HasPrefix(strings.ToLower(raw), "ssh://") {
		parseSSHRemoteURL(out, raw)
		return
	}
	out.RemoteCodePath = filepath.ToSlash(raw)
}

func parseSSHRemoteURL(out *ParsedUserInput, raw string) {
	rest := strings.TrimPrefix(raw, "ssh://")
	rest = strings.TrimPrefix(rest, "SSH://")
	var userInfo, hostPart, remotePath string
	if i := strings.Index(rest, "@"); i >= 0 {
		userInfo = rest[:i]
		rest = rest[i+1:]
	}
	if i := strings.Index(rest, "/"); i >= 0 {
		hostPart = rest[:i]
		remotePath = rest[i:]
	} else {
		hostPart = rest
	}
	if userInfo != "" {
		if j := strings.Index(userInfo, ":"); j >= 0 {
			out.SSHUsername = userInfo[:j]
			out.SSHPassword = userInfo[j+1:]
		} else {
			out.SSHUsername = userInfo
		}
	}
	if hostPart != "" {
		out.SSHHost = hostPart
	}
	if remotePath != "" {
		out.RemoteCodePath = filepath.ToSlash(remotePath)
	}
}

// parseSSHConnectSpec parses values like root@192.168.1.4:22 or root@host:/opt/src.
func parseSSHConnectSpec(out *ParsedUserInput, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if strings.HasPrefix(strings.ToLower(raw), "ssh://") {
		parseSSHRemoteURL(out, raw)
		return
	}
	at := strings.LastIndex(raw, "@")
	if at <= 0 {
		out.SSHHost = raw
		return
	}
	out.SSHUsername = raw[:at]
	hostPart := raw[at+1:]
	if i := strings.Index(hostPart, "/"); i >= 0 {
		out.SSHHost = hostPart[:i]
		out.RemoteCodePath = filepath.ToSlash(hostPart[i:])
		return
	}
	out.SSHHost = hostPart
}

func parseSSHHostValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "@") {
		var tmp ParsedUserInput
		parseSSHConnectSpec(&tmp, raw)
		return tmp.SSHHost
	}
	return raw
}

func mergeSSHFields(base, overlay *ParsedUserInput) {
	if base == nil || overlay == nil {
		return
	}
	if strings.TrimSpace(base.SSHHost) == "" {
		base.SSHHost = overlay.SSHHost
	}
	if base.SSHPort == 0 && overlay.SSHPort > 0 {
		base.SSHPort = overlay.SSHPort
	}
	if strings.TrimSpace(base.SSHUsername) == "" {
		base.SSHUsername = overlay.SSHUsername
	}
	if strings.TrimSpace(base.SSHPassword) == "" {
		base.SSHPassword = overlay.SSHPassword
	}
	if strings.TrimSpace(base.SSHPrivateKey) == "" {
		base.SSHPrivateKey = overlay.SSHPrivateKey
	}
	if strings.TrimSpace(base.SSHKeyPassphrase) == "" {
		base.SSHKeyPassphrase = overlay.SSHKeyPassphrase
	}
	if strings.TrimSpace(base.RemoteCodePath) == "" {
		base.RemoteCodePath = overlay.RemoteCodePath
	}
}

func sshDial(parsed *ParsedUserInput) (*utils.SSHClient, error) {
	if parsed == nil {
		return nil, utils.Error("nil parsed")
	}
	host, port, err := utils.ParseStringToHostPort(strings.TrimSpace(parsed.SSHHost))
	if err != nil || host == "" {
		host = strings.TrimSpace(parsed.SSHHost)
		port = parsed.SSHPort
	}
	if port <= 0 {
		port = 22
	}
	addr := utils.HostPort(host, port)
	user := strings.TrimSpace(parsed.SSHUsername)
	if user == "" {
		user = "root"
	}
	keyPath := strings.TrimSpace(parsed.SSHPrivateKey)
	if keyPath != "" {
		keyPath = expandHomePath(keyPath)
		passphrase := strings.TrimSpace(parsed.SSHKeyPassphrase)
		if passphrase != "" {
			return utils.SSHDialWithKeyWithPassphrase(addr, user, keyPath, passphrase)
		}
		return utils.SSHDialWithKey(addr, user, keyPath)
	}
	pass := strings.TrimSpace(parsed.SSHPassword)
	if pass == "" {
		return nil, utils.Error("ssh auth required: set ssh_password or ssh_key")
	}
	return utils.SSHDialWithPasswd(addr, user, pass)
}

func expandHomePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func localRemoteCodeCacheDir(workDir string, parsed *ParsedUserInput) (string, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return "", utils.Error("empty work dir for remote code cache")
	}
	remote := strings.TrimSpace(parsed.RemoteCodePath)
	baseName := filepath.Base(strings.TrimSuffix(remote, "/"))
	if baseName == "" || baseName == "." || baseName == string(filepath.Separator) {
		baseName = "source"
	}
	host := strings.TrimSpace(parsed.SSHHost)
	host = strings.ReplaceAll(host, ":", "_")
	host = strings.ReplaceAll(host, "@", "_")
	dir := filepath.Join(workDir, "remote_code", host, baseName)
	return filepath.Abs(dir)
}

// ResolveRemoteCodePath pulls remote source over SSH when configured and sets parsed.CodePath.
func ResolveRemoteCodePath(ctx context.Context, r aicommon.AIInvokeRuntime, workDir string, parsed *ParsedUserInput) (*ParsedUserInput, error) {
	if parsed == nil {
		return nil, utils.Error("nil parsed")
	}
	if !SSHRemoteSourceConfigured(parsed) {
		return parsed, nil
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	localDir, err := localRemoteCodeCacheDir(workDir, parsed)
	if err != nil {
		return nil, err
	}
	if st, statErr := os.Stat(localDir); statErr == nil && st.IsDir() && hasAnyFile(localDir) {
		if abs, absErr := AbsCodeDir(localDir); absErr == nil {
			parsed.CodePath = abs
			if r != nil {
				r.AddToTimeline("[ssa_discovery]", fmt.Sprintf(
					"SSH remote code cache hit: remote=%s -> local=%s (skip re-download)",
					parsed.RemoteCodePath, abs,
				))
			}
			return parsed, nil
		}
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return nil, utils.Wrapf(err, "mkdir remote code cache")
	}
	if r != nil {
		r.AddToTimeline("[ssa_discovery]", fmt.Sprintf(
			"SSH fetch start: host=%s user=%s remote=%s -> %s",
			parsed.SSHHost, parsed.SSHUsername, parsed.RemoteCodePath, localDir,
		))
	}
	log.Infof("ssa_api_discovery: ssh fetch host=%s remote=%s local=%s", parsed.SSHHost, parsed.RemoteCodePath, localDir)

	client, err := sshDial(parsed)
	if err != nil {
		return nil, utils.Wrapf(err, "ssh dial")
	}
	defer func() { _ = client.Close() }()

	remoteRoot := filepath.ToSlash(strings.TrimSpace(parsed.RemoteCodePath))
	if !strings.HasPrefix(remoteRoot, "/") {
		return nil, utils.Errorf("remote_code_path must be absolute on remote host, got %q", remoteRoot)
	}
	if err := verifyRemoteDirectory(client, remoteRoot); err != nil {
		return nil, err
	}
	if err := copyRemoteDirectorySFTP(client, remoteRoot, localDir); err != nil {
		return nil, utils.Wrapf(err, "sftp copy %s", remoteRoot)
	}
	abs, err := AbsCodeDir(localDir)
	if err != nil {
		return nil, err
	}
	parsed.CodePath = abs
	if r != nil {
		r.AddToTimeline("[ssa_discovery]", fmt.Sprintf("SSH fetch done: local Code path=%s", abs))
	}
	return parsed, nil
}

func verifyRemoteDirectory(client *utils.SSHClient, remotePath string) error {
	_, err := client.Cmd(fmt.Sprintf("test -d %s", shellQuote(remotePath))).Output()
	if err != nil {
		return utils.Errorf("remote path is not a directory or not accessible: %s (%v)", remotePath, err)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func hasAnyFile(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if !d.IsDir() {
			found = true
		}
		return nil
	})
	return found
}

func copyRemoteDirectorySFTP(client *utils.SSHClient, remoteRoot, localRoot string) error {
	sftpClient, err := client.NewSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()
	return walkCopyRemoteDir(sftpClient, remoteRoot, localRoot)
}

func walkCopyRemoteDir(sftpClient *sftp.Client, remoteRoot, localRoot string) error {
	remoteRoot = path.Clean(filepath.ToSlash(remoteRoot))
	return walkCopyRemoteDirAt(sftpClient, remoteRoot, localRoot, remoteRoot)
}

func walkCopyRemoteDirAt(sftpClient *sftp.Client, remoteRoot, localRoot, remotePath string) error {
	entries, err := sftpClient.ReadDir(remotePath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(remoteRoot, remotePath)
	if err != nil {
		return err
	}
	localPath := localRoot
	if rel != "." {
		localPath = filepath.Join(localRoot, filepath.FromSlash(rel))
	}
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return err
	}
	for _, ent := range entries {
		name := ent.Name()
		if name == "." || name == ".." {
			continue
		}
		remoteChild := path.Join(remotePath, name)
		if ent.IsDir() {
			if err := walkCopyRemoteDirAt(sftpClient, remoteRoot, localRoot, remoteChild); err != nil {
				return err
			}
			continue
		}
		if ent.Mode()&os.ModeSymlink != 0 {
			continue
		}
		relChild, err := filepath.Rel(remoteRoot, remoteChild)
		if err != nil {
			return err
		}
		dst := filepath.Join(localRoot, filepath.FromSlash(relChild))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyRemoteFileSFTP(sftpClient, remoteChild, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyRemoteFileSFTP(sftpClient *sftp.Client, remotePath, localPath string) error {
	src, err := sftpClient.Open(remotePath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Sync()
}
