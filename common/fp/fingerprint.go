package fp

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/fp/fingerprint"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type RuleBlock struct {
	Probe   *NmapProbe
	Matched []*NmapMatch
}

type Matcher struct {
	Config  *Config
	matcher *fingerprint.Matcher
}

type PortState string

func (p *PortState) String() string {
	return string(*p)
}

var (
	OPEN    PortState = "open"
	CLOSED  PortState = "closed"
	UNKNOWN PortState = "unknown"
)

type ScanTarget struct {
	Host  string
	Port  int
	Proto TransportProto
}

type MatchResult struct {
	Target      string           `json:"target"`
	Port        int              `json:"port"`
	State       PortState        `json:"state"`
	Reason      string           `json:"reason"`
	Fingerprint *FingerprintInfo `json:"fingerprint"`
}

func (m *MatchResult) Identifier() string {
	return utils.CalcSha256(fmt.Sprintf("%v://%v:%v", m.GetProto(), m.Target, m.Port))
}

func (m *MatchResult) Tidy() {
	if m == nil {
		return
	}
	if m.Fingerprint == nil {
		return
	}

	if m.Fingerprint.Proto == "" {
		m.Fingerprint.Proto = TCP
	}

	m.Fingerprint.ServiceName = strings.Join(
		utils2.RemoveRepeatedWithStringSlice(strings.Split(m.Fingerprint.ServiceName, "/")),
		"/",
	)
	if m.Fingerprint.Banner == "" && len(m.Fingerprint.HttpFlows) > 0 {
		m.Fingerprint.Banner = utils2.EscapeInvalidUTF8Byte(lowhttp.ReplaceHTTPPacketBody(m.Fingerprint.HttpFlows[0].ResponseHeader, m.Fingerprint.HttpFlows[0].ResponseBody, false))
	}
}

func (m *MatchResult) Merge(f *MatchResult) {
	if m == nil || f == nil {
		return
	}

	if m.State != OPEN && f.State == OPEN {
		m.State = OPEN
	}

	if !(m.Target == f.Target && m.Port == f.Port) {
		return
	}

	if m.Fingerprint == nil {
		m.Fingerprint = f.Fingerprint
		return
	}

	if f.Fingerprint == nil {
		return
	}
	m.Fingerprint.CPEs = utils2.RemoveRepeatedWithStringSlice(append(m.Fingerprint.CPEs, f.Fingerprint.CPEs...))
	if f.Fingerprint.ServiceName != "" && m.Fingerprint.ServiceName == "" {
		m.Fingerprint.Banner = f.Fingerprint.Banner
		m.Fingerprint.Raw = f.Fingerprint.Raw
	}
	if m.Fingerprint.ServiceName != f.Fingerprint.ServiceName {
		f.Fingerprint.ServiceName = strings.Trim(f.Fingerprint.ServiceName, "/")
		m.Fingerprint.ServiceName = fmt.Sprintf("%v/%v", m.Fingerprint.ServiceName, f.Fingerprint.ServiceName)
	}
	m.Fingerprint.ServiceName = strings.Trim(m.Fingerprint.ServiceName, "/")

	m.Fingerprint.HttpFlows = append(m.Fingerprint.HttpFlows, f.Fingerprint.HttpFlows...)
	if f.Fingerprint.CPEFromUrls != nil && m.Fingerprint.CPEFromUrls != nil {
		for k, v := range f.Fingerprint.CPEFromUrls {
			_, ok := m.Fingerprint.CPEFromUrls[k]
			if ok {
				m.Fingerprint.CPEFromUrls[k] = append(m.Fingerprint.CPEFromUrls[k], v...)
			} else {
				m.Fingerprint.CPEFromUrls[k] = v
			}
		}
	}
	m.Fingerprint.TLSInspectResults = append(m.Fingerprint.TLSInspectResults, f.Fingerprint.TLSInspectResults...)
	m.Fingerprint.CheckedTLS = m.Fingerprint.CheckedTLS || f.Fingerprint.CheckedTLS
}

func (m *MatchResult) HaveDialFailed() bool {
	return utils2.MatchAnyOfGlob(m.Reason,
		"*dial tcp*: i/o timeout",
		"*dial tcp* connect: connection refused",
		"*connection reset by peer",
	)
}

