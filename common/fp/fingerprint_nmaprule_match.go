package fp

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"net"
	"sync"
	"time"
)

func tcpConnectionMaker(host string, port interface{}, proxy []string, timeout time.Duration) (net.Conn, error) {
	proxy = utils2.StringArrayFilterEmpty(proxy)
	return utils2.TCPConnect(utils2.HostPort(host, port), timeout, proxy...)
}

func (f *Matcher) matchWithContext(ctx context.Context, ip net.IP, port int, config *Config) (*MatchResult, error) {
	host := ip.String()

	result := &MatchResult{
		Target: host,
		Port:   port,
		State:  UNKNOWN,
	}

	// 获取需要匹配的指纹
	firstBlock, blocks, bestMode := GetRuleBlockByConfig(port, config)
	if len(blocks) <= 0 && firstBlock == nil {
		return nil, errors.New("empty rules is not allowed")
	}
	_ = bestMode
	if firstBlock != nil {
		if config.CanScanUDP() {
			blocks = append(blocks, firstBlock)
		} else if config.CanScanTCP() {
			log.Infof("%s - %v ", firstBlock.Probe.Name, firstBlock.Probe.Proto)

			state, info, err := f.matchBlock(ctx, ip, port, firstBlock, config)
			result.State = state
			result.Fingerprint = info
			if err != nil {
				result.Reason = err.Error()
			}
			if (result.Fingerprint != nil && result.Fingerprint.Banner != "") || result.State == CLOSED {
				return specialCase(result, config), nil
			}
		}
	}

	// active mode is allowed to send packet
	if !config.ActiveMode {
		return specialCase(result, config), nil
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

		select {
		case <-ctx.Done():
			return specialCase(result, config), nil
		default:
		}

		// 处理结果
		probeSwg.Add()
		go func() {
			defer probeSwg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("probe failed: %s", err)
				}
			}()

			if block.Probe != nil {
				if block.Probe.Rarity > config.RarityMax && !utils2.IntArrayContains(block.Probe.DefaultPorts, port) {
					log.Debugf("filter probe[%v] for raritymax: %v current: %v", block.Probe.Name, config.RarityMax, block.Probe.Rarity)
					return
				}
			}
			log.Debugf("try %s probe[%v] rarity[%v] %#v", utils2.HostPort(host, port), block.Probe.Index, block.Probe.Rarity, block.Probe.Payload)
			state, info, err := f.matchBlock(ctx, ip, port, block, config)
			collectResultLock.Lock()
			defer collectResultLock.Unlock()
			states = append(states, state)
			infos = append(infos, info)
			if err != nil {
				log.Errorf("fingerprint match met err: %s", err)
				errs = append(errs, err)
			}
		}()
	}
	probeSwg.Wait()

	if result.State != OPEN {
		result.State = mergeStates(states...)
	}
	result.Fingerprint = mergeInfo(infos...)
	if result.State != OPEN && len(errs) > 0 {
		result.Reason = mergeError(errs...).Error()
	}

	if result.State == CLOSED {
		return specialCase(result, config), nil
	}

	if result.Fingerprint != nil && result.Fingerprint.ServiceName != "tcp" && result.Fingerprint.ServiceName != "udp" {
		return specialCase(result, config), nil
	}
	return specialCase(result, config), nil
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
		// 当有 Raw 字段时，代表和 match rule 匹配成功了
		if len(root.Raw) == 0 && len(infoIns.Raw) != 0 {
			root = infoIns
		}

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
