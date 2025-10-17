package fp

import (
	"sort"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
)

type byRarity []*RuleBlock

func (a byRarity) Len() int           { return len(a) }
func (a byRarity) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byRarity) Less(i, j int) bool { return a[i].Probe.Rarity < a[j].Probe.Rarity }

type byIndex []*RuleBlock

func (a byIndex) Len() int           { return len(a) }
func (a byIndex) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byIndex) Less(i, j int) bool { return a[i].Probe.Index < a[j].Probe.Index }

type byName []*RuleBlock

func (a byName) Len() int           { return len(a) }
func (a byName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byName) Less(i, j int) bool { return a[i].Probe.Name > a[j].Probe.Name }

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

			// 过滤稀有度
			if probe.Rarity > config.RarityMax {
				// log.Debugf("Probe %s is skipped for rarity is %v (config %v)", probe.Name, probe.Rarity, config.RarityMax)
				continue
			}

			// log.Debugf("use probe [%v]%s all %v match rules", probe.Proto, probe.Name, len(matches))
			blocks = append(blocks, &RuleBlock{
				Probe:   probe,
				Matched: matches,
			})
		}
	}

	// 如果 probe 端口匹配到了，则说明这些是最合适的
	// 同时也合并一些通用规则，以提高非标准端口服务的识别率
	if len(bestBlocks) > 0 {
		// 排序端口特定规则
		sort.Sort(byName(bestBlocks))
		sort.Sort(byIndex(bestBlocks))
		sort.Sort(byRarity(bestBlocks))
		//result := funk.Map(bestBlocks, func(i *RuleBlock) string {
		//	return i.Probe.Name
		//})
		//panic(strings.Join(result.([]string), "/"))

		if len(blocks) > 0 {
			sort.Sort(byName(blocks))
			sort.Sort(byIndex(blocks))
			sort.Sort(byRarity(blocks))
		}

		// 合并规则：bestBlocks 在前，blocks 在后
		combinedBlocks := append(bestBlocks, blocks...)

		// 应用 ProbesMax 限制
		if config.ProbesMax > 0 && len(combinedBlocks) > config.ProbesMax {
			log.Debugf("filter probe only[%v] by config ProbeMax, total: %v", config.ProbesMax, len(combinedBlocks))
			return emptyBlock, combinedBlocks[:config.ProbesMax], true
		}
		return emptyBlock, combinedBlocks, true
	}

	// 如果没有过滤出任何 blocks 就直接退出
	if len(blocks) <= 0 {
		return
	}

	sort.Sort(byName(blocks))
	sort.Sort(byIndex(blocks))
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
		log.Debugf("filter probe only[%v] by config ProbeMax, total: %v", config.ProbesMax, len(blocks))
		return emptyBlock, blocks[:config.ProbesMax], false
	}
	return
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
