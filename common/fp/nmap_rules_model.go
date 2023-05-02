package fp

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"github.com/pkg/errors"
	"strconv"
	"strings"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
)

import (
	"fmt"
	regexp "github.com/dlclark/regexp2"
)

func init() {
	regexp.DefaultMatchTimeout = time.Second / 2
}

type TransportProto string

var (
	TCP TransportProto = "tcp"
	UDP TransportProto = "udp"
)

type NmapProbe struct {
	Index        int            `json:"index"`
	Rarity       int            `json:"rarity"`
	DefaultPorts []int          `json:"default_ports"`
	Proto        TransportProto `json:"proto"`
	Name         string         `json:"probe_name"`
	Payload      string         `json:"payload"`
	Raw          string         `json:"raw"`
}

type NmapMatch struct {
	ServiceName string `json:"service_name"`

	// m//
	MatchRule *regexp.Regexp `json:"match_rule"`

	// p//
	ProductVerbose string `json:"product_verbose"`

	// i//
	Info string `json:"info"`

	// v//
	Version string `json:"version_verbose"`

	// h
	Hostname string `json:"hostname"`

	// o
	OperationVerbose string `json:"operation_verbose"`

	// d
	DeviceType string `json:"device_type"`

	// From CPE
	CPEs []string `json:"cpes"`

	Raw string `json:"raw"`
}

func UnquoteCStyleString(raw string) (string, error) {
	state := ""
	results := ""
	hexBuffer := ""

	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanBytes)

	index := 0
	for scanner.Scan() {
		index++
		//log.Infof("index: %d", index)

		b := scanner.Bytes()[0]
		if b == '\\' && state != "charStart" {
			state = "charStart"
			continue
		}

		switch state {
		case "hex":
			if len(hexBuffer) < 2 {
				//log.Infof("hex: %s", string(b))
				hexBuffer += string(b)
			}

			if len(hexBuffer) == 2 {
				_ret, err := hex.DecodeString(hexBuffer)
				if err != nil {
					return "", errors.Errorf("parse hex buffer[\\x%s] failed: %s", hexBuffer, err)
				}

				if len(_ret) != 1 {
					return "", errors.Errorf("BUG: \\x%s to %#v", hexBuffer, _ret)
				}

				results += string(_ret)
				state = ""
				hexBuffer = ""
			} else if len(hexBuffer) > 2 {
				return "", errors.Errorf("parse hex failed: \\x%s", hexBuffer)
			} else {
				state = "hex"
			}

		case "charStart":
			switch b {
			case '0':
				results += "\x00"
				break
			case 'a':
				results += "\a"
				break
			case 'b':
				results += "\n"
				break
			case 'f':
				results += "\f"
				break
			case 'n':
				results += "\n"
				break
			case 'r':
				results += "\r"
				break
			case 't':
				results += "\t"
				break
			case 'v':
				results += "\v"
				break
			case 'x':
				state = "hex"
				continue
			}
			state = ""
			continue
		default:
			results += string(b)
			continue
		}

	}
	return results, nil
}

func ExtractBlockFromMatch(raw string) []string {
	results := []string{}
	if !strings.HasSuffix(raw, " ") {
		raw = raw + " "
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanBytes)

	state := "choose"

	currentTag := ""
	currentData := ""
	var delimeter byte = '/'

	for scanner.Scan() {
		b := scanner.Bytes()[0]

		switch state {
		case "open":
			delimeter = b
			state = "data"
		case "data":
			if delimeter == b {
				state = "close"
			}
			currentData += string(b)
			continue
		case "close":
			if b == ' ' {
				r := fmt.Sprintf(
					"%s%s%s",
					currentTag,
					string(delimeter),
					currentData,
				)

				results = append(
					results,
					r,
				)
				currentTag = ""
				currentData = ""
				delimeter = 0
				state = "choose"
				continue
			}
			currentData += string(b)
			continue
		case "choose":
			openDataTag := false
			for _, i := range []byte("pvihodm") {
				if i == b {
					openDataTag = true
				}
			}

			if openDataTag {
				currentTag = string(b)
				state = "open"
				continue
			}

			if b == 'c' {
				state = "cpe"
			}
			continue

		case "cpe":
			// match cpe:/
			NotCPE := false
			for index, i := range []byte("pe:") {
				if !(scanner.Bytes()[0] == i) {
					NotCPE = true
					break
				} else {
					if index <= 1 {
						scanner.Scan()
					}
				}
			}

			if NotCPE {
				state = "choose"
				continue
			}

			currentTag = "cpe:"
			state = "open"
			continue
		default:
			continue
		}
	}
	return results
}

