package netx

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var currentCPS atomic.Int64
var lastCPS atomic.Int64

func GetDialxCPS() int64 {
	return lastCPS.Load()
}

func init() {
	go func() {
		rpsTick := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-rpsTick.C:
				lastCPS.Store(currentCPS.Load())
				currentCPS.Store(0)
			}
		}
	}()
}

// dialPlainTCPConnWithRetry just handle plain tcp connection
// no tls here, but proxy here
func dialPlainTCPConnWithRetry(target string, config *dialXConfig) (retConn net.Conn, err error) {
	defer func() {
		if retConn != nil {
			currentCPS.Add(1)
		}
	}()

	var startTCP = time.Now()
	defer func() {
		if retConn != nil {
			config.TraceInfo.SetTCPDuration(time.Since(startTCP))
		}
	}()

	var retryMax int64 = 3
	if config.EnableTimeoutRetry {
		retryMax = config.TimeoutRetryMax
	}

	// do first as zero
	var timeoutRetryCount int64 = -1
	addTimeoutRetry := func() {
		atomic.AddInt64(&timeoutRetryCount, 1)
	}

	var refuseErrorRetryCount int64 = -1
	addRefuseErrorRetry := func() {
		atomic.AddInt64(&refuseErrorRetryCount, 1)
	}

	minWait, maxWait := config.TimeoutRetryMinWait, config.TimeoutRetryMaxWait
	if minWait > maxWait {
		minWait, maxWait = maxWait, minWait
	}

	var lastError error
RETRY:
	if timeoutRetryCount > retryMax || refuseErrorRetryCount > retryMax {
		if retryMax > 0 {
			return nil, fmt.Errorf("timeout retry(%v) or refuse retry(%v) > max(%v)", timeoutRetryCount, refuseErrorRetryCount, retryMax)
		}
		if lastError != nil {
			return nil, lastError
		}
		return nil, fmt.Errorf("i/o timeout for %v", target)
	}

	// handle plain
	// not need to upgrade
	var conn net.Conn

	if len(config.Proxy) == 0 && config.EnableSystemProxyFromEnv && FixProxy(GetProxyFromEnv()) != "" {
		config.Proxy = append(config.Proxy, FixProxy(GetProxyFromEnv()))
	}

	DnsConfig := NewDefaultReliableDNSConfig()
	for _, o := range config.DNSOpts {
		o(DnsConfig)
	}

	if DnsConfig.DisabledDomain != nil {
		host, _, err := utils.ParseStringToHostPort(target)
		if err == nil && DnsConfig.DisabledDomain.Contains(host) {
			return nil, utils.Errorf("disallow domain %v by config(check your yakit system/network config)", target)
		}
	}

	if len(config.Proxy) <= 0 || config.ForceDisableProxy {
		if len(config.Proxy) == 0 && !config.ForceDisableProxy && config.ForceProxy {
			return nil, utils.Errorf("force proxy but no proxy available for target: %v", target)
		}

		if config.Debug {
			log.Infof("dial %s without proxy", target)
		}
		host, port, err := utils.ParseStringToHostPort(target)
		if err != nil {
			return nil, utils.Errorf("invalid target %#v, cannot find host:port", target)
		}

		// Resolve hostname to IP for dialing
		ip := host
		if net.ParseIP(utils.FixForParseIP(host)) == nil {
			// not valid ip
			ip = LookupFirst(host, config.DNSOpts...)
		}
		if ip == "" {
			return nil, utils.Errorf("cannot resolve %v", target)
		}

		// handle ip address
		if config.DisallowAddress != nil {
			if config.DisallowAddress.Contains(ip) {
				return nil, utils.Errorf("disallow address %v by config(check your yakit system/network config)", ip)
			}
		}

		dialTarget := utils.HostPort(ip, port)

		// Handle strong host mode: bind to specific local IP address
		var localAddr *net.TCPAddr
		if config.StrongHostMode && config.StrongLocalAddrIP != "" {
			// Validate that StrongLocalAddrIP is actually an IP address
			localIP := net.ParseIP(utils.FixForParseIP(config.StrongLocalAddrIP))
			if localIP == nil {
				log.Warnf("strong host mode: StrongLocalAddrIP '%s' is not a valid IP address, ignoring", config.StrongLocalAddrIP)
			} else {
				localAddr = &net.TCPAddr{
					IP:   localIP,
					Port: 0, // Let system choose port
				}
				if config.Debug {
					log.Debugf("strong host mode: binding to local address %s", localAddr.String())
				}
			}
		}

		// Use TCPLocalAddr if explicitly set (takes precedence over strong host mode)
		if config.TCPLocalAddr != nil {
			localAddr = config.TCPLocalAddr
			if config.Debug {
				log.Debugf("using explicit TCPLocalAddr: %s", localAddr.String())
			}
		}

		// Dial with local address binding if specified
		if config.Dialer != nil {
			conn, err = config.Dialer(config.Timeout, dialTarget)
		} else if localAddr != nil {
			// Use net.Dialer to bind to local address
			dialer := &net.Dialer{
				Timeout:   config.Timeout,
				LocalAddr: localAddr,
			}
			conn, err = dialer.Dial("tcp", dialTarget)
		} else {
			conn, err = net.DialTimeout("tcp", dialTarget, config.Timeout)
		}
		if err != nil {
			if config.Debug {
				log.Errorf("dial %s failed: %v", target, err)
			}

			if config.StrongHostMode && config.StrongLocalAddrIP != "" {
				log.Infof("strong host mode dial failed target [%s] |strong host [%s] |failed reason [%s]", target, localAddr, err.Error())
			}

			lastError = err
			var opError *net.OpError
			switch {
			case errors.As(err, &opError):
				if opError.Timeout() && config.EnableTimeoutRetry {
					time.Sleep(utils.JitterBackoff(minWait, maxWait, int(timeoutRetryCount+1)))
					addTimeoutRetry()
					goto RETRY
				}
				if strings.Contains(opError.Error(), "refused") {
					time.Sleep(utils.JitterBackoff(minWait, maxWait, int(timeoutRetryCount+1)))
					addRefuseErrorRetry()
					goto RETRY
				}
			}
			return nil, err
		}
		return conn, nil
	}

	var errs error
	for _, proxy := range config.Proxy {
		conn, err := getConnForceProxy(target, proxy, config)
		if err != nil {
			log.Errorf("proxy conn failed: %s", err)
			lastError = err
			var opError *net.OpError
			switch {
			case errors.As(err, &opError):
				if opError.Timeout() && config.EnableTimeoutRetry {
					time.Sleep(utils.JitterBackoff(minWait, maxWait, int(timeoutRetryCount+1)))
					addTimeoutRetry()
					goto RETRY
				}
				if strings.Contains(opError.Error(), "refused") {
					time.Sleep(utils.JitterBackoff(minWait, maxWait, int(timeoutRetryCount+1)))
					addRefuseErrorRetry()
					goto RETRY
				}
			}
			errs = utils.JoinErrors(errs, err)
			continue
		}
		return conn, nil
	}
	return nil, utils.Wrapf(errs, "connect: %v failed: no proxy available (in %v)", target, config.Proxy)
}

