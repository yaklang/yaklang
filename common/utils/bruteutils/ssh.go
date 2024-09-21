package bruteutils

import (
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

func sshDial(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	conn, err := defaultDialer.DialTCPContext(utils.TimeoutContext(defaultTimeout), network, addr)
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
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
		result := i.Result()

		target := fixToTarget(i.Target, 22)

		config := &ssh.ClientConfig{
			User:            i.Username,
			Auth:            []ssh.AuthMethod{ssh.Password(i.Password)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			BannerCallback: func(message string) error {
				log.Infof("fetch banner: %v from %v", message, i.Target)
				return nil
			},
			HostKeyAlgorithms: []string{"ssh-rsa", "ssh-dss", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ssh-ed25519"},
			Timeout:           10 * time.Second,
		}

		client, err := sshDial("tcp", target, config)
		if err != nil {
			log.Errorf("ssh: %v conn failed: %s try %v:%v", i.Target, err, i.Username, i.Password)
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

		// 在一些路由器中，执行命令是没意义的，知道能进去就行了
		// 之后可以想点办法
		if len(client.SessionID()) > 0 {
			result.Ok = true
		} else {
			log.Warnf("ssh: %v session id is empty", i.Target)
		}
		//var stdoutBuf bytes.Buffer
		//session.Stdout = &stdoutBuf
		//verifyRandomString := utils.RandStringBytes(10)
		//err = session.Run(fmt.Sprintf("echo %s", verifyRandomString))
		//if err != nil {
		//	log.Errorf("ssh: %v run command failed: %s", i.Target, err)
		//	handleSSHError(result, target, err)
		//	return result
		//}
		//if strings.Contains(stdoutBuf.String(), verifyRandomString) {
		//	result.Ok = true
		//}

		return result
	},
}
