package fp

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

func tcpConnectionMaker(host string, port interface{}, proxy []string, timeout time.Duration) (net.Conn, error) {
	return netx.DialTCPTimeout(timeout, utils2.HostPort(host, port), proxy...)
}

func (f *Matcher) matchWithContext(ctx context.Context, ip net.IP, port int, host string, config *Config) (*MatchResult, error) {
	if host == "" {
		host = ip.String()
	}

	result := &MatchResult{
		Target:         host,
		Port:           port,
		State:          UNKNOWN,
		ProtocolFields: make(map[string]any),
	}
	deliver := func() (*MatchResult, error) {
		if result.State == OPEN {
			f.reportOpen(result)
		}
		return specialCase(ctx, result, config), nil
	}

	// 获取需要匹配的指纹. 缓存按端口和传输协议复用, 追加前先复制, 避免改到缓存切片.
	firstBlock, blocks, bestMode := f.ruleBlocks(port, config)
	if len(blocks) <= 0 && firstBlock == nil {
		return nil, errors.New("empty rules is not allowed")
	}
	_ = bestMode
	var earlyHTTP *inventoryHTTPProbe
	if firstBlock != nil {
		if config.CanScanUDP() {
			copied := make([]*RuleBlock, len(blocks), len(blocks)+1)
			copy(copied, blocks)
			blocks = append(copied, firstBlock)
		} else if config.CanScanTCP() {
			log.Debugf("%s - %v ", firstBlock.Probe.Name, firstBlock.Probe.Proto)

			httpIndex := plannedInventoryHTTPProbe(config, blocks)
			if httpIndex >= 0 {
				earlyHTTP = &inventoryHTTPProbe{block: blocks[httpIndex]}
				defer earlyHTTP.stop()
			}
			state, info, err := f.matchTCPBanner(ctx, ip, port, firstBlock, config, earlyHTTP)
			bannerPresent := info != nil && info.Banner != ""
			if state == OPEN && !bannerPresent && earlyHTTP != nil && earlyHTTP.done != nil {
				select {
				case <-earlyHTTP.done:
					if earlyHTTP.outcome == nil {
						earlyHTTP = nil
					} else if earlyHTTP.outcome.verifiedHTTP() {
						info = earlyHTTP.outcome.info
						f.log("verified HTTP response completed banner detection for %s", utils2.HostPort(host, port))
					}
				default:
				}
				// Reserve the running HTTP probe's slot in the existing concurrency limit.
				if earlyHTTP != nil && httpIndex > 0 {
					blocks = slices.Clone(blocks)
					copy(blocks[1:httpIndex+1], blocks[:httpIndex])
					blocks[0] = earlyHTTP.block
				}
			}
			result.State = state
			result.Fingerprint = info
			if info != nil {
				result.mergeProtocolFields(info.protocolFields)
			}
			if err != nil {
				result.Reason = err.Error()
			}
			if bannerPresent || result.State == CLOSED {
				return deliver()
			}
		}
	}

	// active mode is allowed to send packet
	if !config.ActiveMode {
		return deliver()
	}

	var collectResultLock = new(sync.Mutex)
	var states []PortState
	var infos []*FingerprintInfo
	var errs []error
	swgCon := config.ProbesConcurrentMax
	if swgCon <= 1 {
		swgCon = 1
	}
	var probeSwg = utils2.NewSizedWaitGroup(swgCon)
	for _, block := range blocks {
		block := block
		if block == nil || block.Probe.Payload == "" {
			continue
		}

		if err := probeSwg.AddWithContext(ctx); err != nil {
			break
		}

		go func() {
			defer probeSwg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("probe failed: %s", err)
				}
			}()

			if block.Probe != nil {
				if effectiveProbeRarity(block.Probe) > config.RarityMax && !utils2.IntArrayContains(block.Probe.DefaultPorts, port) {
					log.Debugf("filter probe[%v] for raritymax: %v current: %v", block.Probe.Name, config.RarityMax, effectiveProbeRarity(block.Probe))
					return
				}
			}
			log.Debugf("try %s probe[%v] rarity[%v] %#v", utils2.HostPort(host, port), block.Probe.Index, block.Probe.Rarity, block.Probe.Payload)
			var outcome *serviceProbeOutcome
			if earlyHTTP != nil && block == earlyHTTP.block && earlyHTTP.done != nil {
				<-earlyHTTP.done
				outcome = earlyHTTP.outcome
			}
			if outcome == nil {
				state, info, err := f.matchBlock(ctx, ip, port, block, config, nil)
				outcome = &serviceProbeOutcome{state: state, info: info, err: err}
			}
			collectResultLock.Lock()
			defer collectResultLock.Unlock()
			states = append(states, outcome.state)
			infos = append(infos, outcome.info)
			if outcome.err != nil {
				log.Errorf("fingerprint match met err: %s", outcome.err)
				errs = append(errs, outcome.err)
			}
		}()
	}
	probeSwg.Wait()
	for _, info := range infos {
		if info != nil {
			result.mergeProtocolFields(info.protocolFields)
		}
	}

	if result.State != OPEN {
		state := mergeStates(states...)
		if state == OPEN || ctx.Err() == nil {
			result.State = state
		}
	}
	if fingerprint := mergeInfo(infos...); fingerprint != nil || ctx.Err() == nil {
		result.Fingerprint = fingerprint
	}
	if result.State != OPEN && len(errs) > 0 {
		result.Reason = mergeError(errs...).Error()
	}

	return deliver()
}

