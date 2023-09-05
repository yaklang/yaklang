package cve

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"sort"
	"strings"
	"sync"
)

type Graph struct {
	Name            string    `json:"name"`
	NameVerbose     string    `json:"name_verbose"`
	Type            string    `json:"type"`
	TypeVerbose     string    `json:"type_verbose"`
	Data            []*KVPair `json:"data"`
	Reason          string    `json:"reason"` // 如果数据为空的时候，展示原因
	ComplexityGroup string    `json:"complexity_group"`
	AccessVector    string    `json:"access_vector"`
}

func severityVerbose(i string) string {
	switch strings.ToUpper(i) {
	case "MEDIUM":
		return "中危"
	case "HIGH":
		return "高危"
	case "LOW":
		return "低危"
	case "CRITICAL":
		return "严重"
	default:
		return "未知"
	}
}

func complexityVerbose(s string) string {
	switch s {
	case "HIGH":
		return "利用难度高"
	case "LOW":
		return "利用难度低"
	case "MIDDLE":
		return "有一定漏洞利用难度"
	default:
		return "未知/无数据"
	}
}

func accessVectorVerbose(s string) string {
	switch strings.ToUpper(s) {
	case "NETWORK":
		return "网络"
	case "LOCAL":
		return "本地"
	case "ADJACENT_NETWORK":
		return "局域网"
	case "PHYSICAL":
		return "物理介质"
	default:
		return "其他/未知"
	}
}

var (
	cweOnce      = new(sync.Once)
	cweNameMap   = map[string]string{}
	cweSolutions = map[string]string{}
)

func cweSolution(s string) string {
	_ = cweVerbose(s)
	v, ok := cweSolutions[s]
	if ok {
		return v
	} else {
		return ""
	}
}

func cweVerbose(s string) string {
	cweOnce.Do(func() {
		db := consts.GetGormCVEDatabase()
		if db == nil {
			log.Warn("cannot load cwe database")
			return
		}
		for cwe := range cveresources.YieldCWEs(db.Model(&cveresources.CWE{}), context.Background()) {
			cweNameMap[cwe.CWEString()] = cwe.NameZh
			cweSolutions[cwe.CWEString()] = cwe.CWESolution
		}
	})
	s = strings.ToUpper(s)
	if !strings.HasPrefix(s, "CWE-") {
		s = "CWE-" + s
	}
	v, _ := cweNameMap[s]
	if v == "" {
		return s
	}
	return v
}