/*
DialX is netx dial with more options

Proxy is a list of proxy servers, if empty, no proxy will be used, otherwise retry with each proxy until success (no redirect)
*/
func DialX(target string, opt ...DialXOption) (net.Conn, error) {
	config := &dialXConfig{
		Timeout:             10 * time.Second,
		TLSTimeout:          5 * time.Second,
		EnableTimeoutRetry:  false,
		TimeoutRetryMax:     3,
		TimeoutRetryMinWait: 100 * time.Millisecond,
		TimeoutRetryMaxWait: 500 * time.Millisecond,
		DisallowAddress:     utils.NewHostsFilter(),
		TLSNextProto:        []string{"http/1.1"},
	}

	// default init
	defaultDialXOptionsMutex.Lock()
	for _, o := range defaultDialXOptions {
		o(config)
	}
	defaultDialXOptionsMutex.Unlock()

	// user init
	for _, o := range opt {
		o(config)
	}

	startDialConn := time.Now() // dial all time
	defer func() {
		if config.TraceInfo != nil {
			config.TraceInfo.SetTotalDuration(time.Since(startDialConn))
		}
	}()

	clientHelloSpec := config.ClientHelloSpec
	useTls := config.EnableTLS || config.GMTLSSupport

	if !useTls {
		if config.Debug {
			log.Infof("dial %s without tls", target)
		}
		return dialPlainTCPConnWithRetry(target, config)
	}

	// Enable TLS as default
	strategies := []TLSStrategy{TLS_Strategy_Ordinary}
	if config.GMTLSSupport {
		if config.GMTLSOnly {
			strategies = []TLSStrategy{TLS_Strategy_GMDail, TLS_Strategy_Ordinary}
		} else if config.GMTLSPrefer {
			strategies = []TLSStrategy{TLS_Strategy_GMDail, TLS_Strategy_Ordinary}
		} else {
			strategies = []TLSStrategy{TLS_Strategy_Ordinary, TLS_Strategy_GMDail}
		}
	}

	sni := utils.ExtractHost(target)

	minVer, maxVer := consts.GetGlobalTLSVersion()
	var tlsConfig = &gmtls.Config{
		ServerName:         sni,
		MinVersion:         minVer, // nolint[:staticcheck]
		MaxVersion:         maxVer,
		InsecureSkipVerify: true,
		Renegotiation:      gmtls.RenegotiateFreelyAsClient,
	}
	if config.ShouldOverrideTLSConfig {
		tlsConfig = config.TLSConfig
		if tlsConfig.ServerName == "" { // sni is empty , default sni require not empty
			tlsConfig.ServerName = sni
		}
	}
	if config.ShouldOverrideSNI {
		tlsConfig.ServerName = config.SNI
	}

	tlsTimeout := 10 * time.Second
	if config.TLSTimeout > 0 {
		tlsTimeout = config.TLSTimeout
	}

	errs := make([]error, 0, len(strategies))
	for _, strategy := range strategies {
		tempTlsConfig := tlsConfig.Clone()
		if config.Debug {
			log.Infof("dial %v with tls strategy: %v", target, strategy)
		}
		conn, err := dialPlainTCPConnWithRetry(target, config)
		if err != nil {
			return nil, err
		}

		switch strategy {
		case TLS_Strategy_Ordinary:
			tempTlsConfig.GMSupport = nil
		case TLS_Strategy_GMDail:
			tempTlsConfig.GMSupport = &gmtls.GMSupport{
				WorkMode: gmtls.ModeGMSSLOnly,
			}
		default:
			return nil, utils.Errorf("unknown tls strategy %v", strategy)
		}

		startTLSHandshake := time.Now()
		tlsConn, err := UpgradeToTLSConnectionWithTimeout(conn, sni, tempTlsConfig, tlsTimeout, clientHelloSpec, config.TLSNextProto...)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		config.TraceInfo.SetTLSHandshakeDuration(time.Since(startTLSHandshake))
		return tlsConn, nil
	}
	if len(errs) > 0 {
		lastErr := errs[len(errs)-1]
		reasonEN, reasonCN, suggestion := getFriendlyTLSError(lastErr)

		var configParts []string
		if config.ForceDisableProxy {
			configParts = append(configParts, "proxy=disabled")
		} else if len(config.Proxy) > 0 {
			configParts = append(configParts, fmt.Sprintf("proxy=%v", config.Proxy))
		} else if config.EnableSystemProxyFromEnv {
			configParts = append(configParts, "proxy=system")
		} else {
			configParts = append(configParts, "proxy=none")
		}
		if config.SNI != "" {
			sniInfo := fmt.Sprintf("sni=%v", config.SNI)
			if config.ShouldOverrideSNI {
				sniInfo += "(override)"
			}
			configParts = append(configParts, sniInfo)
		}
		configStr := strings.Join(configParts, ", ")

		return nil, utils.Errorf("all tls strategy failed: TLS Connection Failed(%s): %s | Raw Error: %v | Config: %s\nTLS连接失败(%s): %s | 原始错误: %v | 调试配置: %s\n%s",
			target, reasonEN, lastErr, configStr,
			target, reasonCN, lastErr, configStr, suggestion)
	}
	return nil, utils.Error("unknown tls strategy error, BUG here!")
}