const inventoryBannerGrace = 150 * time.Millisecond

type serviceProbeOutcome struct {
	state PortState
	info  *FingerprintInfo
	err   error
}

func (r *serviceProbeOutcome) verifiedHTTP() bool {
	return r != nil && r.err == nil && r.state == OPEN && r.info != nil &&
		r.info.confirmedPlainHTTP && r.info.ServiceName == "http"
}

type inventoryHTTPProbe struct {
	block   *RuleBlock
	done    chan struct{}
	cancel  context.CancelFunc
	outcome *serviceProbeOutcome
}

func plannedInventoryHTTPProbe(config *Config, blocks []*RuleBlock) int {
	if !config.PreferVerifiedHTTP || !config.ActiveMode || len(config.Proxies) != 0 {
		return -1
	}
	for i, block := range blocks {
		if block == nil || block.Probe == nil {
			continue
		}
		probe := block.Probe
		if probe.Proto == TCP && probe.Name == "GetRequest" && probe.Payload == "GET / HTTP/1.0\r\n\r\n" {
			return i
		}
	}
	return -1
}

func (f *Matcher) matchTCPBanner(ctx context.Context, ip net.IP, port int, block *RuleBlock, config *Config, httpProbe *inventoryHTTPProbe) (PortState, *FingerprintInfo, error) {
	if httpProbe == nil {
		return f.matchBlock(ctx, ip, port, block, config, nil)
	}
	bannerCtx, cancelBanner := context.WithCancel(ctx)
	defer cancelBanner()
	bannerDone := make(chan struct{})
	defer close(bannerDone)
	return f.matchBlock(bannerCtx, ip, port, block, config, func(conn *protocolResponseConn) {
		httpCtx, cancelHTTP := context.WithCancel(ctx)
		httpProbe.cancel = cancelHTTP
		httpProbe.done = make(chan struct{})
		go func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Errorf("inventory HTTP probe failed: %v", recovered)
				}
				verified := httpProbe.outcome.verifiedHTTP()
				close(httpProbe.done)
				if verified {
					cancelBanner()
				}
			}()
			grace := time.NewTimer(inventoryBannerGrace)
			defer grace.Stop()
			select {
			case <-bannerDone:
				return
			case <-httpCtx.Done():
				return
			case <-grace.C:
			}
			if conn.readBytes.Load() > 0 || httpCtx.Err() != nil {
				return
			}
			state, info, err := f.matchBlock(httpCtx, ip, port, httpProbe.block, config, nil)
			httpProbe.outcome = &serviceProbeOutcome{state: state, info: info, err: err}
		}()
	})
}

func (p *inventoryHTTPProbe) stop() {
	if p.done != nil {
		p.cancel()
		<-p.done
	}
}

func mergeStates(states ...PortState) PortState {
	for _, state := range states {
		if state == OPEN {
			return OPEN
		}
	}
	return CLOSED
}

// 当 ProbesMax 设置的比较大时,后探测的结果中可能是存在正确的指纹信息的
func mergeInfo(rawInfos ...*FingerprintInfo) *FingerprintInfo {
	var info []*FingerprintInfo
	for _, r := range rawInfos {
		if r == nil {
			continue
		}
		info = append(info, r)
	}
	if len(info) <= 0 {
		return nil
	}

	if len(info) == 1 {
		return info[0]
	}
	root := info[0]
	for _, infoIns := range info[1:] {
		if infoIns == nil {
			continue
		}
		if infoIns.IP != root.IP {
			continue
		}
		if infoIns.Port != root.Port {
			continue
		}
		confirmedPlainHTTP := root.confirmedPlainHTTP || infoIns.confirmedPlainHTTP
		// 当有 Raw 字段时，代表和 match rule 匹配成功了
		if len(root.Raw) == 0 && len(infoIns.Raw) != 0 {
			root = infoIns
		}

		root.confirmedPlainHTTP = confirmedPlainHTTP
		root.HttpFlows = append(root.HttpFlows, infoIns.HttpFlows...)
		root.CPEs = append(root.CPEs, infoIns.CPEs...)
	}
	return root
}

func mergeError(info ...error) error {
	if len(info) <= 0 {
		return nil
	}
	msg := fmt.Sprintf("Merged Error: \n")
	for index, i := range info {
		msg += fmt.Sprintf("  %v. %v\n", index, i)
	}
	return errors.New(msg)
}
