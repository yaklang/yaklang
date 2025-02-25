package tools

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/netx"
	"net"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/finscan"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
)

type _yakFinPortScanConfig struct {
	// rulePath              string
	// onlyUserRule          bool
	// requestTimeout        time.Duration
	// enableFingerprint     bool
	outputFile       string
	outputFilePrefix string
	// fingerprintResultFile string
	waiting         time.Duration
	initFilterPorts string
	initFilterHosts string

	rateLimitDelayMs  float64
	rateLimitDelayGap int // 每隔多少数据包 delay 一次？

	excludeHosts *hostsparser.HostsParser
	excludePorts filter.Filterable

	callback           func(result *finscan.FinScanResult)
	submitTaskCallback func(i string)
}
type finScanOpt func(config *_yakFinPortScanConfig)

func _finScanOptRateLimit(ms int, count int) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		config.rateLimitDelayMs = float64(ms)
		config.rateLimitDelayGap = count
	}
}

// 设置 FIN 扫描的并发可以有效控制精准度
func _finScanOptConcurrent(count int) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		if count <= 0 {
			count = 1000
		}

		config.rateLimitDelayMs = float64(float64((time.Second / time.Duration(count)).Microseconds()) / float64(1e3))
		config.rateLimitDelayGap = 5
		log.Infof("rate limit delay ms: %v(ms)", config.rateLimitDelayMs)
		log.Infof("rate limit delay gap: %v", config.rateLimitDelayGap)
	}
}

// finscan 发出 FIN 包后等待多久？
func _finScanOptWaiting(sec float64) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		config.waiting = utils.FloatSecondDuration(sec)
		if config.waiting <= 0 {
			config.waiting = 5 * time.Second
		}
	}
}

func _finScanOptExcludePorts(ports string) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		if ports == "" {
			return
		}
		config.excludePorts = filter.NewFilter()
		for _, port := range utils.ParseStringToPorts(ports) {
			config.excludePorts.Insert(fmt.Sprint(port))
		}
	}
}

func (c *_yakFinPortScanConfig) IsFiltered(host string, port int) bool {
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

func _finScanOptExcludeHosts(hosts string) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		if hosts == "" {
			return
		}
		config.excludeHosts = hostsparser.NewHostsParser(context.Background(), hosts)
	}
}

// 端口开放的结果保存到文件
func _finScanOptOpenPortResult(file string) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		config.outputFile = file
	}
}

// 端口开放结果保存文件加个前缀，比如 tcp:// https:// http:// 等
func _finScanOptOpenPortResultPrefix(prefix string) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		config.outputFilePrefix = prefix
	}
}

func _finScanOptOpenPortInitHostFilter(f string) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		config.initFilterHosts = f
	}
}

func _finScanOptOpenPortInitPortFilter(f string) finScanOpt {
	return func(config *_yakFinPortScanConfig) {
		config.initFilterPorts = f
	}
}

// 指纹结果保存文件
//func _finScanOptFpResult(file string) scanOpt {
//	return func(config *_yakPortScanConfig) {
//		config.fingerprintResultFile = file
//	}
//}

// 启动指纹扫描
//func _finScanOptEnableFpScan() scanOpt {
//	return func(config *_yakPortScanConfig) {
//		config.enableFingerprint = true
//	}
//}

