package utils

import (
	"bufio"
	"bytes"
	"regexp"
	"github.com/yaklang/yaklang/common/log"
)

type ProtoType string

var (
	TCPProbe        ProtoType = "tcp"
	UDPProbe        ProtoType = "udp"
	probesRegex, _            = regexp.Compile(`Probe (UDP|TCP) (.*?) q\|(.*?)\|`)
	matchedRegex, _           = regexp.Compile(`.*m\|(.*?)\|.*`)
)

type ProbeRule struct {
	Type    ProtoType
	Payload []byte
	Matched []*MatchedRule
}

type MatchedRule struct {
	Matched *regexp.Regexp
}

func ParseNmapServiceMatchedRule(raw []byte) []*MatchedRule {
	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanLines)

	var rules []*MatchedRule

	for scanner.Scan() {
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)

		// skip comment
		if bytes.HasPrefix(line, []byte("#")) || len(line) <= 0 {
			continue
		}

		//log.Infof("line: %s", line)

		if bytes.HasPrefix(bytes.ToLower(line), []byte("match")) {
			for _, raw := range matchedRegex.FindAllSubmatch(line, 1) {
				r, err := regexp.Compile(string(raw[1]))
				if err != nil {
					log.Errorf("failed to compile rule[%s]: %s", line, err)
					continue
				}
				rules = append(rules, &MatchedRule{
					Matched: r,
				})
			}
		} else if bytes.HasPrefix(bytes.ToLower(line), []byte("probe")) {
			log.Error("failed to parse Probes(use ParseNmapServiceProbeRule)")
		}
	}

	return rules
}

func ParseNmapServiceProbeRule(raw []byte) []*ProbeRule {
	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanLines)

	var rules []*ProbeRule

	var currentRule *ProbeRule
	var matchedRules []*MatchedRule
	for scanner.Scan() {
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)

		// skip comment
		if bytes.HasPrefix(line, []byte("#")) || len(line) <= 0 {
			continue
		}

		log.Infof("line: %s", line)

		if bytes.HasPrefix(bytes.ToLower(line), []byte("match")) {
			for _, raw := range matchedRegex.FindAllSubmatch(line, 1) {
				r, err := regexp.Compile(string(raw[1]))
				if err != nil {
					log.Errorf("failed to compile rule[%s]: %s", line, err)
					continue
				}
				matchedRules = append(matchedRules, &MatchedRule{
					Matched: r,
				})
			}
		} else if bytes.HasPrefix(line, []byte("Probe ")) {
			if currentRule != nil {
				currentRule.Matched = matchedRules[:]
				rules = append(rules, currentRule)

				// test
				matchedRules = []*MatchedRule{}
				currentRule = nil
			}

			currentRule = &ProbeRule{}
			for _, results := range probesRegex.FindAllSubmatch(line, 1) {
				protoRaw := string(results[1])
				//productRaw := results[2]
				payloadRaw := results[3]

				switch protoRaw {
				case "TCP":
					currentRule.Type = TCPProbe
					break
				case "UDP":
					currentRule.Type = UDPProbe
					break
				default:
					continue
				}

				currentRule.Payload = ParseCStyleBinaryRawToBytes(payloadRaw)
			}
		}
	}

	if currentRule != nil && len(matchedRules) > 0 {
		currentRule.Matched = matchedRules
		rules = append(rules, currentRule)
	}

	return rules
}
