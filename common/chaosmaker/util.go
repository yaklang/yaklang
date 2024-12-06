package chaosmaker

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
)

func ParseRuleFromRawSuricataRules(content string) []*rule.Storage {

	var rules []*rule.Storage
	for line := range utils.ParseLines(content) {
		log.Infof("start to handle line: %v", line)
		raw, err := surirule.Parse(line)
		if err != nil {
			log.Errorf("cannot parse suricata raw rules: %s", err)
			continue
		}
		for _, r := range raw {
			rules = append(rules, rule.NewRuleFromSuricata(r))
		}
	}

	return rules
}

func ParseRuleFromHTTPRequestRawJSON(content string) []*rule.Storage {
	var rules []*rule.Storage
	for i := range utils.ParseLines(content) {
		var r = map[string]string{}
		err := json.Unmarshal([]byte(i), &r)
		if err != nil {
			log.Error(err)
			continue
		}
		if ret, _ := r["request_base64"]; ret == "" {
			spew.Dump(r)
			continue
		} else {
			raw, _ := codec.DecodeBase64(ret)
			_ = raw
			title, _ := r["title"]
			db := consts.GetGormProfileDatabase()
			if db != nil {
				rules = append(rules, rule.NewHTTPRequestRule(title, raw))
			} else {
				log.Error("database empty")
			}
		}
	}
	return rules
}
func CompleteTCPFlow(raw []byte, mtu int) [][]byte {
	var flow [][]byte
	var err error
	if len(raw) <= mtu {
		flow, err = pcapx.CompleteTCPFlow(raw)
	} else {
		// 分片, 如果需要的话
		pk := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default)
		if pk == nil {
			return nil
		}
		nw := pk.NetworkLayer()
		if nw == nil {
			return nil
		}
		tcp := pk.TransportLayer()
		if tcp == nil {
			return nil
		}
		payload := tcp.LayerPayload()
		flow, err = pcapx.CreateTCPFlowFromPayload(
			net.JoinHostPort(nw.NetworkFlow().Src().String(), tcp.TransportFlow().Src().String()),
			net.JoinHostPort(nw.NetworkFlow().Dst().String(), tcp.TransportFlow().Dst().String()),
			payload,
		)
	}
	if err != nil {
		log.Errorf("complete tcp flow failed: %v", err)
		return nil
	}
	return flow
}