func (m *MatchResult) IsOpen() bool {
	if m == nil {
		return false
	}
	return m.State == OPEN
}

func (m *MatchResult) String(schemaForce ...string) string {
	schema := "tcp"
	if m.Fingerprint != nil {
		schema = strings.ToLower(string(m.GetProto()))
	}
	if len(schemaForce) > 0 {
		schema = strings.Join(schemaForce, "+")
	}
	if schema == "" {
		schema = string(m.GetProto())
	}
	return fmt.Sprintf("%v://%v\t%5s\t%s", schema, utils2.HostPort(m.Target, m.Port), m.State, m.GetServiceName())
}

func (m *MatchResult) GetCPEs() []string {
	if m.Fingerprint == nil {
		return nil
	}
	return m.Fingerprint.CPEs
}

func (m *MatchResult) GetBanner() string {
	if m.Fingerprint == nil {
		return ""
	}

	if _, err := strconv.Unquote(m.Fingerprint.Banner); err != nil {
		return strconv.Quote(m.Fingerprint.Banner)
	} else {
		return m.Fingerprint.Banner
	}
}

func (m *MatchResult) GetHtmlTitle() string {
	if m.Fingerprint == nil {
		return ""
	}

	var titles []string
	for _, f := range m.Fingerprint.HttpFlows {
		res := utils2.ExtractTitleFromHTMLTitle(string(f.ResponseBody), "")
		if res != "" {
			titles = append(titles, res)
		}
	}
	return strings.Join(titles, "|")
}

func (m *MatchResult) GetServiceName() string {
	return m.getServiceName()
}

func (m *MatchResult) GetResponseRaw() []byte {
	if m.Fingerprint == nil {
		return nil
	}
	if len(m.Fingerprint.HttpFlows) > 0 {
		flow, err := lo.Last(m.Fingerprint.HttpFlows)
		if err != nil {
			return nil
		}
		return lowhttp.ReplaceHTTPPacketBodyFast(flow.ResponseHeader, flow.ResponseBody)
	}

	return nil
}

func (m *MatchResult) GetRequestRaw() (bool, []byte) {
	if m.Fingerprint == nil {
		return false, nil
	}
	if len(m.Fingerprint.HttpFlows) > 0 {
		flow, err := lo.Last(m.Fingerprint.HttpFlows)
		if err != nil {
			return false, nil
		}
		return flow.IsHTTPS, lowhttp.ReplaceHTTPPacketBodyFast(flow.RequestHeader, flow.RequestBody)
	}

	return false, nil
}

func (m *MatchResult) GetFuzzRequest() *mutate.FuzzHTTPRequest {
	urls := utils2.ParseStringToUrls(m.Target)
	if !m.IsOpen() && len(urls) > 0 {
		https, reqs, err := lowhttp.ParseUrlToHttpRequestRaw("GET", urls[0])
		if err != nil {
			return nil
		}
		freq, _ := mutate.NewFuzzHTTPRequest(reqs, mutate.OptHTTPS(https))
		if freq != nil {
			return freq
		}
		return nil
	}

	var freq *mutate.FuzzHTTPRequest
	var err error
	https, reqBytes := m.GetRequestRaw()
	if reqBytes != nil {
		freq, _ = mutate.NewFuzzHTTPRequest(reqBytes, mutate.OptHTTPS(https))
		if freq != nil {
			return freq
		}
	}

	var targetUrl string
	if netx.IsTLSService(m.Target) {
		targetUrl = fmt.Sprintf("https://%v", m.Target)
	} else {
		targetUrl = fmt.Sprintf("http://%v", m.Target)
	}
	https, reqs, err := lowhttp.ParseUrlToHttpRequestRaw("GET", targetUrl)
	if err != nil {
		return nil
	}
	freq, _ = mutate.NewFuzzHTTPRequest(reqs, mutate.OptHTTPS(https))
	if freq != nil {
		return freq
	}
	return nil
}

