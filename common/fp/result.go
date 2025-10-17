package fp

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
	tablewriter "github.com/olekukonko/tablewriter"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type FingerprintInfo struct {
	IP                string                   `json:"ip"`
	Port              int                      `json:"port"`
	Proto             TransportProto           `json:"proto"`
	ServiceName       string                   `json:"service_name"`
	ProductVerbose    string                   `json:"product_verbose"`
	Info              string                   `json:"info"`
	Version           string                   `json:"version"`
	Hostname          string                   `json:"hostname"`
	OperationVerbose  string                   `json:"operation_verbose"`
	DeviceType        string                   `json:"device_type"`
	CPEs              []string                 `json:"cpes"`
	Raw               string                   `json:"raw"`
	Banner            string                   `json:"banner"`
	CPEFromUrls       map[string][]*schema.CPE `json:"cpe_from_urls"`
	HttpFlows         []*HTTPFlow              `json:"http_flows"`
	CheckedTLS        bool                     `json:"checked_tls"`
	TLSInspectResults []*netx.TLSInspectResult `json:"tls_inspect_results"`
}

type HTTPFlow struct {
	StatusCode     int           `json:"status_code"`
	IsHTTPS        bool          `json:"is_https"`
	RequestHeader  []byte        `json:"request_header"`
	RequestBody    []byte        `json:"request_body"`
	ResponseHeader []byte        `json:"response_header"`
	ResponseBody   []byte        `json:"response_body"`
	CPEs           []*schema.CPE `json:"cp_es"`
}

func (f *FingerprintInfo) FromRegexp2Match(match *regexp2.Match) {
	do := func(raw string) string {
		return parseNmapStringByRegexp2Match(raw, match)
	}

	f.ProductVerbose = do(f.ProductVerbose)
	f.Info = do(f.Info)
	f.Version = do(f.Version)
	f.Hostname = do(f.Hostname)
	f.OperationVerbose = do(f.OperationVerbose)
	f.DeviceType = do(f.DeviceType)

	var cpes []string
	var version string
	for _, c := range f.CPEs {
		cpe := do(c)
		part, err := webfingerprint.ParseToCPE(cpe)
		if err != nil {
			continue
		}
		if part.Version != "*" {
			version = part.Version
		}
		cpes = append(cpes, cpe)
	}
	if f.Version == "" {
		f.Version = version
	}
	f.CPEs = cpes
}

func ToFingerprintInfo(rule *NmapMatch, match *regexp2.Match) *FingerprintInfo {
	info := &FingerprintInfo{
		ServiceName:      rule.ServiceName,
		ProductVerbose:   rule.ProductVerbose,
		Info:             rule.Info,
		Version:          rule.Version,
		Hostname:         rule.Hostname,
		OperationVerbose: rule.OperationVerbose,
		DeviceType:       rule.DeviceType,
		CPEs:             rule.CPEs,
		Raw:              rule.Raw,
	}

	info.FromRegexp2Match(match)

	return info
}

var (
	extractNmapDollarNumberExpr = regexp.MustCompile(`\$([0-9]+)`)
	extractNmapDollarPExpr      = regexp2.MustCompile(`\$P\(([0-9]+)\)`, regexp2.Multiline)
	extractNmapDollarSUBSTExpr  = regexp2.MustCompile(`\$SUBST\( *([0-9]+) *, *"([^"]*?)" *, *"([^"]*?)" *\)`, regexp2.Multiline)
	extractNmapDollarIExpr      = regexp2.MustCompile(`\$I\( *(\d+) *, *"([^"]*)" *\)`, regexp2.Multiline)
)

func parseNmapStringByRegexp2Match(raw string, rawMatch *regexp2.Match) string {
	if !strings.Contains(raw, "$") {
		return raw
	}

	// handle $n
	raw = extractNmapDollarNumberExpr.ReplaceAllStringFunc(raw, func(s string) string {
		indexStr := s[1:]
		index, err := strconv.ParseInt(indexStr, 10, 32)
		if err != nil {
			return s
		}

		if rawMatch == nil {
			return s
		}

		if int(index) < rawMatch.GroupCount() {
			return rawMatch.GroupByNumber(int(index)).String()
		}
		return s
	})

	// handle $P
	replacedByP, err := extractNmapDollarPExpr.ReplaceFunc(raw, func(match regexp2.Match) string {
		indexGroup := match.GroupByNumber(1)
		index, err := strconv.ParseInt(indexGroup.String(), 10, 32)
		if err != nil {
			return match.String()
		}

		if int(index) <= rawMatch.GroupCount() {
			result := rawMatch.GroupByNumber(int(index)).String()
			result = utils.RemoveUnprintableChars(result)
			return result
		}

		return match.String()
	}, -1, -1)
	if err != nil {
		replacedByP = raw
	}

	raw = replacedByP

	// handle $SUBST
	replacedBySUBST, err := extractNmapDollarSUBSTExpr.ReplaceFunc(raw, func(match regexp2.Match) string {
		indexGroup := match.GroupByNumber(1)
		index, err := strconv.ParseInt(indexGroup.String(), 10, 32)
		if err != nil {
			return match.String()
		}

		if int(index) <= rawMatch.GroupCount() {
			// get correct material
			result := rawMatch.GroupByNumber(int(index)).String()
			return strings.ReplaceAll(
				result,
				match.GroupByNumber(2).String(),
				match.GroupByNumber(3).String(),
			)
		}

		return match.String()
	}, -1, -1)
	if err != nil {
		replacedBySUBST = raw
	}
	raw = replacedBySUBST

	// handle $I
	replacedByI, err := extractNmapDollarIExpr.ReplaceFunc(raw, func(match regexp2.Match) string {
		// > for big endian
		// < for little endian
		indexGroup := match.GroupByNumber(1)
		index, err := strconv.ParseInt(indexGroup.String(), 10, 32)
		if err != nil {
			return match.String()
		}

		if int(index) > rawMatch.GroupCount() {
			return match.String()
		}

		data := []byte(rawMatch.GroupByNumber(int(index)).String())
		if len(data) > 8 {
			return match.String()
		}

		var dataArray [8]byte
		copy(dataArray[:], data[:8])

		var ret interface{}
		switch match.GroupByNumber(2).String() {
		case ">":
			if len(data) <= 2 {
				ret = binary.BigEndian.Uint16(dataArray[:])
			} else if len(data) <= 4 && len(data) > 2 {
				ret = binary.BigEndian.Uint32(dataArray[:])
			} else {
				ret = binary.BigEndian.Uint64(dataArray[:])
			}
		case "<":
			if len(data) <= 2 {
				ret = binary.LittleEndian.Uint16(dataArray[:])
			} else if len(data) <= 4 && len(data) > 2 {
				ret = binary.LittleEndian.Uint32(dataArray[:])
			} else {
				ret = binary.LittleEndian.Uint64(dataArray[:])
			}
		default:
			return match.String()
		}
		return fmt.Sprint(ret)
	}, -1, -1)
	if err != nil {
		replacedByI = raw
	}

	raw = replacedByI
	return raw
}

