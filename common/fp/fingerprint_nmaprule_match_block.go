package fp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

func getDeadlineFromContext(ctx context.Context, timeout time.Duration) time.Time {
	ddl, ok := ctx.Deadline()
	if ok {
		return ddl
	}

	return time.Now().Add(timeout)
}

// readBanner stops when the probe context is cancelled. StableReader only
// watches its own timeout, so a cancelled scan used to keep the connection
// until that timeout expired.
func readBanner(ctx context.Context, conn net.Conn, timeout time.Duration, maxSize int) []byte {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	if ddl, ok := ctx.Deadline(); ok {
		if remain := time.Until(ddl); remain < timeout {
			if remain <= 0 {
				timeout = time.Millisecond
			} else {
				timeout = remain
			}
		}
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()
	banner := stableReader(conn, timeout, maxSize)
	close(stop)
	return banner
}

func dialProbeUDP(ctx context.Context, target string, config *Config) (net.Conn, error) {
	timeout := time.Second
	var proxies []string
	if config != nil {
		if config.ProbeTimeout > 0 {
			timeout = config.ProbeTimeout
		}
		proxies = config.Proxies
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opts := []netx.DialXOption{
		netx.DialX_WithContext(ctx),
		netx.DialX_WithTimeout(timeout),
	}
	triedSocks := false
	var last error
	for _, proxy := range proxies {
		if !socks5Proxy(proxy) {
			continue
		}
		triedSocks = true
		conn, err := netx.DialSocks5UDPContext(ctx, target, proxy, opts...)
		if err == nil {
			return conn, nil
		}
		last = err
		log.Errorf("socks5 udp associate failed: %v", err)
	}
	if triedSocks {
		return nil, last
	}
	conn, _, err := netx.DialUdpX(target, opts...)
	return conn, err
}

func socks5Proxy(proxy string) bool {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return false
	}
	scheme := proxy
	if i := strings.Index(proxy, "://"); i >= 0 {
		scheme = proxy[:i]
	}
	switch strings.ToLower(scheme) {
	case "socks", "socks5", "socks5h", "s5":
		return true
	default:
		return false
	}
}

// 设置 Banner 检查函数
var bannerToString = func(banner []byte) string {
	if banner == nil {
		return ""
	}
	result, err := utils2.GBKSafeString(banner)
	if err != nil {
		return strconv.Quote(string(banner))
	}

	return utils2.RemoveUnprintableChars(result)
}

var stableReader = utils2.StableReader

//func stableReader(conn io.Reader, timeout time.Duration, maxSize int) []byte {
//	var buffer bytes.Buffer
//	// read first connection
//	go io.Copy(&buffer, conn)
//	timer := time.After(timeout)
//	var banner []byte
//	var bannerHash string
//	for {
//		// check for every 0.5 seconds
//		select {
//		case <-timer:
//			break
//		default:
//			time.Sleep(500 * time.Millisecond)
//		}
//
//		if buffer.Bytes() == nil {
//			continue
//		}
//
//		if len(buffer.Bytes()) > maxSize {
//			banner = buffer.Bytes()
//			break
//		}
//
//		currentHash := codec.Sha1(buffer.Bytes())
//		if currentHash == bannerHash {
//			break
//		}
//		banner = buffer.Bytes()
//		bannerHash = currentHash
//	}
//	return banner
//}

func match(rule *NmapMatch, data []rune, port int, host net.IP, safeBanner string, proto TransportProto) *FingerprintInfo {
	matchResult, err := rule.MatchRule.FindRunesMatch(data)
	if err != nil {
		return nil
	}
	if matchResult != nil {
		info := ToFingerprintInfo(rule, matchResult)
		info.Banner = safeBanner
		info.Port = port
		info.IP = host.String()
		info.Proto = proto
		return info
	}
	return nil
}

func (f *Matcher) matchBlock(ctx context.Context, host net.IP, port int, block *RuleBlock, config *Config) (open PortState, _ *FingerprintInfo, _ error) {
	rootCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, config.ProbeTimeout)
	defer cancel()

	dialTimeout := config.ProbeTimeout
	if block.Probe.Payload == "" {
		dialTimeout = 5 * time.Second
	}

	if block.Probe.Proto == TCP || block.Probe.Proto == UDP || fmt.Sprint(block.Probe.Proto) == "" {
		if fmt.Sprint(block.Probe.Proto) == "" {
			block.Probe.Proto = TCP
		}

		if block.Probe.Proto == TCP && config.CanScanTCP() {
			dialCtx, dialCancel := context.WithTimeout(rootCtx, dialTimeout)
			conn, err := netx.DialContext(dialCtx, utils2.HostPort(host.String(), port), config.Proxies...)
			dialCancel()
			if err != nil {
				return CLOSED, nil, utils2.Errorf("%s: %v", block.Probe.Name, err)
			}
			defer conn.Close()

			// Send Payload
			if block.Probe.Payload != "" {
				log.Debugf(
					"active probe for %s:%v probe: [%v]%s %v",
					host, port, block.Probe.Proto, block.Probe.Name, strconv.Quote(block.Probe.Payload),
				)
				_ = conn.SetWriteDeadline(getDeadlineFromContext(ctx, 3*time.Second))
				_, _ = conn.Write([]byte(block.Probe.Payload))
			}

			banner := readBanner(ctx, conn, config.ProbeTimeout, config.FingerprintDataSize)
			log.Debugf(
				"active probe for %s:%v recv(%v)-%v: %v",
				host, port, block.Probe.Proto, block.Probe.Name, strconv.Quote(string(banner)),
			)
			// check banner
			var (
				resultFingerprintInfo = &FingerprintInfo{
					IP:          host.String(),
					Port:        port,
					ServiceName: string(block.Probe.Proto),
					//ProductVerbose: fmt.Sprintf("%s:%v", strings.ToUpper(string(block.Probe.Proto)), port),
					Banner: bannerToString(banner),
					CPEs:   []string{},
					Proto:  block.Probe.Proto,
				}
			)
			if banner == nil {
				return OPEN, resultFingerprintInfo, nil
			}
			shortBanner := utils2.RemoveUnprintableChars(string(banner))
			if len(shortBanner) > 30 {
				shortBanner = shortBanner[:30] + "..."
			}

			if info := applyMatchRules(rootCtx, block, banner, port, host, resultFingerprintInfo.Banner, block.Probe.Proto); info != nil {
				resultFingerprintInfo = info
			}
			return OPEN, resultFingerprintInfo, nil
		}

		if block.Probe.Proto == UDP && config.CanScanUDP() {
			if block.Probe.Payload == "" {
				return UNKNOWN, nil, utils2.Error("udp cannot support null banner")
			}
			var (
				resultFingerprintInfo = &FingerprintInfo{
					IP:          host.String(),
					Port:        port,
					ServiceName: string(block.Probe.Proto),
					//ProductVerbose: fmt.Sprintf("%s:%v", strings.ToUpper(string(block.Probe.Proto)), port),
					Banner: "",
					CPEs:   []string{},
					Proto:  block.Probe.Proto,
				}
			)

			udpConn, err := dialProbeUDP(ctx, utils2.HostPort(host.String(), port), config)
			if err != nil {
				return CLOSED, resultFingerprintInfo, nil
			}
			defer udpConn.Close()

			_, _ = udpConn.Write([]byte(block.Probe.Payload))
			banner := readBanner(ctx, udpConn, config.ProbeTimeout, config.FingerprintDataSize)
			if banner != nil {
				resultFingerprintInfo.Banner = bannerToString(banner)
				log.Infof("%s udp banner: %v", block.Probe.Name, resultFingerprintInfo.Banner)
				if info := applyMatchRules(rootCtx, block, banner, port, host, resultFingerprintInfo.Banner, block.Probe.Proto); info != nil {
					resultFingerprintInfo = info
				}
			} else {
				return UNKNOWN, resultFingerprintInfo, nil
			}

			return OPEN, resultFingerprintInfo, nil
		}
		return UNKNOWN, nil, errors.Errorf("BUG: Probe.Proto/Config.Proto is not right set")
	} else {
		return UNKNOWN, nil, errors.Errorf("BUG: Probe is not right setting")
	}
}

func (c *Config) CanScanTCP() bool {
	for _, i := range c.TransportProtos {
		if i == TCP {
			return true
		}
	}
	return false
}

func (c *Config) CanOnlyScanTCP() bool {
	for _, i := range c.TransportProtos {
		if i != TCP {
			return false
		}
	}
	return len(c.TransportProtos) > 0
}

func (c *Config) CanOnlyScanUDP() bool {
	if c == nil {
		return false
	}
	for _, i := range c.TransportProtos {
		if i != UDP {
			return false
		}
	}
	return len(c.TransportProtos) > 0
}

func (c *Config) CanScanUDP() bool {
	for _, i := range c.TransportProtos {
		if i == UDP {
			return true
		}
	}
	return false
}
