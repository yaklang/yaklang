// Package send
// @Author bcy2007  2023/9/18 11:39
package send

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/bas/core"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"runtime"
	"time"
)

const (
	infoMessage = "log"
	basMessage  = "bas"
)

type Sender struct {
	target string
	rules  map[int]string
	iface  string
}

type Message struct {
	Message     interface{} `json:"content"`
	MessageType string      `json:"type"`
}

func (message *Message) ToJsonStr() string {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Errorf("message to json string error: %v", err)
		return ""
	}
	return string(messageBytes)
}

type RuleInfo struct {
	RuleID int      `json:"ruleId"`
	Md5    []string `json:"md5"`
}

func (ruleInfo *RuleInfo) ToJsonStr() string {
	ruleBytes, err := json.Marshal(ruleInfo)
	if err != nil {
		log.Errorf("rule info to json string error: %v", err)
		return ""
	}
	return string(ruleBytes)
}

func CreateSender(target string, rules map[int]string) (*Sender, error) {
	sender := &Sender{
		target: target,
		rules:  rules,
	}
	err := sender.init()
	if err != nil {
		return nil, utils.Errorf("Sender init error: %v", err)
	}
	return sender, nil
}

func (sender *Sender) init() error {
	var iface string
	var err error
	system := runtime.GOOS
	if system == "darwin" {
		iface, err = core.GetInterfaceInDarwin()
	} else if system == "linux" {
		iface, err = core.GetInterfaceInLinux()
	} else {
		return utils.Errorf("system %v not supported", runtime.GOOS)
	}
	if err != nil {
		return utils.Errorf("get interface info error: %v", err)
	}
	if iface == "" {
		return utils.Error("no interface info get")
	}
	sender.iface = iface
	return nil
}

func (sender *Sender) ruleReplace(ruleStr string) (string, error) {
	if sender.target == "" {
		return "", utils.Error("send packet target blank")
	}
	itemCompiler, _ := regexp.Compile(`(any|\$[a-zA-Z_]+|\d+|\[.+])`)
	items := itemCompiler.FindAllString(ruleStr, 4)
	if len(items) < 4 {
		return "", utils.Error("cannot find enough rule src & dst")
	}
	targetStr := fmt.Sprintf("%v %v -> %v %v", core.TestIP+"/32", items[1], sender.target+"/32", items[3])
	targetCompiler, _ := regexp.Compile(`(any|\$[a-zA-Z_]+|\d+|\[.+])\s+(any|\$[a-zA-Z_]+|\d+|\[.+])\s+->\s+(any|\$[a-zA-Z_]+|\d+|\[.+])\s+(any|\$[a-zA-Z_]+|\d+|\[.+])`)
	target := targetCompiler.FindAllString(ruleStr, -1)
	if len(target) == 0 {
		return "", utils.Error("cannot find enough rule target")
	}
	finalStr := targetCompiler.ReplaceAllString(ruleStr, targetStr)
	return finalStr, nil
}

