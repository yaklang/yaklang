package fp

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
)

func GetRuleBlockByConfig(currentPort int, config *Config) (emptyBlock *RuleBlock, blocks []*RuleBlock, ok bool) {
	var bestBlocks []*RuleBlock
	for probe, matches := range config.GetFingerprintRules() {

		// 只有 TCP 才能匹配 TCP
		if probe.Payload == "" && config.CanScanTCP() {
			if emptyBlock == nil {
				emptyBlock = &RuleBlock{Probe: probe, Matched: matches}
			} else {
				emptyBlock.Matched = append(emptyBlock.Matched, matches...)
			}
			continue
		}

		if funk.InInts(probe.DefaultPorts, currentPort) {
			// 检查协议和配置协议是否相同
			if probe.Proto == TCP && config.CanScanTCP() {
				bestBlocks = append(bestBlocks, &RuleBlock{Probe: probe, Matched: matches})
			}

			if probe.Proto == UDP && config.CanScanUDP() {
				bestBlocks = append(bestBlocks, &RuleBlock{Probe: probe, Matched: matches})
			}
		} else {
			// 过滤协议
			isFitProto := false
			for _, proto := range config.TransportProtos {
				if probe.Proto == proto {
					isFitProto = true
				}
			}
			if !isFitProto {
				// log.Debugf("skip [%v] %s for config[%#v]", probe.Proto, probe.Name, config.TransportProtos)
				continue
			}

			// 根据是否是主动模式规律规则
			if !config.ActiveMode {
				if len(probe.Payload) > 0 {
					// log.Debugf("skipped [%v] %s for active sending payload", probe.Proto, probe.Name)
					continue
				}
			}

			// 过滤稀有度。覆盖值只影响非端口探针，端口命中在上面已经收进 bestBlocks。
			if effectiveProbeRarity(probe) > config.RarityMax {
				continue
			}

			// log.Debugf("use probe [%v]%s all %v match rules", probe.Proto, probe.Name, len(matches))
			blocks = append(blocks, &RuleBlock{
				Probe:   probe,
				Matched: matches,
			})
		}
	}

	// 端口命中的探针单独占一份预算，不跟通用探针抢 ProbesMax，也不因稀有度被丢掉。
	// 通用预算里再把常见协议的覆盖探针换进来。总上限见 defaultProbeTotalBudget。
	sortPortProbeBlocks(bestBlocks)
	sortProbeBlocks(blocks)
	portLimit, genericLimit, limited := selectionLimits(config)
	if limited && len(bestBlocks) > portLimit {
		log.Debugf("keep %d of %d port-matched probes", portLimit, len(bestBlocks))
		bestBlocks = bestBlocks[:portLimit]
	}
	candidates := blocks
	if limited {
		blocks = preferLoweredProbes(candidates, blocks, bestBlocks, genericLimit, config.RarityMax)
	}

	if len(bestBlocks) > 0 {
		return emptyBlock, append(bestBlocks, blocks...), true
	}
	if len(blocks) == 0 {
		return
	}
	return emptyBlock, blocks, false
}

func GetRuleBlockByServiceName(serviceName string, config *Config) (blocks []*RuleBlock) {
	blockMap := make(map[string]*RuleBlock)
	// 遍历所有指纹规则
	for probe, matches := range config.GetFingerprintRules() {
		// 在每个规则块中查找符合条件的服务名
		for _, match := range matches {
			// 如果服务名包含指定的 serviceName
			if match.ServiceName == serviceName {
				// 如果已经有相同的probe.Name，直接在其Matched中追加
				if block, ok := blockMap[probe.Name]; ok {
					block.Matched = append(block.Matched, match)
				} else {
					// 否则创建新的RuleBlock并添加到blocks和blockMap中
					block := &RuleBlock{
						Probe:   probe,
						Matched: []*NmapMatch{match},
					}
					blocks = append(blocks, block)
					blockMap[probe.Name] = block
				}
				// 由于一个规则块可能有多个匹配项，所以这里不要 break，继续搜索
			}
		}
	}
	return
}
