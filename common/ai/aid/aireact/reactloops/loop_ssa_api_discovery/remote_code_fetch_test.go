package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUserInput_SSHRemoteSourceLines(t *testing.T) {
	in := `Target: http://127.0.0.1:8080
ssh: root@192.168.1.10:22
ssh_password: secret
remote_code_path: /opt/publiccms
admin_auth: admin1/pass1
`
	p, err := ParseUserInputLenient(in)
	require.NoError(t, err)
	require.True(t, SSHRemoteSourceConfigured(p))
	require.Equal(t, "192.168.1.10:22", p.SSHHost)
	require.Equal(t, "root", p.SSHUsername)
	require.Equal(t, "secret", p.SSHPassword)
	require.Equal(t, "/opt/publiccms", p.RemoteCodePath)
	require.Empty(t, p.CodePath)
}

func TestParseUserInput_SSHRemoteURL(t *testing.T) {
	in := `target: http://127.0.0.1:8080
remote_source: ssh://deploy:pass@10.0.0.5/var/www/app
`
	p, err := ParseUserInputLenient(in)
	require.NoError(t, err)
	require.True(t, SSHRemoteSourceConfigured(p))
	require.Equal(t, "10.0.0.5", p.SSHHost)
	require.Equal(t, "deploy", p.SSHUsername)
	require.Equal(t, "pass", p.SSHPassword)
	require.Equal(t, "/var/www/app", p.RemoteCodePath)
}

func TestParseUserInput_SSHCombinedPath(t *testing.T) {
	in := `target: http://127.0.0.1:8080
ssh: ops@10.0.0.8/opt/project
ssh_password: x
`
	p, err := ParseUserInputLenient(in)
	require.NoError(t, err)
	require.Equal(t, "10.0.0.8", p.SSHHost)
	require.Equal(t, "ops", p.SSHUsername)
	require.Equal(t, "/opt/project", p.RemoteCodePath)
}

func TestParseUserInput_SSHKeyAuth(t *testing.T) {
	in := `target: http://127.0.0.1:8080
ssh_host: 10.0.0.8
ssh_user: root
ssh_key: ~/.ssh/id_rsa
remote_code_path: /data/src
`
	p, err := ParseUserInputLenient(in)
	require.NoError(t, err)
	require.True(t, SSHRemoteSourceConfigured(p))
	require.Equal(t, "/data/src", p.RemoteCodePath)
	require.Equal(t, "~/.ssh/id_rsa", p.SSHPrivateKey)
}

func TestLocalRemoteCodeCacheDir(t *testing.T) {
	p := &ParsedUserInput{
		SSHHost:        "192.168.1.4:22",
		RemoteCodePath: "/opt/publiccms",
	}
	dir, err := localRemoteCodeCacheDir("/tmp/work", p)
	require.NoError(t, err)
	require.Contains(t, dir, "remote_code")
	require.Contains(t, dir, "publiccms")
}

func TestResolveRemoteCodePath_SkipsWhenNotConfigured(t *testing.T) {
	p := &ParsedUserInput{CodePath: "/tmp/local"}
	out, err := ResolveRemoteCodePath(nil, nil, "/tmp/work", p)
	require.NoError(t, err)
	require.Equal(t, "/tmp/local", out.CodePath)
}