func (m *MatchResult) getServiceName() string {
	if m == nil {
		return ""
	}

	if m.Fingerprint == nil {
		return ""
	}

	productsVer := make(map[string]string)
	var products []string
	for _, cpe := range m.Fingerprint.CPEs {
		cpeInstance, err := webfingerprint.ParseToCPE(cpe)
		if err != nil {
			continue
		}

		product := cpeInstance.Product
		if product == "*" {
			product = m.Fingerprint.ServiceName
		}
		version := cpeInstance.Version
		if version == "*" {
			version = ""
		}
		if versionVer, ok := productsVer[product]; ok || versionVer == "" {
			productsVer[product] = version
		}
	}

	for k, v := range productsVer {
		if v == "" {
			products = append(products, k)
		} else {
			products = append(products, fmt.Sprintf("%v[%v]", k, v))
		}
	}
	sn := m.Fingerprint.ServiceName
	if sn != "" {
		if aliasSn, ok := FingerprintAlias[sn]; ok {
			sn = aliasSn
		}
		products = append(products, sn)
	}
	products = utils2.RemoveRepeatStringSlice(products)
	sort.Strings(products)

	if len(products) <= 0 {
		var fixDefaultSchema string
		if m.GetProto() == UDP {
			fixDefaultSchema = GetDefaultUDPServiceName(m.Port)
		} else {
			fixDefaultSchema = GetDefaultTCPServiceName(m.Port)
		}
		products = append(products, fixDefaultSchema)
	}

	result := strings.Join(products, "/")
	if result != "" {
		return result
	}

	return m.Fingerprint.ServiceName
}

func (m *MatchResult) GetProto() TransportProto {
	if m.Fingerprint == nil {
		return TCP
	}
	return m.Fingerprint.Proto
}

func (m *MatchResult) GetDomains() []string {
	if ret := net.ParseIP(utils2.FixForParseIP(m.Target)); ret == nil {
		return []string{m.Target}
	} else {
		return []string{}
	}
}

var FpMatchResultTTLCache = utils.NewTTLCache[*MatchResult](3 * time.Minute)

func SetMatchResultCache(addr string, result *MatchResult) {
	FpMatchResultTTLCache.SetWithTTL(addr, result, 3*time.Minute)
}

func databaseCacheKey(addr string) string {
	return fmt.Sprintf("DatabaseCache_MatchResult_%v", addr)
}

func SetMatchResultDatabaseCache(addr string, result *MatchResult) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		log.Error("gorm database failed: empty")
		return
	}
	addr = databaseCacheKey(addr)
	raw, err := json.Marshal(result)
	if err != nil {
		log.Errorf("json marshal matchResult: %s", err)
		return
	}
	err = yakit.SetKeyWithTTL(db, addr, strconv.Quote(string(raw)), 600)
	if err != nil {
		log.Errorf("set key with ttl failed: %s", err)
	}
}

func GetMatchResultDatabaseCache(addr string) *MatchResult {
	originAddr := addr
	db := consts.GetGormProfileDatabase()
	if db == nil {
		log.Error("gorm database failed: empty")
		return nil
	}
	addr = databaseCacheKey(addr)

	raw := yakit.GetKey(db, addr)
	if raw == "" {
		return nil
	}

	rawBytes, err := strconv.Unquote(raw)
	if err != nil {
		return nil
	}

	var m MatchResult
	err = json.Unmarshal([]byte(rawBytes), &m)
	if err != nil {
		return nil
	}
	SetMatchResultDatabaseCache(originAddr, &m)
	return &m
}

func GetMatchResultCache(addr string) *MatchResult {
	result, ok := FpMatchResultTTLCache.Get(addr)
	if !ok {
		return nil
	}

	SetMatchResultCache(addr, result)
	return result
}

func NewFingerprintMatcher(rules map[*NmapProbe][]*NmapMatch, config *Config) (*Matcher, error) {
	if config == nil {
		log.Debugf("build fingerprint match without config, use default")
		config = NewConfig()
	}

	matcher := &Matcher{
		Config:  config,
		matcher: fingerprint.NewMatcher(),
	}

	return matcher, nil
}

func NewDefaultFingerprintMatcher(config *Config) (*Matcher, error) {
	return NewFingerprintMatcher(nil, config)
}