func (sender *Sender) SendPack() error {
	infoList := make([]RuleInfo, 0)
	packets := make([][]byte, 0)
	for ruleID, ruleInfo := range sender.rules {
		p, md5List, err := sender.generatePack(ruleInfo)
		if err != nil {
			msg := Message{Message: fmt.Sprintf("rule %v generate packet error: %v", ruleInfo, err), MessageType: infoMessage}
			log.Error(msg.ToJsonStr())
			continue
		}
		ruleInfo := RuleInfo{RuleID: ruleID, Md5: md5List}
		infoList = append(infoList, ruleInfo)
		packets = append(packets, p...)
		if len(packets) >= 100 {
			msg := Message{Message: infoList, MessageType: basMessage}
			//log.Info(msg.ToJsonStr())
			fmt.Println(msg.ToJsonStr())
			//for num, packet := range packets {
			//	pcapx.InjectRaw(packet, pcapx.WithIface(sender.iface))
			//	if num%5 == 0 {
			//		time.Sleep(100 * time.Millisecond)
			//	}
			//}
			infoList = infoList[:0]
			packets = packets[:0]
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(packets) > 0 {
		msg := Message{Message: infoList, MessageType: basMessage}
		//log.Info(msg.ToJsonStr())
		fmt.Println(msg.ToJsonStr())
		//for _, packet := range packets {
		//	pcapx.InjectRaw(packet, pcapx.WithIface(sender.iface))
		//}
	}
	return nil
}

func (sender *Sender) generatePack(ruleStr string) ([][]byte, []string, error) {
	ruleStr, err := sender.ruleReplace(ruleStr)
	if err != nil {
		return nil, nil, utils.Errorf("rule replace error: %v", err)
	}
	rules, err := surirule.Parse(ruleStr)
	if err != nil {
		return nil, nil, utils.Errorf("parse suricate rule error: %v", err)
	}
	var fRule []*rule.Storage
	for _, r := range rules {
		fRule = append(fRule, rule.NewRuleFromSuricata(r))
	}
	mk := chaosmaker.NewChaosMaker()
	mk.FeedRule(fRule...)
	traffics := make([][]byte, 0)
	md5List := make([]string, 0)
	i := 0
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("aaaaaaaaa")
			return
		}
	}()
	for traffic := range mk.Generate() {
		var exTraffic []byte
		var err error
		if i == 1 {
			exTraffic, err = sender.trafficIPAddressReplaceEx(traffic)
			if err != nil {
				return nil, nil, err
			}
		} else {
			exTraffic, err = sender.trafficIPAddressReplace(traffic)
			if err != nil {
				return nil, nil, err
			}
		}
		traffics = append(traffics, exTraffic)
		result, err := core.PacketDataAnalysis(exTraffic)
		if err != nil {
			msg := Message{Message: fmt.Sprintf("packet data analysis error: %v", err), MessageType: infoMessage}
			log.Error(msg.ToJsonStr())
			continue
		}
		if len(result) != 0 {
			md5List = append(md5List, codec.Md5(result))
		}
		if i < 4 {
			i += 1
		} else {
			i = 0
		}
	}
	return traffics, md5List, nil
}

func (sender *Sender) trafficIPAddressReplaceEx(traffic []byte) ([]byte, error) {
	srcBytes, err := core.ParseIPAddressToByte(sender.target)
	dstBytes, _ := core.ParseIPAddressToByte(core.TestIP)
	if err != nil {
		return traffic, utils.Errorf("parse target %v error: %v", sender.target, err)
	}
	for num, b := range srcBytes {
		traffic[core.EthernetLength+core.IPv4SrcStartPos-1+num] = b
	}
	for num, b := range dstBytes {
		traffic[core.EthernetLength+core.IPv4DstStartPos-1+num] = b
	}
	lengthFlag := traffic[core.EthernetLength+core.IPv4LengthCheck-1]
	length := core.CalculateIPv4Length(lengthFlag)
	sum := checksum(traffic[core.EthernetLength : core.EthernetLength+length])
	sum1 := byte(sum >> 8)
	sum2 := byte(sum)
	traffic[core.EthernetLength+core.IPv4CheckSumA-1] = sum1
	traffic[core.EthernetLength+core.IPv4CheckSumB-1] = sum2
	return traffic, nil
}

func (sender *Sender) trafficIPAddressReplace(traffic []byte) ([]byte, error) {
	srcBytes, _ := core.ParseIPAddressToByte(core.TestIP)
	dstBytes, err := core.ParseIPAddressToByte(sender.target)
	if err != nil {
		return traffic, utils.Errorf("parse target %v error: %v", sender.target, err)
	}
	for num, b := range srcBytes {
		traffic[core.EthernetLength+core.IPv4SrcStartPos-1+num] = b
	}
	for num, b := range dstBytes {
		traffic[core.EthernetLength+core.IPv4DstStartPos-1+num] = b
	}
	lengthFlag := traffic[core.EthernetLength+core.IPv4LengthCheck-1]
	length := core.CalculateIPv4Length(lengthFlag)
	sum := checksum(traffic[core.EthernetLength : core.EthernetLength+length])
	sum1 := byte(sum >> 8)
	sum2 := byte(sum)
	traffic[core.EthernetLength+core.IPv4CheckSumA-1] = sum1
	traffic[core.EthernetLength+core.IPv4CheckSumB-1] = sum2
	return traffic, nil
}

func checksum(bytes []byte) uint16 {
	// Clear checksum bytes
	bytes[10] = 0
	bytes[11] = 0

	// Compute checksum
	var csum uint32
	for i := 0; i < len(bytes); i += 2 {
		csum += uint32(bytes[i]) << 8
		csum += uint32(bytes[i+1])
	}
	for {
		// Break when sum is less or equals to 0xFFFF
		if csum <= 65535 {
			break
		}
		// Add carry to the sum
		csum = (csum >> 16) + uint32(uint16(csum))
	}
	// Flip all the bits
	return ^uint16(csum)
}
