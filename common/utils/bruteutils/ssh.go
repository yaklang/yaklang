package bruteutils

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/crypto/ssh"
	"net"
	"strconv"
	"strings"
	"time"
)

var DefaultDailer = &net.Dialer{Timeout: 5 * time.Second}

var sshAuth = &DefaultServiceAuthInfo{
	ServiceName:  "ssh",
	DefaultPorts: "22",
	DefaultUsernames: []string{
		"root", "admin", "ruijie",
	},
	DefaultPasswords: []string{
		"root", "admin123", "root@123", "123456", "admin", "admin@123", "Admin@huawei.com",
		"Changeme_@123", "huawei@123", "h3c@123", "admin@123456", "ruijie", "ruijie@123",
	},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 22)

		conn, err := DefaultDailer.Dial("tcp", i.Target)
		if err != nil {
			log.Errorf("ssh:\\\\%v conn failed: %s", i.Target, err)
			res := i.Result()
			res.Finished = true
			return res
		}
		defer conn.Close()

		raw, _ := utils.ReadConnWithTimeout(conn, 2*time.Second)
		if raw == nil {
			res := i.Result()
			res.Finished = true
			return res
		}
		println(strconv.Quote(string(raw)))
		return i.Result()
	},
	BrutePass: func(item *BruteItem) *BruteItemResult {
		log.Infof("ssh client start to handle: %s", item.String())
		defer log.Infof("ssh finished to handle: %s", item.String())

		result := item.Result()

		var target = fixToTarget(item.Target, 22)

		config := &ssh.ClientConfig{
			User:            item.Username,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		}
		config.Auth = []ssh.AuthMethod{ssh.Password(item.Password)}

		client, err := ssh.Dial("tcp", target, config)
		if err != nil {
			// 107.187.110.241/24
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
				return result
			case strings.Contains(err.Error(), "too many colons in address"):
				result.Finished = true
				return result
			case strings.Contains(err.Error(), "attempted methods [none], no supported"):
				result.Finished = true
				return result
			default:
				log.Warnf("dial ssh://%s failed: %s", target, err)
				return result
			}
		}
		defer client.Close()

		//session, err := client.NewSession()
		//if err != nil {
		//	return result
		//}
		//
		//err = session.Run("echo 123123123")
		//if err != nil {
		//	return result
		//}

		result.Ok = true
		return result
	},
}
