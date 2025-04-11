package utils

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
)

func IsUDPPortAvailable(p int) bool {
	return IsPortAvailableWithUDP("127.0.0.1", p)
}

func IsTCPPortAvailable(p int) bool {
	return IsPortAvailable("127.0.0.1", p)
}

func GetRandomAvailableTCPPort() int {
RESET:
	lis, err := net.Listen("tcp", ":0")
	if err == nil {
		port := lis.Addr().(*net.TCPAddr).Port
		_ = lis.Close()
		return port
	} else {
		// fallback
		randPort := 55000 + rand.Intn(10000)
		if !IsTCPPortOpen("127.0.0.1", randPort) && IsTCPPortAvailable(randPort) {
			return randPort
		} else {
			goto RESET
		}
	}
}

func GetRangeAvailableTCPPort(startPort, endPort, maxRetries int) (int, error) {
	if startPort > endPort {
		return 0, Errorf("start port must be less than end port")
	}
	if endPort > 65535 {
		endPort = 65535
	}
	src := rand.NewSource(time.Now().UnixNano())
	r := rand.New(src)

	for i := 0; i < maxRetries; i++ {
		randPort := startPort + r.Intn(endPort-startPort+1)
		if !IsTCPPortOpen("127.0.0.1", randPort) && IsTCPPortAvailable(randPort) {
			return randPort, nil
		}
	}

	return 0, Errorf("unable to find an available port after %d retries", maxRetries)
}

func GetRandomAvailableUDPPort() int {
RESET:
	randPort := 55000 + rand.Intn(10000)
	if IsUDPPortAvailable(randPort) {
		return randPort
	} else {
		goto RESET
	}
}

func IsUDPPortAvailableWithLoopback(p int) bool {
	return IsPortAvailableWithUDP("127.0.0.1", p)
}

func IsTCPPortAvailableWithLoopback(p int) bool {
	return IsPortAvailable("127.0.0.1", p)
}

func IsPortAvailable(host string, p int) bool {
	lis, err := net.Listen("tcp", HostPort(host, p))
	if err != nil {
		return false
	}
	_ = lis.Close()
	return true
}

func IsTCPPortOpen(host string, p int) bool {
	dialer := net.Dialer{}
	dialer.Timeout = 10 * time.Second
	conn, err := dialer.Dial("tcp", HostPort(host, p))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func IsPortAvailableWithUDP(host string, p int) bool {
	addr := fmt.Sprintf("%s:%v", host, p)
	lis, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Errorf("%s is unavailable: %s", addr, err)
		return false
	}
	defer func() {
		_ = lis.Close()
	}()
	return true
}

func GetRandomLocalAddr() string {
	return HostPort("127.0.0.1", GetRandomAvailableTCPPort())
	// return HostPort("127.0.0.1", 161)
}

func GetSystemNameServerList() ([]string, error) {
	client, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil, errors.Errorf("get system nameserver list failed: %s", err)
	}
	return client.Servers, nil
}

func GetHomeDir() (string, error) {
	h, _ := os.UserHomeDir()
	if h != "" {
		return h, nil
	}

	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		usr, err := user.Current()
		if err != nil {
			return "", errors.Errorf("get os use failed: %s", err)
		} else {
			homeDir = usr.HomeDir
		}
	}
	return homeDir, nil
}

func GetHomeDirDefault(d string) string {
	home, err := GetHomeDir()
	if err != nil {
		return d
	}
	return home
}

func InDebugMode() bool {
	return os.Getenv("DEBUG") != "" || os.Getenv("PALMDEBUG") != "" || os.Getenv("YAKLANGDEBUG") != ""
}

func InGithubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") != ""
}

func InTestcase() bool {
	if len(os.Args) > 0 {
		if strings.HasSuffix(strings.ToLower(os.Args[1]), ".test") {
			return true
		}
	}
	for _, v := range os.Args {
		if strings.Contains(v, "-test.v") {
			return true
		}
		if strings.Contains(v, "-test.run") {
			return true
		}
	}
	return false
}

func Debug(f func()) {
	if InDebugMode() {
		f()
	}
}

func EnableDebug() {
	os.Setenv("YAKLANGDEBUG", "1")
}