func parseNmapProbe(line string) (*NmapProbe, error) {
	results := strings.SplitN(line, " ", 4)
	if len(results) != 4 {
		return nil, errors.New("Parse nmap probe failed: length of blocks is not 4")
	}

	ProbeBanner, ProtoRaw, Name, Payload := results[0], results[1], results[2], results[3]
	if ProbeBanner != "Probe" {
		return nil, errors.New("not a valid probe")
	}

	var proto TransportProto
	switch strings.ToUpper(ProtoRaw) {
	case "TCP":
		proto = TCP
		break
	case "UDP":
		proto = UDP
		break
	default:
		return nil, errors.Errorf("proto error, (only TCP/UDP) got %s", ProtoRaw)
	}

	// verfiy payload
	if len(Payload) < 3 {
		return nil, errors.New("invalid payload length")
	}

	sepStart, sepEnd := Payload[1], Payload[len(Payload)-1]
	if !(strings.HasPrefix(Payload, "q") && sepStart == sepEnd) {
		return nil, errors.Errorf("invalid payload format: %s", line)
	}

	RealPayload := Payload[2 : len(Payload)-1]
	payload, err := UnquoteCStyleString(RealPayload)
	if err != nil {
		return nil, errors.Errorf("unquote payload[%v] failed: %s", RealPayload, err)
	}
	//log.Debugf("fetch probe[%s] with:%s", line, payload)

	return &NmapProbe{
		Proto:   proto,
		Name:    Name,
		Payload: payload,
	}, nil
}

func parseNmapMatch(line string) (*NmapMatch, error) {
	rule, err := parseMatchRule([]byte(line))
	if err != nil {
		return nil, errors.Errorf("parse nmap rule[%s] failed: %s", line, err)
	}

	match := &NmapMatch{}
	match.Raw = line
	match.ServiceName = rule.ServiceName

	if block, ok := rule.DataBlocks['p']; ok {
		match.ProductVerbose = string(block.Content)
	}

	if b, ok := rule.DataBlocks['b']; ok {
		match.Version = string(b.Content)
	}

	if b, ok := rule.DataBlocks['i']; ok {
		match.Info = string(b.Content)
	}

	if b, ok := rule.DataBlocks['h']; ok {
		match.Hostname = string(b.Content)
	}

	if b, ok := rule.DataBlocks['o']; ok {
		match.OperationVerbose = string(b.Content)
	}

	if b, ok := rule.DataBlocks['d']; ok {
		match.DeviceType = string(b.Content)
	}

	if b, ok := rule.DataBlocks['m']; ok {
		var options regexp.RegexOptions
		if bytes.Contains(b.Option, []byte("i")) {
			options = options | regexp.IgnoreCase
		}

		if bytes.Contains(b.Option, []byte("s")) {
			options = options | regexp.Singleline
		}

		raw := string(b.Content)
		raw = strings.ReplaceAll(raw, `\0`, `\x00`)
		match.MatchRule, err = regexp.Compile(raw, options)
		if err != nil {
			return nil, errors.Errorf("compile %s failed: %s", string(b.Content), err)
		}
	}

	for _, b := range rule.CpeBlocks {
		match.CPEs = append(match.CPEs, fmt.Sprintf("cpe:/%s", string(b.Content)))
	}

	return match, nil
}

func parseRarity(line string) (int, error) {
	re, err := regexp.Compile(`rarity +(\d+)`, regexp.IgnoreCase)
	if err != nil {
		return 0, errors.Errorf("compile rarity regexp failed: %s", err)
	}

	match, err := re.FindStringMatch(line)
	if err != nil {
		return 0, errors.Errorf("find match failed: %s", err)
	}
	number := match.GroupByNumber(1)
	rarityInt, err := strconv.ParseInt(number.String(), 10, 32)
	if err != nil {
		return 0, errors.Errorf("parse rarity from string: %s failed: %s", number.String(), err)
	}

	return int(rarityInt), nil
}

func parsePorts(line string) []int {
	rets := strings.SplitN(line, " ", 2)
	if len(rets) < 2 {
		return nil
	} else {
		portsRaw := rets[1]
		return utils.ParseStringToPorts(portsRaw)
	}
}

func ParseNmapProbe(raw string) ([]*NmapProbe, error) {
	probes := []*NmapProbe{}

	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		probe, err := parseNmapProbe(line)
		if err != nil {
			log.Warnf("parse [%s] failed: %s", line, err)
			continue
		}

		probes = append(probes, probe)
	}

	return probes, nil
}

func ParseNmapMatch(raw string) ([]*NmapMatch, error) {
	probes := []*NmapMatch{}

	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		probe, err := parseNmapMatch(line)
		if err != nil {
			log.Warnf("parse [%s] failed: %s", line, err)
			continue
		}

		probes = append(probes, probe)
	}

	return probes, nil
}
