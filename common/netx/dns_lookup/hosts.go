package dns_lookup

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	hosts = new(sync.Map)
)

func init() {
	err := LoadHostsFromFile(GetDefaultHostsFilePath())
	if err != nil {
		log.Infof("load hosts from file failed: %v", err)
	}
}

func AddHost(host string, ip string) {
	log.Debugf("load hostfile: %24s -> %v", host, ip)
	hosts.Store(host, ip)
}

func DeleteHost(host string) {
	log.Debugf("delete hostfile: %24s", host)
	hosts.Delete(host)
}

func GetHost(host string) (ip string, ok bool) {
	ipRaw, ok := hosts.Load(host)
	if ok {
		return ipRaw.(string), true
	}
	return "", false
}

func IsExistedHost(host string) bool {
	_, ok := hosts.Load(host)
	return ok
}

func GetDefaultHostsFilePath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("SystemRoot"), `System32\Drivers\etc\hosts`)
	}
	return "/etc/hosts"
}

// LoadHostsFromFile loads hosts file
func LoadHostsFromFile(p string) error {
	p = utils.GetFirstExistedFile(p)
	if p == "" {
		return utils.Error("hosts file doesn't exist")
	}
	hostsContents, err := os.ReadFile(p)
	if err != nil {
		return utils.Errorf("cannot read hosts: %v reason: %v", p, err)
	}
	hostsFileCh := utils.ParseStringToLines(string(hostsContents))
	for _, line := range hostsFileCh {
		line = strings.TrimSpace(line)
		// skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// discard comment part
		before, _, _ := strings.Cut(line, "#")
		before = strings.TrimSpace(before)
		if before == "" {
			continue
		}
		tokens := strings.Fields(before)
		if len(tokens) > 1 {
			ip := tokens[0]
			for _, hostname := range tokens[1:] {
				AddHost(hostname, ip)
			}
		}
	}
	return nil
}
