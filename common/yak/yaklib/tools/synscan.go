package tools

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/hybridscan"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/utils/pcapfix"
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

type _yakPortScanConfig struct {
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
	netInterface    string

	rateLimitDelayMs  float64
	rateLimitDelayGap int // 每隔多少数据包 delay 一次？

	excludeHosts *hostsparser.HostsParser
	excludePorts filter.Filterable

	// ctx 是本次 syn 扫描的取消上下文. 历史上 synscan 内部硬编码
	// context.Background(), 导致一旦目标是 tarpit / 全端口响应的异常主机, 即使上层
	// (如 AI 插件) 取消任务, syn 扫描仍会把全部端口扫完, 持续刷屏 + 占用网卡/CPU,
	// 形成资源泄漏. 注入可取消 ctx 后, cancel 会一路传到 hybridscan / synscan.Scanner,
	// 让发包循环与结果投递立刻短路退出.
	// 关键词: synscan ctx 注入, syn 扫描可取消, AI 插件 cancel 传播
	ctx context.Context

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

// rateLimit syn scan 的配置选项，设置 syn 扫描的速率
// @param {int} ms 延迟多少毫秒
// @param {int} count 每隔多少个数据包延迟一次
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.rateLimit(1, 2000) // 每隔 2000 个数据包延迟 1 毫秒
//
// )
// die(err)
// ```
func _scanOptRateLimit(ms int, count int) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.rateLimitDelayMs = float64(ms)
		config.rateLimitDelayGap = count
	}
}

// concurrent syn scan 的配置选项，设置 syn 扫描的并发数
// @param {int} count 并发数
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.concurrent(1000) // 并发 1000
//
// )
// die(err)
// ```
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

// iface syn scan 的配置选项，设置 syn 扫描的网卡
// @param {string} iface 网卡名称
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("192.168.1.1/24", "1-65535",
//
//	synscan.iface("eth0") // 使用 eth0 网卡
//
// )
// die(err)
// ```
func _scanOptIface(iface string) scanOpt {
	return func(config *_yakPortScanConfig) {
		if iface == "" {
			return
		}
		config.netInterface = iface
	}
}

// wait syn scan 的配置选项，设置等待对端的反应时间
// @param {float64} sec 等待时间，单位秒
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.wait(5) // 等待 5 秒
//
// )
// die(err)
// ```
func _scanOptWaiting(sec float64) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.waiting = utils.FloatSecondDuration(sec)
		if config.waiting <= 0 {
			config.waiting = 5 * time.Second
		}
	}
}

// excludePorts syn scan 的配置选项，设置本次扫描排除的端口
// @param {string} ports 端口，支持 1-65535、1,2,3、1-100,200-300 格式
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.excludePorts("1-100,200-300") // 排除 1-100 和 200-300 端口
//
// )
// die(err)
// ```
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

// excludeHosts syn scan 的配置选项，设置本次扫描排除的主机
// @param {string} hosts 主机，支持逗号分割、CIDR、-的格式
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("192.168.1.1/24", "1-65535",
//
//	synscan.excludeHosts("192.168.1.1,192.168.1.3-10,192.168.1.1/26")
//
// )
// die(err)
// ```
func _scanOptExcludeHosts(hosts string) scanOpt {
	return func(config *_yakPortScanConfig) {
		if hosts == "" {
			return
		}
		config.excludeHosts = hostsparser.NewHostsParser(context.Background(), hosts)
	}
}

// outputFile syn scan 的配置选项，设置本次扫描结果保存到指定的文件
// @param {string} file 文件路径
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.outputFile("/tmp/open_ports.txt")
//
// )
// die(err)
// ```
func _scanOptOpenPortResult(file string) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.outputFile = file
	}
}

