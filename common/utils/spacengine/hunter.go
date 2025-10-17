package spacengine

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/hunter"
)

func resultToSpacengineList(filter string, result *gjson.Result) []*base.NetSpaceEngineResult {
	rData := result.Get("data.arr").Array()
	results := make([]*base.NetSpaceEngineResult, len(rData))
	for index, d := range rData {
		isTls := false
		dataMap := d.Map()
		webTitle := dataMap["web_title"].String()
		company := dataMap["company"].String()
		os := dataMap["os"].String()
		banner := dataMap["banner"].String()
		port := dataMap["port"].String()
		url := dataMap["url"].String()
		protocol, baseProtocol := dataMap["protocol"].String(), dataMap["base_protocol"].String()
		isp := dataMap["isp"].String()
		country, province, city := dataMap["country"].String(), dataMap["province"].String(), dataMap["city"].String()
		components := dataMap["component"].Array()

		if utils.MatchAnyOfSubString(strings.ToLower(fmt.Sprintf("%s%s", protocol, baseProtocol)), "https", "tls") {
			isTls = true
		}

		host, domain := dataMap["ip"].String(), dataMap["domain"].String()

		if domain != "" {
			host = domain
		}
		var locations []string

		if country != "" {
			locations = append(locations, country)
		}
		if province != "" {
			locations = append(locations, province)
		}
		if city != "" {
			locations = append(locations, city)
		}
		if company != "" {
			locations = append(locations, company)
		}
		var fps []string
		if os != "" {
			fps = append(fps, os)
		}
		for _, c := range components {
			version, name := c.Get("version").String(), c.Get("name").String()
			if version != "" {
				fps = append(fps, fmt.Sprintf("%v[%v]", name, version))
			} else {
				fps = append(fps, name)
			}
		}
		fps = utils.RemoveRepeatStringSlice(fps)

		results[index] = &base.NetSpaceEngineResult{
			Addr:            utils.HostPort(host, port),
			FromEngine:      "hunter",
			HtmlTitle:       webTitle,
			Domains:         domain,
			Province:        province,
			Url:             url,
			ConfirmHttps:    isTls,
			Host:            host,
			City:            city,
			Asn:             baseProtocol,
			Location:        strings.Join(locations, "/"),
			ServiceProvider: isp,
			FromFilter:      filter,
			Fingerprints:    strings.Join(fps, "/"),
			Banner:          utils.ParseStringToVisible(banner),
		}
	}

	return results
}

func HunterQuery(key, query string, maxPage, pageSize, maxRecord int, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	return HunterQueryWithConfig(key, query, maxPage, pageSize, maxRecord, nil, domains...)
}

func HunterQueryWithConfig(key, query string, maxPage, pageSize, maxRecord int, config *base.QueryConfig, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	if config == nil {
		config = &base.QueryConfig{RandomDelayRange: 3} // 默认3秒延迟，保持兼容性
	}
	ch := make(chan *base.NetSpaceEngineResult)
	var client *hunter.HunterClient
	if len(domains) > 0 && domains[0] != "" {
		client = hunter.NewClientEx(key, domains[0])
	} else {
		client = hunter.NewClient(key)
	}
	// too large page size will cause hunter 493 status code
	if pageSize > 10 {
		pageSize = 10
	}

	go func() {
		defer close(ch)

		var nextFinished bool
		var count int
		for page := 1; page <= maxPage; page++ {
			if nextFinished {
				break
			}

			log.Infof("Start to query hunter for %v", query)
			result, err := client.Query(query, page, pageSize)
			if err != nil {
				log.Errorf("hunter client query next failed: %s", err)
				break
			}
			total := gjson.Get(result.Raw, "data.total").Int()
			records := resultToSpacengineList(query, result)

			// 如果当前页没有数据，停止翻页
			if len(records) == 0 {
				nextFinished = true
				break
			}

			for _, record := range records {
				if nextFinished {
					break
				}
				ch <- record

				count++
				if count >= int(total) {
					nextFinished = true
					break
				}

				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
					break
				}
			}

			// 在翻页之间应用随机延迟
			if !nextFinished {
				base.ApplyRandomDelay(config.RandomDelayRange)
			}
		}
	}()
	return ch, nil
}
