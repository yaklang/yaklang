package guard

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/yaklib"
)

type ApacheDetail struct {
	Process *PsProcess
	MainPid int `json:"main_pid"`

	ExecutableFile string `json:"executable_file"`
	Version        string `json:"version"`
	HttpdRoot      string `json:"httpd_root"`
	ConfigPath     string `json:"config_path"`

	// inconfig
	//ServerRoot   string `json:"server_root"`
	//DocumentRoot string `json:"document_root"`
	//ServerName   string `json:"server_name"`
	//ExecCGI      bool   `json:"exec_cgi"`
	//Indexes      bool   `json:"indexes"`
	isServing bool
	Timestamp int64 `json:"timestamp"`
}

func (n *ApacheDetail) IsServing() bool {
	return n.isServing
}

func (n *ApacheDetail) CalcHash() string {
	return utils.CalcSha1(n.MainPid, n.ExecutableFile, n.HttpdRoot, n.ConfigPath, n.Version)
}

func searchApacheProcess(c context.Context) ([]int, error) {
	cmds := []string{
		`ps ax -o pid,ppid,command | grep "apache2" | grep -v "grep"`,
		`ps ax -o pid,ppid,command | grep "httpd" | grep -v "grep"`,
	}

	var pids []int
	for _, cmd := range cmds {
		c := exec.CommandContext(c,
			"sh", "-c", cmd,
		)
		raw, err := c.CombinedOutput()
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(bytes.NewBuffer(raw))
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			np := utils.NewBlockParser(bytes.NewBuffer(scanner.Bytes()))
			pidRaw := np.NextStringBlock()
			ppidRaw := np.NextStringBlock()
			if ppidRaw != "1" {
				continue
			}
			pid, err := strconv.ParseInt(pidRaw, 10, 64)
			if err != nil {
				continue
			}

			pids = append(pids, int(pid))
		}
	}
	return pids, nil
}

func getApachePid(c context.Context) []int {
	for i := range make([]int, 4) {
		switch i {
		case 0:
			// 方法一，用 apache2.pid 判断
			var pids []int
			for _, pidFile := range []string{"/var/run/apache2.pid", "/var/run/httpd.pid"} {
				raw, err := ioutil.ReadFile(pidFile)
				if err != nil {
					continue
				}

				pid, err := strconv.ParseInt(string(raw), 10, 64)
				if err != nil {
					continue
				}
				pids = append(pids, int(pid))
			}

			if len(pids) <= 0 {
				continue
			}

			return pids
		case 1:
			raw, err := searchApacheProcess(c)
			if err != nil {
				log.Errorf(err.Error())
			}
			return raw
		}
	}

	return nil
}

func getApacheProcess(c context.Context) []*PsProcess {
	var procs []*PsProcess
	for _, pid := range getApachePid(c) {
		cmd := exec.CommandContext(
			c,
			"ps", "-p", fmt.Sprint(pid),
			"-o", "user,pid,%cpu,%mem,vsz,rss,tty,stat,start,time,ppid,command",
		)
		ps, err := psCallAndParseWithCmd(cmd)
		if err != nil {
			continue
		}
		procs = append(procs, ps...)
	}
	return procs
}

func GetApacheDetail(c context.Context) []*ApacheDetail {
	var details []*ApacheDetail
	for _, proc := range getApacheProcess(c) {
		binFile := utils.NewBlockParser(bytes.NewBufferString(proc.Command)).NextStringBlock()
		raw, err := exec.CommandContext(c, binFile, "-V").CombinedOutput()
		if err != nil {
			log.Errorf("binFile: %v is not executable", err)
			continue
		}

		detail := &ApacheDetail{
			Process:        proc,
			MainPid:        proc.Pid,
			ExecutableFile: binFile,
			Timestamp:      time.Now().Unix(),
		}
		s := bufio.NewScanner(bytes.NewBuffer(raw))
		s.Split(bufio.ScanLines)

		for s.Scan() {
			line := strings.TrimSpace(strings.ToLower(s.Text()))
			switch true {
			case strings.HasPrefix(line, "server version:"):
				detail.Version = yaklib.Grok(s.Text(), "Server version: Apache/%{COMMONVERSION:version}.*").Get("version")
				fallthrough
			case strings.HasPrefix(line, "-d httpd_root"):
				detail.HttpdRoot = yaklib.Grok(s.Text(), ` *-D HTTPD_ROOT="%{PATH:root}".*`).Get("root")
			case strings.HasPrefix(line, "-d server_config_file"):
				detail.ConfigPath = yaklib.Grok(s.Text(), ` *-D SERVER_CONFIG_FILE="%{PROG:config}"`).Get("config")
			}
		}

		if cConfig := yaklib.Grok(proc.Command, `-f %{PROG:config}`).Get("config"); cConfig != "" {
			detail.ConfigPath = cConfig
		}
		if !filepath.IsAbs(detail.ConfigPath) {
			detail.ConfigPath = filepath.Join(detail.HttpdRoot, detail.ConfigPath)
		}
		details = append(details, detail)
	}
	return details
}

type ApacheGuardCallback func(detail ApacheDetail)

type ApacheGuardTarget struct {
	guardTargetBase

	callbacks []ApacheGuardCallback
	cache     *ttlcache.Cache
}

func (a *ApacheGuardTarget) do() {
	if a.callbacks == nil {
		return
	}

	details := GetApacheDetail(utils.TimeoutContext(time.Duration(a.intervalSeconds) * time.Second))

	for _, d := range details {
		a.cache.SetWithTTL(d.CalcHash(), d, time.Duration(2*a.intervalSeconds)*time.Second)
	}
}

func NewApacheGuardTarget(
	interval int, cbs ...ApacheGuardCallback) *ApacheGuardTarget {
	t := &ApacheGuardTarget{
		guardTargetBase: guardTargetBase{intervalSeconds: interval},
		callbacks:       cbs,
		cache:           ttlcache.NewCache(),
	}
	t.children = t
	t.cache.SetNewItemCallback(func(key string, value interface{}) {
		d, ok := value.(*ApacheDetail)
		if ok {
			for _, c := range cbs {
				res := *d
				res.isServing = true
				c(res)
			}
		}
	})
	t.cache.SetExpirationCallback(func(key string, value interface{}) {
		d, ok := value.(*ApacheDetail)
		if ok {
			for _, c := range cbs {
				res := *d
				res.isServing = false
				c(res)
			}
		}
	})
	return t
}
