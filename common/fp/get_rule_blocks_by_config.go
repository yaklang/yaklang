package fp

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"sort"
)

type byRarity []*RuleBlock

func (a byRarity) Len() int           { return len(a) }
func (a byRarity) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byRarity) Less(i, j int) bool { return a[i].Probe.Rarity < a[j].Probe.Rarity }

type byName []*RuleBlock

func (a byName) Len() int           { return len(a) }
func (a byName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byName) Less(i, j int) bool { return a[i].Probe.Name > a[j].Probe.Name }

func GetRuleBlockByConfig(currentPort int, config *Config) (emptyBlock *RuleBlock, blocks []*RuleBlock, ok bool) {
	var bestBlocks []*RuleBlock
	for probe, matches := range config.FingerprintRules {

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
				//log.Debugf("skip [%v] %s for config[%#v]", probe.Proto, probe.Name, config.TransportProtos)
				continue
			}

			// 根据是否是主动模式规律规则
			if !config.ActiveMode {
				if len(probe.Payload) > 0 {
					//log.Debugf("skipped [%v] %s for active sending payload", probe.Proto, probe.Name)
					continue
				}
			}

			// 过滤稀有度
			if probe.Rarity > config.RarityMax {
				//log.Debugf("Probe %s is skipped for rarity is %v (config %v)", probe.Name, probe.Rarity, config.RarityMax)
				continue
			}

			//log.Debugf("use probe [%v]%s all %v match rules", probe.Proto, probe.Name, len(matches))
			blocks = append(blocks, &RuleBlock{
				Probe:   probe,
				Matched: matches,
			})
		}
	}

	// 如果 probe 端口匹配到了，则说明这些是最合适的，如果匹配不到，再去使用剩下的内容
	if len(bestBlocks) > 0 {
		sort.Sort(byName(bestBlocks))
		sort.Sort(byRarity(bestBlocks))
		//result := funk.Map(bestBlocks, func(i *RuleBlock) string {
		//	return i.Probe.Name
		//})
		//panic(strings.Join(result.([]string), "/"))
		if config.ProbesMax > 0 && config.ProbesMax < len(bestBlocks) {
			log.Infof("filter probe only[%v] by config ProbeMax, total: %v", config.ProbesMax, len(bestBlocks))
			return emptyBlock, bestBlocks[:config.ProbesMax], true
		}
		return emptyBlock, bestBlocks, true
	}

	// 如果没有过滤出任何 blocks 就直接退出
	if len(blocks) <= 0 {
		return
	}

	sort.Sort(byName(blocks))
	sort.Sort(byRarity(blocks))
	blocks = funk.Filter(blocks, func(block *RuleBlock) bool {
		if block.Probe == nil {
			if block.Probe.Rarity > config.RarityMax {
				return false
			}
		}
		return true
	}).([]*RuleBlock)
	if config.ProbesMax > 0 && config.ProbesMax < len(blocks) {
		log.Infof("filter probe only[%v] by config ProbeMax, total: %v", config.ProbesMax, len(blocks))
		return emptyBlock, blocks[:config.ProbesMax], true
	}
	return
}