// dialPlainUdpConn get abstract udp conn, with global netx config (disallow address, etc)
func dialPlainUdpConn(target string, config *dialXConfig) (udpConn *net.UDPConn, remoteAddr *net.UDPAddr, err error) {
	host, port, err := utils.ParseStringToHostPort(target)
	if err != nil {
		return nil, nil, utils.Errorf("invalid target %#v, cannot find host:port", target)
	}

	host = utils.FixForParseIP(host)
	ipIns := net.ParseIP(host)
	if ipIns == nil {
		// not valid ip
		host = LookupFirst(host, config.DNSOpts...)
		if ipIns = net.ParseIP(host); ipIns == nil {
			return nil, nil, utils.Errorf("cannot resolve %v", target)
		}
	}

	// handle ip address
	if config.DisallowAddress != nil {
		if config.DisallowAddress.Contains(host) {
			return nil, nil, utils.Errorf("disallow address %v by config(check your yakit system/network config)", host)
		}
	}

	remoteAddr = &net.UDPAddr{
		IP:   ipIns,
		Port: port,
	}

	if config.JustListen {
		udpConn, err = net.ListenUDP("udp", config.LocalAddr)
		return
	}

	udpConn, err = net.DialUDP("udp", config.LocalAddr, remoteAddr)
	return
}