// 指纹扫描-探测请求超时
//
//	func _finScanOptFingerprintRequestTimeout(i float64) scanOpt {
//		return func(config *_yakPortScanConfig) {
//			config.requestTimeout = utils.FloatSecondDuration(i)
//			if config.requestTimeout <= 0 {
//				config.requestTimeout = 5 * time.Second
//			}
//		}
//	}
func _finscanDo(targetChan chan string, ports string, config *_yakFinPortScanConfig) (chan *finscan.FinScanResult, error) {
	if targetChan == nil {
		return nil, utils.Errorf("empty target")
	}
	newTargetChan, sampleTarget := filterTargetChannel(targetChan, config.IsFiltered)

	closeResult := make(chan *finscan.FinScanResult, 10000)

	go func() {
		var stringFilter filter.Filterable

		defer func() {
			close(closeResult)
			config.excludePorts.Close()
			stringFilter.Close()

			if err := recover(); err != nil {
				log.Errorf("fin failed: %v", err)
			}
		}()

		finScanOptions, err := finscan.CreateConfigOptionsByTargetNetworkOrDomain(sampleTarget, 10*time.Second)
		if err != nil {
			log.Errorf("init fin scanner failed: %s", err)
			return
		}
		finScanConfig, err := finscan.NewConfig(finScanOptions...)
		if err != nil {
			log.Errorf("create finscan config failed: %s", err)
			return
		}
		scanner, err := finscan.NewScanner(context.Background(), finScanConfig)
		scanner.SetRateLimit(config.rateLimitDelayMs, config.rateLimitDelayGap)

		if err != nil {
			log.Errorf("create fin scanner failed: %s", err)
			return
		}

		log.Info("preparing for result collectors")
		openPortLock := new(sync.Mutex)
		var closePortCount int

		log.Infof("start submit task and scan...")
		uid := uuid.New().String()
		hostsFilter := utils.NewHostsFilter()
		portsFilter := utils.NewPortsFilter(ports)
		stringFilter = filter.NewFilter()

		hostsFilter.Add(config.initFilterHosts)
		portsFilter.Add(config.initFilterPorts)

		// No rsp = open | filtered
		err = scanner.RegisterRstAckHandler(uid, func(ip net.IP, port int) {
			openPortLock.Lock()
			defer openPortLock.Unlock()

			defer func() {
				if err := recover(); err != nil {
					log.Warnf("panic for handling fin result: %v", err)
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

			closePortCount++
			r := utils.HostPort(ip.String(), port)
			log.Infof("found closed port -> tcp://%v", r)

			result := &finscan.FinScanResult{
				Host:   ip.String(),
				Port:   port,
				Status: finscan.CLOSED_STATE,
			}
			// config.callCallback(result)

			select {
			case closeResult <- result:
			}
		})
		if err != nil {
			log.Errorf("register finscan result handler failed: %s", err)
			return
		}

		var portInts []int
		for _, p := range utils.ParseStringToPorts(ports) {
			if config.IsFiltered("", p) {
				continue
			}
			portInts = append(portInts, p)
		}

		ports = utils.ConcatPorts(portInts)

		for target := range newTargetChan {
			if config.IsFiltered(target, 0) {
				continue
			}

			log.Infof("start to submit finscan for %s ports: %v", target, ports)
			// 默认的整体 target 一定要包含进去
			hostsFilter.Add(target)
			if !utils.IsIPv4(target) {
				hostsFilter.Add(netx.LookupAll(target, netx.WithTimeout(5*time.Second))...)
			}

			hostRaw, portRaw, _ := utils.ParseStringToHostPort(target)
			if portRaw > 0 {
				// 如果 host 可以解析出端口的话，就需要额外增加 host 的解析
				portsFilter.Add(fmt.Sprint(portRaw))
				hostsFilter.Add(hostRaw)
				if !utils.IsIPv4(hostRaw) {
					hostsFilter.Add(netx.LookupAll(hostRaw, netx.WithTimeout(5*time.Second))...)
				}
				_ = scanner.RandomScan(hostRaw, fmt.Sprint(portRaw), true)
			}
			err = scanner.RandomScan(target, ports, true)
			if err != nil {
				log.Errorf("submit finscan failed: %s", err)
				return
			}
		}
		scanner.WaitChannelEmpty()
		log.Infof("finished submitting.")

		log.Infof("waiting last packet (fin) for %v seconds", config.waiting) // waiting remote rsp half RTT
		select {
		case <-time.After(config.waiting):
		}
	}()
	return closeResult, nil
}

// FinPortScanExports 为了防止网卡过载，5个是上限
//  1. waiting 实现
//  2. timeout
var FinPortScanExports = map[string]interface{}{
	"Scan": func(target string, port string, opts ...finScanOpt) (chan *finscan.FinScanResult, error) {
		config := &_yakFinPortScanConfig{
			waiting:           10 * time.Second,
			rateLimitDelayMs:  1,
			rateLimitDelayGap: 5,
		}
		for _, opt := range opts {
			opt(config)
		}
		return _finscanDo(hostsToChan(target), port, config)
	},

	//"callback":           _finScanOptCallback,
	//"submitTaskCallback": _finScanOptSubmitTaskCallback,
	"excludePorts":   _finScanOptExcludePorts,
	"excludeHosts":   _finScanOptExcludeHosts,
	"wait":           _finScanOptWaiting,
	"outputFile":     _finScanOptOpenPortResult,
	"outputPrefix":   _finScanOptOpenPortResultPrefix,
	"initHostFilter": _finScanOptOpenPortInitHostFilter,
	"initPortFilter": _finScanOptOpenPortInitPortFilter,
	"rateLimit":      _finScanOptRateLimit,
	"concurrent":     _finScanOptConcurrent,
	//"fpOutputFile":       _scanOptFpResult,
	//"fingerprint":        _scanOptEnableFpScan,
	//"fingerprintTimeout": _scanOptFingerprintRequestTimeout,
}
