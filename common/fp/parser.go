package fp

import (
	"bufio"
	"bytes"
	"strings"
	"yaklang.io/yaklang/common/log"
)

func ParseNmapServiceProbeToRuleMap(raw []byte) (result map[*NmapProbe][]*NmapMatch, err error) {
	result = map[*NmapProbe][]*NmapMatch{}

	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanLines)

	var currentProbe = &NmapProbe{
		Index:   0,
		Proto:   TCP,
		Name:    "Default",
		Payload: "",
	}
	currentIndex := 0
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 跳过注释
		if strings.HasPrefix(line, "#") {
			//log.Debugf("parse comment: %s", line)
			continue
		}

		// 解析 Probe
		if strings.HasPrefix(line, "Probe") {
			probe, err := parseNmapProbe(line)
			if err != nil {
				log.Errorf("parse probe[%s] failed: %s", line, err)
				continue
			}
			currentIndex++
			probe.Index = currentIndex
			probe.Raw = line
			currentProbe = probe
		} else if strings.HasPrefix(line, "match") || strings.HasPrefix(line, "softmatch") {
			match, err := parseNmapMatch(line)
			if err != nil {
				log.Errorf("parse match[%s] failed: %s", line, err)
				continue
			}
			match.Raw = line

			if currentProbe == nil {

			}
			_, ok := result[currentProbe]
			if !ok {
				result[currentProbe] = []*NmapMatch{}
			}
			result[currentProbe] = append(result[currentProbe], match)
		} else if strings.HasPrefix(line, "rarity") {
			rarity, err := parseRarity(line)
			if err != nil {
				log.Errorf("parse[%s] current rarity failed: %s", line, err)
				continue
			}
			currentProbe.Rarity = rarity
		} else if strings.HasPrefix(line, "ports") {
			currentProbe.DefaultPorts = parsePorts(line)
		} else {
			//log.Debugf("unsupported line: %s", line)
		}
	}
	return
}

func ParseNmapServiceProbesTxt(raw string) ([]*NmapProbe, []*NmapMatch, []string) {
	var (
		probes     []*NmapProbe
		matches    []*NmapMatch
		failedRule []string
	)

	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			//log.Debugf("parse comment: %s", line)
			continue
		}

		if strings.HasPrefix(line, "Probe") {
			probe, err := parseNmapProbe(line)
			if err != nil {
				log.Errorf("parse probe[%s] failed: %s", line, err)
				continue
			}
			probes = append(probes, probe)
		} else if strings.HasPrefix(line, "match") || strings.HasPrefix(line, "softmatch") {
			match, err := parseNmapMatch(line)
			if err != nil {
				log.Errorf("parse match[%s] failed: %s", line, err)
				failedRule = append(failedRule, line)
				continue
			}
			matches = append(matches, match)
		} else {
			//log.Debugf("unsupported line: %s", line)
		}
	}
	return probes, matches, failedRule
}
