package lowhttp

import (
	"fmt"
	"os"
	"yaklang/common/log"
	"yaklang/common/utils"
	"runtime"
	"strings"
)

func GetHostsFilePath() string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`%s\System32\Drivers\etc\hosts`, os.Getenv("SystemRoot"))
	}
	return "/etc/hosts"
}

func GetSystemEtcHosts() map[string]string {
	p := GetHostsFilePath()
	results := make(map[string]string)
	if utils.GetFirstExistedFile(p) == "" {
		log.Errorf("hosts file %s doesn't exist", p)
		return results
	}

	hostsFileCh := utils.ParseStringToLines(p)

	items := make(map[string]string)
	for _, line := range hostsFileCh {
		line = strings.TrimSpace(line)
		// skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// discard comment part
		if strings.Contains(line, "#") {
			line = utils.StringBefore(line, "#")
		}
		tokens := strings.Fields(line)
		if len(tokens) > 1 {
			ip := tokens[0]
			for _, hostname := range tokens[1:] {
				items[hostname] = ip
			}
		}
	}
	return items
}
