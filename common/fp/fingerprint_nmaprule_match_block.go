package fp

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"net"
	"strconv"
	"time"
)

func getDeadlineFromContext(ctx context.Context, timeout time.Duration) time.Time {
	ddl, ok := ctx.Deadline()
	if ok {
		return ddl
	}

	return time.Now().Add(timeout)
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
	ctx, _ = context.WithTimeout(ctx, config.ProbeTimeout)

	timeout := config.ProbeTimeout
	if block.Probe.Payload == "" {
		timeout = 5 * time.Second
	}

	if block.Probe.Proto == TCP || block.Probe.Proto == UDP || fmt.Sprint(block.Probe.Proto) == "" {
		if fmt.Sprint(block.Probe.Proto) == "" {
			block.Probe.Proto = TCP
		}

		if block.Probe.Proto == TCP && config.CanScanTCP() {
			conn, err := tcpConnectionMaker(host.String(), port, config.Proxies, timeout)
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

			banner := stableReader(conn, config.ProbeTimeout, config.FingerprintDataSize)
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

			rules := block.Matched
			shortBanner := utils2.RemoveUnprintableChars(string(banner))
			if len(shortBanner) > 30 {
				shortBanner = shortBanner[:30] + "..."
			}

			// check by regexp
			bannerRunesForMatchingRules := utils2.AsciiBytesToRegexpMatchedRunes(banner)
			for _, rule := range rules {
				if rootCtx.Err() != nil {
					break
				}
				if info := match(
					rule,
					bannerRunesForMatchingRules,
					port, host,
					resultFingerprintInfo.Banner, block.Probe.Proto); info != nil {
					resultFingerprintInfo = info
					break
				}
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

			udpConn, err := net.DialTimeout("udp", utils2.HostPort(host.String(), port), config.ProbeTimeout)
			if err != nil {
				return CLOSED, resultFingerprintInfo, nil
			}
			defer udpConn.Close()

			udpConn.Write([]byte(block.Probe.Payload))
			banner := stableReader(udpConn, config.ProbeTimeout, config.FingerprintDataSize)
			if banner != nil {
				resultFingerprintInfo.Banner = bannerToString(banner)
				log.Infof("%s udp banner: %v", block.Probe.Name, resultFingerprintInfo.Banner)
				bannerRunesForMatchingRules := utils2.AsciiBytesToRegexpMatchedRunes(banner)
				for _, rule := range block.Matched {
					if rootCtx.Err() != nil {
						break
					}
					if info := match(
						rule,
						bannerRunesForMatchingRules,
						port, host,
						resultFingerprintInfo.Banner, block.Probe.Proto); info != nil {
						resultFingerprintInfo = info
						break
					}
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

func (c *Config) CanScanUDP() bool {
	for _, i := range c.TransportProtos {
		if i == UDP {
			return true
		}
	}
	return false
}
