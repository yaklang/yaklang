package guard

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"io/ioutil"
	"os/exec"
	"time"

	"strconv"
	"strings"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib"
)

type NginxDetail struct {
	Process            *PsProcess
	MainPid            int    `json:"main_pid"`
	Prefix             string `json:"prefix"`
	ConfigPath         string `json:"config_path"`
	Version            string `json:"version"`
	OpensslVersionFull string `json:"openssl_version_full"`
	OpensslVersion     string `json:"openssl_version"`
	isServing          bool
	Timestamp          int64 `json:"timestamp"`
}

func (n *NginxDetail) IsServing() bool {
	return n.isServing
}

func (n *NginxDetail) CalcHash() string {
	return utils.CalcSha1(n.MainPid, n.ConfigPath, n.Version, n.Process.Command)
}

func getNginxPid(c context.Context) []int {

	for index := range make([]int, 5) {
		switch index {
		case 1:
			// 方法一，用 nginx.pid 判断
			var pid int64
			raw, err := ioutil.ReadFile("/var/run/nginx.pid")
			if err != nil {
				log.Errorf("not found /var/run/nginx.pid,  maybe no existed nginx process")
				continue
			}
			pid, err = strconv.ParseInt(string(raw), 10, 64)
			if err != nil {
				continue
			}
			return []int{int(pid)}
		case 0:
			// 方法二，用 ps 来判断
			raw, err := searchNginxProcess(c)
			if err != nil {
				log.Errorf("search nginx process failed: %s", err)
				continue
			}
			return raw
		}
	}
	return nil
}

func searchNginxProcess(c context.Context) ([]int, error) {
	cmd := exec.CommandContext(
		c,
		"sh", "-c", `ps aux | grep "nginx: master" | awk '{print $2}'`,
	)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, utils.Errorf("grep nginx master failed: %s", err)
	}

	var psPid int
	if cmd.ProcessState != nil {
		psPid = cmd.ProcessState.Pid()
	}

	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	var pids []int
	for scanner.Scan() {
		pid, err := strconv.ParseInt(scanner.Text(), 10, 64)
		if err != nil {
			continue
		}

		if int(pid) == psPid {
			continue
		}

		pids = append(pids, int(pid))
	}
	return pids, nil
}

func getNginxProcess(c context.Context) []*PsProcess {
	var procs []*PsProcess
	for _, pid := range getNginxPid(c) {
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

func GetNginxDetail(c context.Context) []*NginxDetail {
	var details []*NginxDetail
	for _, proc := range getNginxProcess(c) {
		cmdLine := proc.Command
		res := yaklib.GrokWithMultiPattern(
			cmdLine,
			"nginx: master process *%{NGINXPATH:nginx_path}.*(-c %{PATH:config})?.*",
			map[string]string{
				"NGINXPATH": `[\w._/%-]+`,
			},
		)
		if res == nil {
			log.Errorf("extract main path failed: %s", proc.Command)
			continue
		}

		detail := &NginxDetail{
			Process:   proc,
			MainPid:   proc.Pid,
			Timestamp: time.Now().Unix(),
		}
		raw, err := exec.CommandContext(
			c, res.GetOr("nginx_path", "nginx"), "-V",
		).CombinedOutput()
		if err != nil {
			log.Errorf("found nginx version failed: %s", err)
			continue
		}

		s := bufio.NewScanner(bytes.NewBuffer(raw))
		s.Split(bufio.ScanLines)
		for s.Scan() {
			l := strings.ToLower(s.Text())
			if strings.HasPrefix(l, "nginx version") {
				detail.Version = yaklib.Grok(s.Text(), "nginx version: nginx/%{COMMONVERSION:version}").Get("version")
			} else if strings.HasPrefix(l, "built with openssl") {
				r := yaklib.GrokWithMultiPattern(
					s.Text(), `built with OpenSSL %{OPENSSLVERSION:version}`,
					map[string]string{
						"OPENSSLVERSION": `%{COMMONVERSION}.*`,
					},
				)
				detail.OpensslVersion = r.Get("COMMONVERSION")
				detail.OpensslVersionFull = r.Get("version")
			} else if strings.HasPrefix(l, "configure arguments") {
				detail.Prefix = yaklib.Grok(
					s.Text(), `--prefix=%{PATH:prefix}`,
				).Get("prefix")
				detail.ConfigPath = yaklib.Grok(
					s.Text(), `--conf-path=%{PATH:data}`,
				).Get("data")
			}
		}

		detail.ConfigPath = res.GetOr("config", detail.ConfigPath)
		details = append(details, detail)
	}
	return details
}

type NginxGuardCallback func(NginxDetail)

type NginxGuardTarget struct {
	guardTargetBase

	callbacks []NginxGuardCallback
	cache     *ttlcache.Cache
}

func (n *NginxGuardTarget) do() {
	if n.callbacks == nil {
		return
	}

	details := GetNginxDetail(utils.TimeoutContext(time.Duration(n.intervalSeconds) * time.Second))

	for _, d := range details {
		n.cache.SetWithTTL(d.CalcHash(), d, time.Duration(2*n.intervalSeconds)*time.Second)
	}
}

func NewNginxGuardTarget(
	interval int, cbs ...NginxGuardCallback,
) *NginxGuardTarget {
	t := &NginxGuardTarget{
		guardTargetBase: guardTargetBase{
			intervalSeconds: interval,
		},
		callbacks: cbs,
		cache:     ttlcache.NewCache(),
	}
	t.guardTargetBase.children = t
	t.cache.SetNewItemCallback(func(key string, value interface{}) {
		d, ok := value.(*NginxDetail)
		if ok {
			for _, c := range cbs {
				res := *d
				res.isServing = true
				c(res)
			}
		}
	})
	t.cache.SetExpirationCallback(func(key string, value interface{}) {
		d, ok := value.(*NginxDetail)
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
