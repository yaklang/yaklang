package yaklib

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"yaklang/common/domainextractor"
	"yaklang/common/filter"
	"yaklang/common/go-funk"
	"yaklang/common/jsonextractor"
	"yaklang/common/utils"
	"yaklang/common/utils/lowhttp"
	"yaklang/common/utils/suspect"
)

func _strJoin(i interface{}, d interface{}) (defaultResult string) {
	var s = utils.InterfaceToString(d)
	defaultResult = utils.InterfaceToString(i)
	defer func() {
		recover()
	}()
	defaultResult = strings.Join(funk.Map(i, func(element interface{}) string {
		return utils.InterfaceToString(element)
	}).([]string), s)
	return
}

var (
	StringsExport = map[string]interface{}{
		// 基础字符串工具
		"IndexAny":       strings.IndexAny,
		"StartsWith":     strings.HasPrefix,
		"EndsWith":       strings.HasSuffix,
		"Title":          strings.Title,
		"Join":           _strJoin,
		"TrimLeft":       strings.TrimLeft,
		"TrimPrefix":     strings.TrimPrefix,
		"TrimRight":      strings.TrimRight,
		"TrimSuffix":     strings.TrimSuffix,
		"Trim":           strings.Trim,
		"Split":          strings.Split,
		"SplitAfter":     strings.SplitAfter,
		"SplitAfterN":    strings.SplitAfterN,
		"SplitN":         strings.SplitN,
		"ToLower":        strings.ToLower,
		"ToUpper":        strings.ToUpper,
		"HasPrefix":      strings.HasPrefix,
		"HasSuffix":      strings.HasSuffix,
		"Repeat":         strings.Repeat,
		"ToTitleSpecial": strings.ToTitleSpecial,
		"ToTitle":        strings.ToTitle,
		"Contains":       strings.Contains,
		"ReplaceAll":     strings.ReplaceAll,
		"Replace":        strings.Replace,
		"NewReader":      strings.NewReader,
		"Index":          strings.Index,
		"Count":          strings.Count,
		"Compare":        strings.Compare,
		"ContainsAny":    strings.ContainsAny,
		"EqualFold":      strings.EqualFold,
		"Fields":         strings.Fields,
		"IndexByte":      strings.IndexByte,
		"LastIndex":      strings.LastIndex,
		"LastIndexAny":   strings.LastIndexAny,
		"LastIndexByte":  strings.LastIndexByte,
		"ToLowerSpecial": strings.ToLowerSpecial,
		"ToUpperSpecial": strings.ToUpperSpecial,
		"ToValidUTF8":    strings.ToValidUTF8,

		// 特有的
		"RandStr":                utils.RandStringBytes,
		"f":                      _sfmt,
		"SplitAndTrim":           utils.PrettifyListFromStringSplited,
		"StringSliceContains":    utils.StringSliceContain,
		"StringSliceContainsAll": utils.StringSliceContainsAll,
		"RemoveRepeat":           utils.RemoveRepeatStringSlice,
		"RandSecret":             utils.RandSecret,
		"IsStrongPassword":       utils.IsStrongPassword,
		"ExtractStrContext":      utils.ExtractStrContextByKeyword,

		// 支持 url、host:port 的解析成 Host Port
		"CalcSimilarity":                 utils.CalcSimilarity,
		"CalcTextMaxSubStrStability":     utils.CalcTextSubStringStability,
		"CalcSSDeepStability":            utils.CalcSSDeepStability,
		"CalcSimHashStability":           utils.CalcSimHashStability,
		"CalcSimHash":                    utils.SimHash,
		"CalcSSDeep":                     utils.SSDeepHash,
		"ParseStringToHostPort":          utils.ParseStringToHostPort,
		"IsIPv6":                         utils.IsIPv6,
		"IsIPv4":                         utils.IsIPv4,
		"StringContainsAnyOfSubString":   utils.StringContainsAnyOfSubString,
		"ExtractHost":                    utils.ExtractHost,
		"ExtractDomain":                  extractDomain,
		"ExtractRootDomain":              extractRootDomain,
		"ExtractJson":                    extractValidJson,
		"ExtractJsonWithRaw":             extractJsonEx,
		"LowerAndTrimSpace":              utils.StringLowerAndTrimSpace,
		"HostPort":                       utils.HostPort,
		"ParseStringToHTTPRequest":       lowhttp.ParseStringToHttpRequest,
		"SplitHostsToPrivateAndPublic":   utils.SplitHostsToPrivateAndPublic,
		"ParseBytesToHTTPRequest":        lowhttp.ParseBytesToHttpRequest,
		"ParseStringToHTTPResponse":      lowhttp.ParseStringToHTTPResponse,
		"ParseBytesToHTTPResponse":       lowhttp.ParseBytesToHTTPResponse,
		"FixHTTPResponse":                lowhttp.FixHTTPResponse,
		"ExtractBodyFromHTTPResponseRaw": lowhttp.ExtractBodyFromHTTPResponseRaw,
		"FixHTTPRequest":                 lowhttp.FixHTTPRequestOut,
		"ExtractURLFromHTTPRequestRaw":   lowhttp.ExtractURLFromHTTPRequestRaw,
		"ExtractURLFromHTTPRequest":      lowhttp.ExtractURLFromHTTPRequest,
		"ExtractTitle": func(i interface{}) string {
			return utils.ExtractTitleFromHTMLTitle(utils.InterfaceToString(i), "")
		},
		"SplitHTTPHeadersAndBodyFromPacket": lowhttp.SplitHTTPHeadersAndBodyFromPacket,
		"MergeUrlFromHTTPRequest":           lowhttp.MergeUrlFromHTTPRequest,
		"ReplaceHTTPPacketBody":             lowhttp.ReplaceHTTPPacketBody,

		"ParseStringToHosts":              utils.ParseStringToHosts,
		"ParseStringToPorts":              utils.ParseStringToPorts,
		"ParseStringToUrls":               utils.ParseStringToUrls,
		"ParseStringToUrlsWith3W":         utils.ParseStringToUrlsWith3W,
		"ParseStringToCClassHosts":        utils.ParseStringToCClassHosts,
		"ParseStringUrlToWebsiteRootPath": utils.ParseStringUrlToWebsiteRootPath,
		"ParseStringUrlToUrlInstance":     utils.ParseStringUrlToUrlInstance,
		"UrlJoin":                         utils.UrlJoin,
		"IPv4ToCClassNetwork":             utils.GetCClassByIPv4,
		"ParseStringToLines":              utils.ParseStringToLines,
		"PathJoin":                        filepath.Join,
		"Grok":                            Grok,
		"JsonToMapList":                   JsonToMapList,
		"JsonStreamToMapList":             JsonStreamToMapList,
		"JsonToMap":                       JsonToMap,
		"ParamsGetOr":                     ParamsGetOr,
		"ToJsonIndentStr": func(d interface{}) string {
			raw, err := json.MarshalIndent(d, "", "    ")
			if err != nil {
				return ""
			}
			return string(raw)
		},
		"TrimSpace": strings.TrimSpace,

		"NewFilter": filter.NewFilter,

		"RegexpMatch": reMatch,

		"MatchAllOfRegexp":    utils.MatchAllOfRegexp,
		"MatchAllOfGlob":      utils.MatchAllOfGlob,
		"MatchAllOfSubString": utils.MatchAllOfSubString,
		"MatchAnyOfRegexp":    utils.MatchAnyOfRegexp,
		"MatchAnyOfGlob":      utils.MatchAnyOfGlob,
		"MatchAnyOfSubString": utils.MatchAnyOfSubString,

		"IntersectString":     funk.IntersectString,
		"Subtract":            funk.SubtractString,
		"ToStringSlice":       utils.InterfaceToStringSlice,
		"VersionGreater":      utils.VersionGreater,
		"VersionGreaterEqual": utils.VersionGreaterEqual,
		"VersionEqual":        utils.VersionEqual,
		"VersionLessEqual":    utils.VersionLessEqual,
		"VersionLess":         utils.VersionLess,
	}
)

func init() {
	for k, v := range suspect.GuessExports {
		StringsExport[k] = v
	}
}

func extractValidJson(i interface{}) []string {
	return jsonextractor.ExtractStandardJSON(utils.InterfaceToString(i))
}

func extractJsonEx(i interface{}) ([]string, []string) {
	return jsonextractor.ExtractJSONWithRaw(utils.InterfaceToString(i))
}

func extractDomain(i interface{}) []string {
	return domainextractor.ExtractDomains(utils.InterfaceToString(i))
}

func extractRootDomain(i interface{}) []string {
	return domainextractor.ExtractRootDomains(utils.InterfaceToString(i))
}
