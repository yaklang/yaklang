package hidsevent

import (
	"context"
	"github.com/kevinburke/ssh_config"
	"github.com/pkg/errors"
	"os/exec"
	"strings"
	"yaklang.io/yaklang/common/log"

	"os"
)

// 1. 获取 SSH 精确版本信息
// 2. 配置文件, 公钥私钥监控
// 3. 配置文件关键选项:
//    1. 是否允许密码登录
//    2. 是否允许空密码
//    3. 密钥登录

type SSHInfo struct {
	Version                string `json:"version"`
	SSHV2                  bool   `json:"sshv2"`
	PermitEmptyPasswords   bool   `json:"permit_empty_passwords"`
	PasswordAuthentication bool   `json:"password_authentication"`
	HostKey                string `json:"host_key"`
}

func GetSSHVersion(ctx context.Context) (string, error) {
	raw, err := exec.CommandContext(ctx, "ssh", "-V").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.Trim(string(raw), "\n"), err
}

func GetKeyAllValue(c *ssh_config.Config, alias, key string, spiStr string) string {
	retStr := ""
	lowerKey := strings.ToLower(key)
	for _, host := range c.Hosts {
		if !host.Matches(alias) {
			continue
		}
		for _, node := range host.Nodes {
			switch t := node.(type) {
			case *ssh_config.Empty:
				continue
			case *ssh_config.KV:
				// "keys are case insensitive" per the spec
				lkey := strings.ToLower(t.Key)
				if lkey == "match" {
					panic("can't handle Match directives")
				}
				if lkey == lowerKey {
					retStr += t.Value + spiStr
				}
			case *ssh_config.Include:
				val := t.Get(alias, key)
				if val != "" {
					retStr += val + spiStr
				}
			}
		}
	}
	retStr = strings.Trim(retStr, spiStr)
	return retStr
}

func GetSSHConfigValue(filePath string, alias, key string) string {

	f, err := os.Open(filePath)
	if err != nil {
		log.Errorf("read ssh config file=%v err=%v", filePath, err)
		return ssh_config.Get(alias, key)
	}

	cfg, err := ssh_config.Decode(f)

	f.Close()

	if err != nil {
		log.Errorf("ssh config file=%v read error=%v", filePath, err)
		return ssh_config.Get(alias, key)
	}

	resStr := GetKeyAllValue(cfg, alias, key, "|")
	if len(resStr) != 0 {
		return resStr
	}

	return ssh_config.Get(alias, key)
}

func GetSSHInfo(ctx context.Context, filePath string) (*SSHInfo, error) {

	info := &SSHInfo{}
	versioninfo, err := GetSSHVersion(ctx)
	if err != nil {
		return nil, errors.Errorf("get ssh version err=%v", err)
	}
	info.Version = versioninfo
	info.PasswordAuthentication = GetSSHConfigValue(filePath, "", "PasswordAuthentication") == "yes"
	info.PermitEmptyPasswords = GetSSHConfigValue(filePath, "", "PermitEmptyPasswords") == "yes"
	info.HostKey = GetSSHConfigValue(filePath, "", "HostKey")

	return info, nil
}