func DialUdpX(target string, opt ...DialXOption) (*net.UDPConn, *net.UDPAddr, error) {
	config := &dialXConfig{
		DisallowAddress: utils.NewHostsFilter(),
	}

	// default init
	defaultDialXOptionsMutex.Lock()
	for _, o := range defaultDialXOptions {
		o(config)
	}
	defaultDialXOptionsMutex.Unlock()

	// user init
	for _, o := range opt {
		o(config)
	}
	return dialPlainUdpConn(target, config)
}

func getFriendlyTLSError(err error) (en, cn, suggestion string) {
	if err == nil {
		return "Unknown Error", "未知错误", ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "EOF"), strings.Contains(msg, "unexpected EOF"):
		return "Connection Closed by Remote", "连接被远程关闭(目标服务器主动断开连接)",
			"建议: 1.检查目标是否支持HTTPS 2.尝试开启「随机TLS指纹」 3.检查是否与其他代理软件冲突: 如Clash/V2Ray开启了TUN或Fake-IP模式请尝试关闭"
	case strings.Contains(msg, "permission denied"):
		return "Permission Denied", "连接被系统拒绝(权限不足或被防火墙拦截)",
			"建议: 1.检查防火墙设置 2.尝试以管理员权限运行 3.国产系统(如银河麒麟)请检查「安全中心」-「网络保护」-「应用程序联网」是否拦截了本程序"
	case strings.Contains(msg, "connection refused"):
		return "Connection Refused", "目标服务器拒绝连接(端口未开放或服务未启动)",
			"建议: 1.确认目标端口正确 2.确认目标服务已启动"
	case strings.Contains(msg, "connection reset"):
		return "Connection Reset", "连接被重置(可能被防火墙或安全设备拦截)",
			"建议: 1.尝试开启「随机TLS指纹」绕过检测 2.检查是否被WAF拦截 3.尝试使用代理"
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "deadline exceeded"):
		return "Connection Timeout", "连接超时(网络不通或目标无响应)",
			"建议: 1.检查网络连接 2.确认目标可达 3.尝试增加超时时间"
	case strings.Contains(msg, "no such host"):
		return "DNS Lookup Failed", "域名解析失败(DNS无法解析该域名)",
			"建议: 1.检查域名拼写 2.尝试更换DNS服务器 3.检查网络连接"
	case strings.Contains(msg, "certificate"), strings.Contains(msg, "x509"):
		return "Certificate Verify Failed", "证书验证失败",
			"建议: 1.目标使用自签名证书属正常现象 2.如目标为国密站点可尝试开启「国密TLS」"
	case strings.Contains(msg, "handshake failure"):
		return "TLS Handshake Failed", "TLS握手失败(协议不兼容)",
			"建议: 1.尝试开启「随机TLS指纹」 2.如目标为国密站点可尝试开启「国密TLS」 3.检查TLS版本兼容性"
	case strings.Contains(msg, "network is unreachable"):
		return "Network Unreachable", "网络不可达",
			"建议: 1.检查网络连接 2.检查路由配置"
	case strings.Contains(msg, "no route to host"):
		return "No Route to Host", "无法路由到目标主机",
			"建议: 1.检查目标IP是否正确 2.检查网络和路由配置"
	case strings.Contains(msg, "protocol version"):
		return "TLS Version Mismatch", "TLS协议版本不兼容",
			"建议: 1.尝试开启「随机TLS指纹」 2.如目标为国密站点可尝试开启「国密TLS」"
	case strings.Contains(msg, "closed"):
		return "Connection Closed", "连接已关闭",
			"建议: 1.尝试开启「随机TLS指纹」 2.检查是否与其他代理软件冲突: 如Clash/V2Ray开启了TUN或Fake-IP模式请尝试关闭"
	default:
		return "Connection Failed", "连接失败",
			"建议: 1.尝试开启「随机TLS指纹」 2.检查网络连接"
	}
}
