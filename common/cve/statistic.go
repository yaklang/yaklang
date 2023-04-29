package cve

import (
	"fmt"
	"yaklang/common/cve/cveresources"
	"yaklang/common/utils"
	"strings"
)

type KVPair struct {
	Key        string `json:"key"`
	Value      int    `json:"value"`
	ShowValue  int    `json:"show_value"`
	KeyVerbose string `json:"key_verbose"`
	Detail     string `json:"detail"`
}

func NewStatistics(source string) *Statistics {
	s := &Statistics{Source: source}
	s.init()
	return s
}

type Statistics struct {
	Source    string
	BySources map[string]*Statistics

	Total int

	// 环形分析器，从内圈到外圈
	NoAuthNetworkHighExploitableCount int
	NoAuthNetworkCount                int
	NetworkCount                      int

	// NETWORK/LOCAL/ADJACENT_NETWORK/PHYSICAL
	CWECounter               map[string]int /* 探明漏洞类型 */
	AccessVectorCounter      map[string]int /* 总体攻击路径的统计 */
	ComplexityCounter        map[string]int /* 攻击复杂度判定 */
	NetworkComplexityCounter map[string]int /* 网络攻击复杂度 */
	LocalComplexityCounter   map[string]int /* 本地攻击复杂度 */
	YearsCounter             map[string]int /* 按年度统计 */
	SeverityCounter          map[string]int /* 按危险程度统计 */
}

func (s *Statistics) init() {

	if s == nil {
		return
	}
	s.AccessVectorCounter = make(map[string]int)
	s.ComplexityCounter = make(map[string]int)
	s.NetworkComplexityCounter = make(map[string]int)
	s.LocalComplexityCounter = make(map[string]int)
	s.YearsCounter = make(map[string]int)
	s.SeverityCounter = make(map[string]int)
	s.CWECounter = make(map[string]int)
}

func (s *Statistics) Feed(c *cveresources.CVE) {
	s.Total++

	yearStr := fmt.Sprint(c.Year())
	_, ok := s.YearsCounter[yearStr]
	if ok {
		s.YearsCounter[yearStr]++
	} else {
		s.YearsCounter[yearStr] = 1
	}

	severity := c.Severity
	if severity == "" {
		severity = "UNKNOWN"
	}
	_, ok = s.SeverityCounter[severity]
	if ok {
		s.SeverityCounter[severity]++
	} else {
		s.SeverityCounter[severity] = 1
	}

	accessVector := c.AccessVector
	if accessVector == "" {
		accessVector = "UNKNOWN"
	}
	var isLocal = strings.ToUpper(accessVector) == "LOCAL"
	var isNetwork = strings.ToUpper(accessVector) == "NETWORK" || strings.ToUpper(accessVector) == "ADJACENT_NETWORK"
	_, ok = s.AccessVectorCounter[accessVector]
	if ok {
		s.AccessVectorCounter[accessVector]++
	} else {
		s.AccessVectorCounter[accessVector] = 1
	}

	complexity := c.AccessComplexity
	if complexity == "" {
		complexity = "UNKNOWN"
	}
	_, ok = s.ComplexityCounter[complexity]
	if ok {
		s.ComplexityCounter[complexity]++
	} else {
		s.ComplexityCounter[complexity] = 1
	}

	if isLocal {
		_, ok = s.LocalComplexityCounter[complexity]
		if ok {
			s.LocalComplexityCounter[complexity]++
		} else {
			s.LocalComplexityCounter[complexity] = 1
		}
	}

	if isNetwork {
		_, ok = s.NetworkComplexityCounter[complexity]
		if ok {
			s.NetworkComplexityCounter[complexity]++
		} else {
			s.NetworkComplexityCounter[complexity] = 1
		}
	}

	noAuth := strings.ToUpper(c.Authentication) == "NONE"
	highExploitable := c.ExploitabilityScore >= 6.0
	if isNetwork {
		s.NetworkCount++
	}
	if isNetwork && noAuth {
		s.NoAuthNetworkCount++
	}
	if isNetwork && noAuth && highExploitable {
		s.NoAuthNetworkHighExploitableCount++
	}

	for _, cwe := range utils.StringArrayFilterEmpty(utils.PrettifyListFromStringSplited(c.CWE, "|")) {
		if cwe == "" {
			cwe = "UNKNOWN"
		}
		_, ok := s.CWECounter[cwe]
		if ok {
			s.CWECounter[cwe]++
		} else {
			s.CWECounter[cwe] = 1
		}
	}
}

func (s *Statistics) FeedSource(source string, c *cveresources.CVE) {
	s.Feed(c)
	if s.Source == source || source == "" {
		return
	}
	if s.BySources == nil {
		s.BySources = map[string]*Statistics{}
	}
	stats, ok := s.BySources[source]
	if !ok {
		stats = &Statistics{Source: source}
		s.BySources[source] = stats
	}

	stats.Feed(c)
}
