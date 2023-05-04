package tools

import (
	"context"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/hybridscan"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/utils/pcapfix"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"net"
	"os"
	"sync"
	"time"
)

type _yakPortScanConfig struct {
	//rulePath              string
	//onlyUserRule          bool
	//requestTimeout        time.Duration
	//enableFingerprint     bool
	outputFile       string
	outputFilePrefix string
	//fingerprintResultFile string
	waiting         time.Duration
	initFilterPorts string
	initFilterHosts string

	rateLimitDelayMs  float64
	rateLimitDelayGap int // 每隔多少数据包 delay 一次？

	excludeHosts *hostsparser.HostsParser
	excludePorts *filter.StringFilter

	callback           func(result *synscan.SynScanResult)
	submitTaskCallback func(i string)
}

func (i *_yakPortScanConfig) callCallback(r *synscan.SynScanResult) {
	if i == nil {
		return
	}

	if i.callback == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("synscan callback failed: %s", err)
		}
	}()

	i.callback(r)
}

func (i *_yakPortScanConfig) callSubmitTaskCallback(r string) {
	if i == nil {
		return
	}

	if i.submitTaskCallback == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("synscan callback failed: %s", err)
		}
	}()

	i.submitTaskCallback(r)
}

type scanOpt func(config *_yakPortScanConfig)

func _scanOptRateLimit(ms int, count int) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.rateLimitDelayMs = float64(ms)
		config.rateLimitDelayGap = count
	}
}

// 设置 SYN 扫描的并发可以有效控制精准度
func _scanOptSYNConcurrent(count int) scanOpt {
	return func(config *_yakPortScanConfig) {
		if count <= 0 {
			count = 1000
		}

		config.rateLimitDelayMs = float64(float64((time.Second / time.Duration(count)).Microseconds()) / float64(1e3))
		config.rateLimitDelayGap = 5
		log.Infof("rate limit delay ms: %v(ms)", config.rateLimitDelayMs)
		log.Infof("rate limit delay gap: %v", config.rateLimitDelayGap)
	}
}

// synscan 发出 SYN 包后等待多久？
func _scanOptWaiting(sec float64) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.waiting = utils.FloatSecondDuration(sec)
		if config.waiting <= 0 {
			config.waiting = 5 * time.Second
		}
	}
}

func _scanOptExcludePorts(ports string) scanOpt {
	return func(config *_yakPortScanConfig) {
		if ports == "" {
			return
		}
		config.excludePorts = filter.NewFilter()
		for _, port := range utils.ParseStringToPorts(ports) {
			config.excludePorts.Insert(fmt.Sprint(port))
		}
	}
}

func (c *_yakPortScanConfig) IsFiltered(host string, port int) bool {
	if c == nil {
		return false
	}

	if c.excludeHosts != nil && host != "" {
		if c.excludeHosts.Contains(host) {
			return true
		}
	}

	if c.excludePorts != nil && port > 0 {
		if c.excludePorts.Exist(fmt.Sprint(port)) {
			return true
		}
	}

	return false
}

func _scanOptExcludeHosts(hosts string) scanOpt {
	return func(config *_yakPortScanConfig) {
		if hosts == "" {
			return
		}
		config.excludeHosts = hostsparser.NewHostsParser(context.Background(), hosts)
	}
}

// 端口开放的结果保存到文件
func _scanOptOpenPortResult(file string) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.outputFile = file
	}
}

// 端口开放结果保存文件加个前缀，比如 tcp:// https:// http:// 等
func _scanOptOpenPortResultPrefix(prefix string) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.outputFilePrefix = prefix
	}
}

func _scanOptOpenPortInitHostFilter(f string) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.initFilterHosts = f
	}
}

func _scanOptOpenPortInitPortFilter(f string) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.initFilterPorts = f
	}
}

// 指纹结果保存文件
//func _scanOptFpResult(file string) scanOpt {
//	return func(config *_yakPortScanConfig) {
//		config.fingerprintResultFile = file
//	}
//}

// 启动指纹扫描
//func _scanOptEnableFpScan() scanOpt {
//	return func(config *_yakPortScanConfig) {
//		config.enableFingerprint = true
//	}
//}

// 指纹扫描-探测请求超时
//func _scanOptFingerprintRequestTimeout(i float64) scanOpt {
//	return func(config *_yakPortScanConfig) {
//		config.requestTimeout = utils.FloatSecondDuration(i)
//		if config.requestTimeout <= 0 {
//			config.requestTimeout = 5 * time.Second
//		}
//	}
//}

