package bruteutils

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/crypto/ssh"
)

func handleSSHError(result *BruteItemResult, target string, err error) {
	switch true {
	// 			case m, err := regexp.MatchString(`ssh: handshake failed.*?connection reset by peer`, err.Error()); m
	case strings.Contains(err.Error(), "connection reset by peer"):
		utils.Debug(func() {
			log.Errorf("%v's connection is closed by peer", target)
		})
		fallthrough
	case strings.Contains(err.Error(), "connect: connection refused"):
		utils.Debug(func() {
			log.Errorf("%v's connection is refused", target)
		})
		result.Finished = true
	case strings.Contains(err.Error(), "too many colons in address"):
		result.Finished = true
	// case strings.Contains(err.Error(), "attempted methods [none], no supported"):
	// 	result.Finished = true
	default:
		log.Warnf("dial ssh://%s failed: %s", target, err)
	}
}

var sshAuth = &DefaultServiceAuthInfo{
	ServiceName:  "ssh",
	DefaultPorts: "22",
	DefaultUsernames: []string{
		"root", "admin", "ruijie",
	},
	DefaultPasswords: []string{
		"root", "admin123", "root@123", "123456", "admin", "admin@123", "Admin@huawei.com",
		"Changeme_@123", "huawei@123", "h3c@123", "admin@123456", "ruijie", "ruijie@123", "",
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		log.Infof("ssh client start to handle: %s", i.String())
		defer log.Infof("ssh finished to handle: %s", i.String())

		result := i.Result()

		var target = fixToTarget(i.Target, 22)

		config := &ssh.ClientConfig{
			User:            i.Username,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		}
		config.Auth = []ssh.AuthMethod{ssh.Password(i.Password)}

		client, err := ssh.Dial("tcp", target, config)
		if err != nil {
			// 107.187.110.241/24
			log.Errorf("ssh: %v conn failed: %s", i.Target, err)
			handleSSHError(result, target, err)
			return result
		}
		defer client.Close()

		// 创建SSH会话
		session, err := client.NewSession()
		if err != nil {
			log.Errorf("ssh: %v create session failed: %s", i.Target, err)
			handleSSHError(result, target, err)
			return result
		}
		defer session.Close()

		var stdoutBuf bytes.Buffer
		session.Stdout = &stdoutBuf
		verifyRandomString := utils.RandStringBytes(10)
		err = session.Run(fmt.Sprintf("echo %s", verifyRandomString))
		if err != nil {
			log.Errorf("ssh: %v run command failed: %s", i.Target, err)
			handleSSHError(result, target, err)
			return result
		}
		if strings.Contains(stdoutBuf.String(), verifyRandomString) {
			result.Ok = true
		}

		return result
	},
}