type MatcherResultAnalysis struct {
	TotalScannedPort         int                 `json:"total_scaned_port"`
	TotalOpenPort            int                 `json:"total_open_port"`
	TargetOpenPortCountMap   map[string]int      `json:"target_open_port_count_map"`
	TargetClosedPortCountMap map[string]int      `json:"target_closed_port_count_map"`
	ClosedPort               []string            `json:"closed_port"`
	OpenPortCPEMap           map[string][]string `json:"open_port_cpe_map"`
	OpenPortServiceMap       map[string]string   `json:"open_port_service_map"`
}

func (s *MatcherResultAnalysis) Show() {
	var tw *tablewriter.Table
	tw = tablewriter.NewWriter(os.Stdout)
	tw.Header([]string{"主机", "开放端口数"})
	for host, port := range s.TargetOpenPortCountMap {
		tw.Append([]string{host, fmt.Sprint(port)})
	}
	tw.Render()
	println()

	tw = tablewriter.NewWriter(os.Stdout)
	tw.Header([]string{"端口", "指纹（简）"})
	for port, service := range s.OpenPortServiceMap {
		tw.Append([]string{port, fmt.Sprint(service)})
	}
	tw.Render()
	println()

	tw = tablewriter.NewWriter(os.Stdout)
	tw.Append([][]string{
		{"扫描总数", fmt.Sprint(s.TotalScannedPort)},
		{"开放端口", fmt.Sprint(s.TotalOpenPort)},
		{"关闭端口", fmt.Sprint(len(s.ClosedPort))},
	})
	tw.Render()
}

func (s *MatcherResultAnalysis) ToJson(file string) {
	if file == "" {
		return
	}

	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		log.Errorf("marshal json failed: %s", err)
		return
	}

	err = ioutil.WriteFile(file, raw, os.ModePerm)
	if err != nil {
		log.Errorf("output to json[%s] failed: %s", file, err)
		return
	}
}

func MatcherResultsToAnalysis(res []*MatchResult) *MatcherResultAnalysis {
	result := &MatcherResultAnalysis{
		TotalScannedPort:         0,
		TotalOpenPort:            0,
		TargetOpenPortCountMap:   make(map[string]int),
		TargetClosedPortCountMap: make(map[string]int),
		OpenPortCPEMap:           make(map[string][]string),
		OpenPortServiceMap:       make(map[string]string),
	}

	for _, r := range res {
		result.TotalScannedPort++

		var portTag string
		if r.Fingerprint != nil {
			if len(r.Fingerprint.HttpFlows) > 0 || strings.HasPrefix(r.GetBanner(), "HTTP/") {
				var schema = "http"
				if len(r.Fingerprint.HttpFlows) > 0 && r.Fingerprint.HttpFlows[0].IsHTTPS {
					schema = "https"
				}
				portTag = fmt.Sprintf("%s://%s", schema, utils.HostPort(r.Fingerprint.IP, r.Fingerprint.Port))
			} else {
				schema := r.Fingerprint.ServiceName
				if schema == "" {
					schema = string(r.Fingerprint.Proto)
				}
				portTag = fmt.Sprintf("%s://%s", schema, utils.HostPort(r.Fingerprint.IP, r.Fingerprint.Port))
			}
		} else {
			portTag = fmt.Sprintf("%s/%v", utils.HostPort(r.Target, r.Port), r.Reason)
		}

		if r.State != OPEN {
			_, ok := result.TargetClosedPortCountMap[r.Target]
			if !ok {
				result.TargetClosedPortCountMap[r.Target] = 1
			} else {
				result.TargetClosedPortCountMap[r.Target]++
			}
			result.ClosedPort = append(result.ClosedPort, portTag)
			continue
		}

		if r.Fingerprint == nil {
			continue
		}

		result.TotalOpenPort++
		_, ok := result.TargetOpenPortCountMap[r.Target]
		if !ok {
			result.TargetOpenPortCountMap[r.Target] = 1
		} else {
			result.TargetOpenPortCountMap[r.Target]++
		}

		result.OpenPortCPEMap[portTag] = r.GetCPEs()
		result.OpenPortServiceMap[portTag] = strings.ReplaceAll(r.GetServiceName(), "[*]", "")
	}

	return result
}