func _synScanDo(targetChan chan string, ports string, config *_yakPortScanConfig) (chan *synscan.SynScanResult, error) {
	if targetChan == nil {
		return nil, utils.Error("empty target")
	}

	filteredTargetChan, targetList := filterTargetChannel(targetChan, config)

	sampleTarget := getSampleTarget(targetList)

	openResult := make(chan *synscan.SynScanResult, 10000)

	errChan := make(chan error)
	go func() {
		defer close(errChan)
		err := runScan(sampleTarget, filteredTargetChan, ports, config, openResult)
		if err != nil {
			errChan <- err
		}

	}()

	select {
	case err := <-errChan:
		return nil, err
	case res, ok := <-openResult:
		if ok {
			openResult <- res
		}
		return openResult, nil
	}
}

func filterTargetChannel(targetChan chan string, config *_yakPortScanConfig) (chan string, []string) {
	result := <-targetChan
	targetList := []string{result}
	newTargetChan := make(chan string, 1)
	newTargetChan <- result

	go func() {
		defer close(newTargetChan)
		for {
			select {
			case result, ok := <-targetChan:
				if !ok {
					return
				}
				if config.IsFiltered(result, 0) {
					continue
				}
				newTargetChan <- result
				targetList = append(targetList, result)
			}
		}
	}()

	return newTargetChan, targetList
}

func getSampleTarget(targetList []string) string {
	var sampleTarget string

	if len(targetList) == 1 {
		sampleTarget = targetList[0]
	} else {
		for _, target := range targetList {
			if !utils.IsLoopback(target) {
				sampleTarget = target
				break
			}
		}
		if sampleTarget == "" {
			sampleTarget = targetList[1]
		}
	}

	return sampleTarget
}

func runScan(sampleTarget string, filteredTargetChan chan string, ports string, config *_yakPortScanConfig, openResult chan *synscan.SynScanResult) error {
	synScanOptions, err := synscan.CreateConfigOptionsByTargetNetworkOrDomain(sampleTarget, 10*time.Second)
	if err != nil {
		return utils.Errorf("init syn scanner failed: %v", err)
	}

	synScanConfig, err := synscan.NewConfig(synScanOptions...)
	if err != nil {
		return fmt.Errorf("create synscan config failed: %w", err)
	}

	scanCenterConfig, err := hybridscan.NewDefaultConfigWithSynScanConfig(synScanConfig)

	if err != nil {
		return fmt.Errorf("default config failed: %w", err)
	}

	// Fingerprint scan switch
	scanCenterConfig.DisableFingerprintMatch = true // !config.enableFingerprint

	log.Info("start create hyper scan center...")
	scanCenter, err := hybridscan.NewHyperScanCenter(context.Background(), scanCenterConfig)
	if err != nil {
		return utils.Errorf("create hyper scan center failed: %s", err)
	}

	defer scanCenter.Close()
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("syn failed: %v", err)
		}
	}()
	defer close(openResult)

	scanCenter.SetSynScanRateLimit(config.rateLimitDelayMs, config.rateLimitDelayGap)

	log.Info("preparing for result collectors")
	var openPortLock = new(sync.Mutex)
	var openPortCount int

	// Output file
	var outputFile *os.File
	if config.outputFile != "" {
		var err error
		outputFile, err = os.OpenFile(config.outputFile, os.O_RDWR|os.O_CREATE, os.ModePerm)
		if err != nil {
			log.Errorf("open file %v failed; %s", config.outputFile, err)
		}
		if outputFile != nil {
			defer outputFile.Close()
		}
	}

	log.Infof("start submit task and scan...")
	uid := uuid.NewV4().String()
	hostsFilter := utils.NewHostsFilter()
	portsFilter := utils.NewPortsFilter(ports)
	stringFilter := filter.NewFilter()

	hostsFilter.Add(config.initFilterHosts)
	portsFilter.Add(config.initFilterPorts)

	err = scanCenter.RegisterSynScanOpenPortHandler(uid, func(ip net.IP, port int) {
		openPortLock.Lock()
		defer openPortLock.Unlock()

		defer func() {
			if err := recover(); err != nil {
				log.Warnf("panic for handling syn result: %v", err)
				return
			}
		}()

		addr := utils.HostPort(ip.String(), port)
		if stringFilter.Exist(addr) {
			return
		}
		stringFilter.Insert(addr)

		if !hostsFilter.Contains(addr) {
			// 端口不在范围内
			if !portsFilter.Contains(port) {
				return
			}
			if !hostsFilter.Contains(ip.String()) {
				return
			}
		}

		openPortCount++
		log.Debugf("found open port -> tcp://%v", addr)

		result := &synscan.SynScanResult{
			Host: ip.String(),
			Port: port,
		}
		config.callCallback(result)

		select {
		case openResult <- result:
		}

		if outputFile != nil {
			outputFile.Write(
				[]byte(fmt.Sprintf(
					"%s%v\n",
					config.outputFilePrefix,
					addr,
				)),
			)
		}
	})
	if err != nil {
		return fmt.Errorf("register synscan result handler failed: %w", err)
	}
	defer scanCenter.UnregisterSynScanOpenPortHandler(uid)
	scanCenter.GetSYNScanner().OnSubmitTask(func(addr string, port int) {
		config.callSubmitTaskCallback(utils.HostPort(addr, port))
	})

	portInts := getFilteredPorts(ports, config)

	ports = utils.ConcatPorts(portInts)
	for target := range filteredTargetChan {
		if config.IsFiltered(target, 0) {
			continue
		}
		log.Debugf("start to submit synscan for %s ports: %v", target, ports)
		hostsFilter.Add(target)
		if !utils.IsIPv4(target) {
			hostsFilter.Add(utils.GetIPsFromHostWithTimeout(5*time.Second, target, nil)...)
		}

		hostRaw, portRaw, _ := utils.ParseStringToHostPort(target)
		if portRaw > 0 {
			portsFilter.Add(fmt.Sprint(portRaw))
			hostsFilter.Add(hostRaw)
			if !utils.IsIPv4(target) {
				hostsFilter.Add(utils.GetIPsFromHostWithTimeout(5*time.Second, target, nil)...)
			}
			_ = scanCenter.SubmitOpenPortScanTask(hostRaw, fmt.Sprint(portRaw), true, true)
		}
		err = scanCenter.SubmitOpenPortScanTask(target, ports, true, true)
		if err != nil {
			return fmt.Errorf("submit synscan failed: %w", err)
		}
	}
	scanCenter.WaitWriteChannelEmpty()
	log.Infof("finished submitting.")

	log.Infof("waiting last packet (SYN) for %v seconds", config.waiting)
	select {
	case <-time.After(config.waiting):
	}

	log.Infof("total %v open port(s) found", openPortCount)

	return nil
}

