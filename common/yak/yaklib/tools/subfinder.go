package tools

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/subdomain"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

func contentToTmpFile(raw []byte) (string, error) {
	f, err := consts.TempFile("palm-*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = f.Write(raw)
	if err != nil {
		return "", err
	}

	return f.Name(), err
}

func contentToTmpFileStr(raw string) (string, error) {
	return contentToTmpFile([]byte(raw))
}

type SubFinderInstance struct {
	binary  string
	timeout time.Duration
}

type subFinderResult struct {
	Host   string `json:"host"`
	Source string `json:"source"`
	IP     string `json:"ip"`
}

func (s *SubFinderInstance) SetTimeoutRaw(t time.Duration) {
	s.timeout = t
}

func (s *SubFinderInstance) SetTimeout(ts string) {
	d, err := time.ParseDuration(ts)
	if err != nil {
		log.Error(err)
		s.timeout = 5 * time.Minute
		return
	}
	s.timeout = d
	return
}

func (s *SubFinderInstance) Exec(domain string, nsServers ...string) ([]*subdomain.SubdomainResult, error) {
	var fResult []*subdomain.SubdomainResult

	rL, err := contentToTmpFileStr(strings.Join(nsServers, "\n"))
	if err != nil {
		return nil, err
	}

	f, err := consts.TempFile("subfinder-result-*.json")
	if err != nil {
		return nil, err
	}
	_ = f.Close()

	options := []string{
		"-d", domain,
		"-rL", rL,
		"-all",
		"-nW", "-v", "-oI", "-json",
		"-o", f.Name(),
	}

	ctx := utils.TimeoutContext(s.timeout)
	cmd := exec.CommandContext(ctx, s.binary, options...)

	if utils.InDebugMode() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	err = cmd.Run()
	if err != nil {
		log.Errorf("subfinder run %#v failed: %s", options, err)
		return nil, err
	}

	results, err := ioutil.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}

	lineScanner := bufio.NewScanner(bytes.NewBuffer(results))
	lineScanner.Split(bufio.ScanLines)
	for lineScanner.Scan() {
		var r = subFinderResult{}
		err := json.Unmarshal(lineScanner.Bytes(), &r)
		if err != nil {
			continue
		}
		result := subdomain.SubdomainResult{
			FromTarget: domain,
			Domain:     r.Host,
			IP:         r.IP, FromModeRaw: subdomain.SEARCH,
			Tags: []string{r.Source},
		}
		fResult = append(fResult, &result)
	}

	return fResult, nil
}

func NewSubFinderInstance() (*SubFinderInstance, error) {
	ins := &SubFinderInstance{}

	home, err := utils.GetHomeDir()
	if err != nil {
		return nil, err
	}

	ins.binary, err = utils.GetFirstExistedPathE(
		"./subfinder",
		path.Join(home, "subfinder"),
		path.Join("/usr/local/bin", "subfinder"),
		path.Join("/usr/bin/", "subfinder"),
	)
	if err != nil {
		return nil, err
	}

	return ins, nil
}