func (s *Statistics) ToGraphs() []*Graph {
	// 环形分析器
	var graphs []*Graph

	g := &Graph{
		Name:        "AttentionRing",
		NameVerbose: "需被关注的CVE信息",
		Type:        "multi-pie",
		TypeVerbose: "多层饼环",
		Data: []*KVPair{
			{
				Key:        "NoAuthNetworkHighExploitable",
				Value:      s.NoAuthNetworkHighExploitableCount,
				ShowValue:  s.NoAuthNetworkHighExploitableCount,
				KeyVerbose: "通过网络无需认证且易于攻击",
			},
			{
				Key:        "NoAuthNetwork",
				Value:      s.NoAuthNetworkCount,
				ShowValue:  s.NoAuthNetworkCount,
				KeyVerbose: "攻击通过网络无需认证",
			},
			{
				Key:        "NetworkCount",
				Value:      s.NetworkCount,
				ShowValue:  s.NetworkCount,
				KeyVerbose: "通过网络攻击",
			},
		},
	}
	graphs = append(graphs, g)

	// CWE
	var pairs []*KVPair
	for cweName, i := range s.CWECounter {
		pairs = append(pairs, &KVPair{
			Key:        cweName,
			Value:      i,
			ShowValue:  i,
			KeyVerbose: cweVerbose(cweName),
			Detail:     cweSolution(cweName),
		})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[i].Value
	})
	g = &Graph{
		Name:        "cwe-analysis",
		NameVerbose: "合规漏洞类型大致分布",
		Type:        "nightingle-rose",
		TypeVerbose: "南丁格尔玫瑰图",
		Data:        pairs,
	}
	graphs = append(graphs, g)

	// accessVector
	pairs = []*KVPair{}
	for access, count := range s.AccessVectorCounter {
		pairs = append(pairs, &KVPair{
			Key:        access,
			Value:      count,
			ShowValue:  count,
			KeyVerbose: accessVectorVerbose(access),
		})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[i].Value
	})
	g = &Graph{
		Name:        "access-vector-analysis",
		NameVerbose: "合规漏洞攻击路径统计",
		Type:        "card",
		TypeVerbose: "通用KV",
		Data:        pairs,
	}
	graphs = append(graphs, g)

	// 网络攻击复杂度
	pairs = []*KVPair{}
	for key, count := range s.NetworkComplexityCounter {
		pairs = append(pairs, &KVPair{
			Key:        key,
			Value:      count,
			ShowValue:  count,
			KeyVerbose: complexityVerbose(key),
			JumpLink:   key,
		})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[i].Value
	})
	g = &Graph{
		Name:            "network-attack-complexity-analysis",
		NameVerbose:     "合规漏洞利用复杂度统计(可联网攻击)",
		Type:            "general",
		TypeVerbose:     "通用KV",
		Data:            pairs,
		AccessVector:    "NETWORK",
		ComplexityGroup: strings.Join([]string{"未知/无数据", "利用难度低", "利用难度高"}, ","),
	}
	graphs = append(graphs, g)

	// 本地攻击复杂度
	pairs = []*KVPair{}
	for key, count := range s.LocalComplexityCounter {
		pairs = append(pairs, &KVPair{
			Key:        key,
			Value:      count,
			ShowValue:  count,
			KeyVerbose: complexityVerbose(key),
			JumpLink:   key,
		})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[i].Value
	})
	g = &Graph{
		Name:            "local-attack-complexity-analysis",
		NameVerbose:     "合规漏洞利用复杂度统计(仅本地攻击)",
		Type:            "general",
		TypeVerbose:     "通用KV",
		Data:            pairs,
		AccessVector:    "LOCAL",
		ComplexityGroup: strings.Join([]string{"未知/无数据", "利用难度低", "利用难度高"}, ","),
	}
	graphs = append(graphs, g)

	// 攻击复杂度
	pairs = []*KVPair{}
	for key, count := range s.ComplexityCounter {
		pairs = append(pairs, &KVPair{
			Key:        key,
			Value:      count,
			ShowValue:  count,
			KeyVerbose: complexityVerbose(key),
			JumpLink:   key,
		})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[i].Value
	})
	g = &Graph{
		Name:            "access-complexity-analysis",
		NameVerbose:     "合规漏洞利用复杂度统计",
		Type:            "general",
		TypeVerbose:     "通用KV",
		Data:            pairs,
		ComplexityGroup: strings.Join([]string{"未知/无数据", "利用难度低", "利用难度高"}, ","),
	}
	graphs = append(graphs, g)

	// 严重程度
	/*pairs = []*KVPair{}
	for key, count := range s.SeverityCounter {
		pairs = append(pairs, &KVPair{
			Key:        key,
			Value:      count,
			ShowValue:  count,
			KeyVerbose: severityVerbose(key),
		})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[i].Value
	})
	g = &Graph{
		Name:            "severtiy-analysis",
		NameVerbose:     "合规漏洞严重程度统计",
		Type:            "general",
		TypeVerbose:     "通用KV",
		Data:            pairs,
		ComplexityGroup: strings.Join([]string{"严重", "高危", "中危", "低危"}, ","),
	}
	graphs = append(graphs, g)*/

	// Years
	pairs = []*KVPair{}
	for key, count := range s.YearsSeverityCounter {
		var a []*KVPair
		yearsCount := 0
		for k, v := range count {
			a = append(a, &KVPair{
				Key:       k,
				Value:     v,
				ShowValue: v,
			})
			yearsCount = yearsCount + v
		}
		pairs = append(pairs, &KVPair{
			Key:        key,
			Value:      yearsCount,
			ShowValue:  yearsCount,
			KeyVerbose: "CVE-" + fmt.Sprint(key),
			Data:       a,
		})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[i].Value
	})
	g = &Graph{
		Name:        "years-analysis",
		NameVerbose: "CVE 年份统计",
		Type:        "year-cve",
		TypeVerbose: "通用KV",
		Data:        pairs,
	}
	graphs = append(graphs, g)

	return graphs
}