func getFilteredPorts(ports string, config *_yakPortScanConfig) []int {
	var filteredPorts []int

	for _, p := range utils.ParseStringToPorts(ports) {
		if config.IsFiltered("", p) {
			continue
		}
		filteredPorts = append(filteredPorts, p)
	}

	return filteredPorts
}

func hostsToChan(hosts string) chan string {
	c := make(chan string)
	go func() {
		defer close(c)
		for _, h := range utils.ParseStringToHosts(hosts) {
			c <- h
		}
	}()
	return c
}

func pingutilsToChan(res chan *pingutil.PingResult) chan string {
	c := make(chan string)
	go func() {
		defer close(c)
		for result := range res {
			if result.Ok {
				log.Infof("ping to synscan for target: %s", result.IP)
				c <- result.IP
			}
		}
	}()
	return c
}

func _synscanFromPingUtils(res chan *pingutil.PingResult, ports string, opts ...scanOpt) (chan *synscan.SynScanResult, error) {
	config := &_yakPortScanConfig{
		//requestTimeout: 5 * time.Second,
		waiting:           5 * time.Second,
		rateLimitDelayMs:  1,
		rateLimitDelayGap: 5,
	}
	for _, opt := range opts {
		opt(config)
	}

	return _synScanDo(pingutilsToChan(res), ports, config)
}

func _scanOptCallback(i func(i *synscan.SynScanResult)) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.callback = i
	}
}

func _scanOptSubmitTaskCallback(i func(string)) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.submitTaskCallback = i
	}
}

// 为了防止网卡过载，5个是上限
//  1. waiting 实现
//  2. timeout
var SynPortScanExports = map[string]interface{}{
	"FixPermission": pcapfix.Fix,
	"Scan": func(target string, port string, opts ...scanOpt) (chan *synscan.SynScanResult, error) {
		config := &_yakPortScanConfig{
			waiting:           5 * time.Second,
			rateLimitDelayMs:  1,
			rateLimitDelayGap: 5,
		}
		for _, opt := range opts {
			opt(config)
		}
		return _synScanDo(hostsToChan(target), port, config)
	},
	"ScanFromPing": _synscanFromPingUtils,

	"callback":           _scanOptCallback,
	"submitTaskCallback": _scanOptSubmitTaskCallback,
	"excludePorts":       _scanOptExcludePorts,
	"excludeHosts":       _scanOptExcludeHosts,
	"wait":               _scanOptWaiting,
	"outputFile":         _scanOptOpenPortResult,
	"outputPrefix":       _scanOptOpenPortResultPrefix,
	"initHostFilter":     _scanOptOpenPortInitHostFilter,
	"initPortFilter":     _scanOptOpenPortInitPortFilter,
	"rateLimit":          _scanOptRateLimit,
	"concurrent":         _scanOptSYNConcurrent,
	//"fpOutputFile":       _scanOptFpResult,
	//"fingerprint":        _scanOptEnableFpScan,
	//"fingerprintTimeout": _scanOptFingerprintRequestTimeout,
}