// outputPrefix syn scan 的配置选项，设置本次扫描结果保存到文件时添加自定义前缀，比如 tcp:// https:// http:// 等，需要配合 outputFile 使用
// @param {string} prefix 前缀
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	 synscan.outputFile("./open_ports.txt"),
//		synscan.outputPrefix("tcp://")
//
// )
// die(err)
// ```
func _scanOptOpenPortResultPrefix(prefix string) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.outputFilePrefix = prefix
	}
}

// initHostFilter syn scan 的配置选项，设置本次扫描的初始主机过滤器
// @param {string} f 主机，支持逗号、CIDR、-分割
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("192.168.1.1/24", "1-65535",
//
//	synscan.initHostFilter("192.168.1.1,192.168.1.2")
//
// )
// die(err)
// ```
func _scanOptOpenPortInitHostFilter(f string) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.initFilterHosts = f
	}
}

// initPortFilter syn scan 的配置选项，设置本次扫描的初始端口过滤器
// @param {string} f 端口，支持逗号、-分割
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("192.168.3.1", "1-65535",
//
//	synscan.initPortFilter("1-100,200-300")
//
// )
// die(err)
// ```
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
	defer config.excludePorts.Close()

	filteredTargetChan, sampleTarget := filterTargetChannel(targetChan, config.IsFiltered)

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

func filterTargetChannel(targetChan chan string, filterFunc func(string, int) bool) (chan string, string) {
	var hasLoopback bool
	var hasSampleTarget bool
	sampleTargetChan := make(chan string, 1)
	newTargetChan := make(chan string, 2) // 2缓冲区,至少有一个是非127

	firstResult := <-targetChan // 取出一个目标 保证有返回值
	if utils.IsLoopback(firstResult) {
		newTargetChan <- "127.0.0.1" // 避免使用 loopback 网卡导致的源地址错误
		hasLoopback = true
	} else {
		sampleTargetChan <- firstResult
		hasSampleTarget = true
		newTargetChan <- firstResult
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer func() {
			close(newTargetChan)
			cancel()
		}()

		for {
			select {
			case result, ok := <-targetChan:
				if !ok {
					return
				}
				if filterFunc(result, 0) {
					continue
				}
				if !utils.IsLoopback(result) {
					if !hasSampleTarget {
						sampleTargetChan <- result
						hasSampleTarget = true
					}
					newTargetChan <- result
				} else if !hasLoopback { // 收取第一个本地回环目标，也仅收取一个
					// newTargetChan <- result // 避免使用 loopback 网卡导致的源地址错误
					newTargetChan <- "127.0.0.1" // 避免使用 loopback 网卡导致的源地址错误
					hasLoopback = true
				}
			}
		}
	}()

	select {
	case sampleTarget := <-sampleTargetChan:
		close(sampleTargetChan)
		return newTargetChan, sampleTarget
	case <-ctx.Done():
	}
	return newTargetChan, firstResult
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
	var (
		synScanOptions []synscan.ConfigOption
		err            error
		stringFilter   filter.Filterable
	)
	if config.netInterface != "" {
		synScanOptions, err = synscan.CreateConfigOptionsByIfaceName(config.netInterface)
	} else {
		synScanOptions, err = synscan.CreateConfigOptionsByTargetNetworkOrDomain(sampleTarget, 10*time.Second)
	}
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

	// scanCtx 取自调用方注入的可取消 ctx (缺省退化为 background). 把它一路透传给
	// hybridscan / synscan.Scanner, 让 cancel 能真正停下发包与等待, 而不是只能等
	// 整段端口扫完. 关键词: synscan runScan ctx 透传, cancel 快速收敛
	scanCtx := config.ctx
	if scanCtx == nil {
		scanCtx = context.Background()
	}

	log.Info("start create hyper scan center...")
	scanCenter, err := hybridscan.NewHyperScanCenter(scanCtx, scanCenterConfig)
	if err != nil {
		return utils.Errorf("create hyper scan center failed: %s", err)
	}

	defer func() {
		scanCenter.Close()
		close(openResult)
		stringFilter.Close()

		if err := recover(); err != nil {
			log.Errorf("syn failed: %v", err)
		}
	}()

	scanCenter.SetSynScanRateLimit(config.rateLimitDelayMs, config.rateLimitDelayGap)

	log.Info("preparing for result collectors")
	openPortLock := new(sync.Mutex)
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
	uid := uuid.New().String()
	hostsFilter := utils.NewHostsFilter()
	portsFilter := utils.NewPortsFilter(ports)
	stringFilter = filter.NewFilter()

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
			// 端口不在范围内,并且不在 host 、port exclude 中
			if !portsFilter.Contains(port) || config.IsFiltered(ip.String(), port) {
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

		// ctx 短路: cancel 后下游可能已停止消费 openResult, 这里不能死等, 否则
		// 持有 openPortLock 永久阻塞, 拖死整个扫描中心.
		select {
		case openResult <- result:
		case <-scanCtx.Done():
			return
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
		// ctx 短路: cancel 后立即停止提交后续目标, 避免对异常主机继续全端口扫描.
		if scanCtx.Err() != nil {
			log.Infof("syn scan submit loop stopped early: context canceled")
			break
		}
		if config.IsFiltered(target, 0) {
			continue
		}
		log.Debugf("start to submit synscan for %s ports: %v", target, ports)
		hostsFilter.Add(target)
		if !utils.IsIPv4(target) {
			hostsFilter.Add(netx.LookupAll(target, netx.WithTimeout(5*time.Second))...)
		}

		hostRaw, portRaw, _ := utils.ParseStringToHostPort(target)
		if portRaw > 0 {
			portsFilter.Add(fmt.Sprint(portRaw))
			hostsFilter.Add(hostRaw)
			if !utils.IsIPv4(target) {
				hostsFilter.Add(netx.LookupAll(target, netx.WithTimeout(5*time.Second))...)
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
	// ctx 短路: cancel 后无需再等待 waiting 收尾, 立即返回让 defer 关闭资源.
	select {
	case <-time.After(config.waiting):
	case <-scanCtx.Done():
		log.Infof("syn scan wait stage stopped early: context canceled")
	}

	log.Infof("total %v open port(s) found", openPortCount)

	return nil
}

// getFilteredPorts 去重、去除udp端口
func getFilteredPorts(ports string, config *_yakPortScanConfig) []int {
	var filteredPorts []int

	for _, p := range utils.ParseStringToPorts(ports) {
		proto, p := utils.ParsePortToProtoPort(p)
		if proto == "udp" {
			log.Errorf("UDP port is not supported in synscan, please use 'servicescan' to scan UDP port: %v", p)
			continue
		}
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
				//log.Infof("ping(%v) to synscan for target: %s", result.Reason, result.IP)
				c <- result.IP
			}
		}
	}()
	return c
}

// Scan 使用 SYN 扫描技术进行端口扫描，它不必打开一个完整的TCP连接，只发送一个SYN包，就能做到打开连接的效果，然后等待对端的反应
// @param {string} target 目标地址，支持 CIDR 格式
// @param {string} port 端口，支持 1-65535、1,2,3、1-100,200-300 格式
// @param {scanOpt} [opts] synscan 扫描参数
// @return {chan *synscan.SynScanResult} 返回结果
// Example:
// ```
// res, err := synscan.Scan("127.0.0.1", "1-65535") //
// die(err)
//
//	for result := range res {
//	  result.Show()
//	}
//
// ```
func _scan(target string, port string, opts ...scanOpt) (chan *synscan.SynScanResult, error) {
	config := getDefaultPortScanConfig()
	for _, opt := range opts {
		opt(config)
	}
	return _synScanDo(hostsToChan(target), port, config)
}

func getDefaultPortScanConfig() *_yakPortScanConfig {
	return &_yakPortScanConfig{
		waiting:           5 * time.Second,
		rateLimitDelayMs:  1,
		rateLimitDelayGap: 5,
		excludePorts:      filter.NewFilter(),
	}
}

// ScanFromPing 对使用 ping.Scan 探测出的存活结果进行端口扫描，需要配合 ping.Scan 使用
// @param {chan *PingResult} res ping.Scan 的扫描结果
// @param {string} ports 端口，支持 1-65535、1,2,3、1-100,200-300 格式
// @param {scanOpt} [opts] synscan 扫描参数
// @return {chan *synscan.SynScanResult} 返回结果
// Example:
// ```
// pingResult, err = ping.Scan("192.168.1.1/24") // 先进行存活探测
// die(err)
// res, err = synscan.ScanFromPing(pingResult, "1-65535") // 对存活结果进行端口扫描
// die(err)
//
//	for r := range res {
//	  r.Show()
//	}
//
// ```
func _synscanFromPingUtils(res chan *pingutil.PingResult, ports string, opts ...scanOpt) (chan *synscan.SynScanResult, error) {
	config := getDefaultPortScanConfig()
	for _, opt := range opts {
		opt(config)
	}

	return _synScanDo(pingutilsToChan(res), ports, config)
}

// callback syn scan 的配置选项，设置一个回调函数，每发现一个端口就会调用一次
// @param {func(i *synscan.SynScanResult)} i 回调函数
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.callback(func(i){
//	   db.SavePortFromResult(i) // 将结果保存到数据库
//	})
//
// )
// die(err)
// ```
func _scanOptCallback(i func(i *synscan.SynScanResult)) scanOpt {
	return func(config *_yakPortScanConfig) {
		config.callback = i
	}
}

// context syn scan 的配置选项，设置扫描的取消上下文。当 ctx 被取消时，syn 扫描会
// 尽快停止发包与结果投递，释放网卡/协程资源，避免对异常目标（如 tarpit/全端口响应
// 主机）持续扫描造成资源浪费与泄漏。
// @param {context.Context} ctx 取消上下文
// @return {scanOpt} 返回配置选项
// Example:
// ```
// ctx, cancel = context.WithCancel(context.Background())
// defer cancel()
// res, err = synscan.Scan("127.0.0.1", "1-65535", synscan.context(ctx))
// die(err)
// ```
func _scanOptContext(ctx context.Context) scanOpt {
	return func(config *_yakPortScanConfig) {
		if ctx == nil {
			return
		}
		config.ctx = ctx
	}
}

// submitTaskCallback syn scan 的配置选项，设置一个回调函数，每提交一个探测数据包的时候，这个回调会执行一次
// @param {func(string)} i 回调函数
// @return {scanOpt} 返回配置选项
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.submitTaskCallback(func(i){
//	   println(i) // 打印要探测的目标
//	})
//
// )
// die(err)
// ```
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
	"Scan":          _scan,
	"ScanFromPing":  _synscanFromPingUtils,

	"callback":           _scanOptCallback,
	"submitTaskCallback": _scanOptSubmitTaskCallback,
	"context":            _scanOptContext,
	"excludePorts":       _scanOptExcludePorts,
	"excludeHosts":       _scanOptExcludeHosts,
	"wait":               _scanOptWaiting,
	"outputFile":         _scanOptOpenPortResult,
	"outputPrefix":       _scanOptOpenPortResultPrefix,
	"initHostFilter":     _scanOptOpenPortInitHostFilter,
	"initPortFilter":     _scanOptOpenPortInitPortFilter,
	"rateLimit":          _scanOptRateLimit,
	"concurrent":         _scanOptSYNConcurrent,
	"iface":              _scanOptIface,
	//"fpOutputFile":       _scanOptFpResult,
	//"fingerprint":        _scanOptEnableFpScan,
	//"fingerprintTimeout": _scanOptFingerprintRequestTimeout,
}
