package spacengine

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
	"github.com/yaklang/yaklang/common/utils/spacengine/zone"
)

func zoneResultToSpacengineList(filter string, result *gjson.Result) []*base.NetSpaceEngineResult {
	dataArray := result.Get("data").Array()
	results := make([]*base.NetSpaceEngineResult, 0, len(dataArray))

	for _, d := range dataArray {
		dataMap := d.Map()
		ip := dataMap["ip"].String()
		port := dataMap["port"].String()
		urlStr := dataMap["url"].String()
		title := dataMap["title"].String()
		company := dataMap["group"].String() // 0.zone 使用 group 表示公司
		os := dataMap["os"].String()
		cms := dataMap["cms"].String()
		service := dataMap["service"].String()
		city := dataMap["city"].String()
		operator := dataMap["operator"].String()
		components := dataMap["component"].Array()

		isTls := false
		if strings.HasPrefix(strings.ToLower(urlStr), "https://") {
			isTls = true
		}

		host := ip
		domain := dataMap["domain"].String()
		if domain != "" {
			host = domain
		}

		var locations []string
		if company != "" {
			locations = append(locations, company)
		}
		if city != "" {
			locations = append(locations, city)
		}

		var fps []string
		if os != "" {
			fps = append(fps, os)
		}
		if cms != "" {
			fps = append(fps, cms)
		}
		if service != "" {
			fps = append(fps, service)
		}
		for _, c := range components {
			version, name := c.Get("version").String(), c.Get("name").String()
			if version != "" {
				fps = append(fps, fmt.Sprintf("%v[%v]", name, version))
			} else if name != "" {
				fps = append(fps, name)
			}
		}
		fps = utils.RemoveRepeatStringSlice(fps)

		banner := dataMap["banner"].String()
		if banner == "" {
			banner = dataMap["banner_os"].String()
		}

		results = append(results, &base.NetSpaceEngineResult{
			Addr:            utils.HostPort(host, port),
			FromEngine:      "zone",
			HtmlTitle:       title,
			Domains:         domain,
			City:            city,
			Url:             urlStr,
			ConfirmHttps:    isTls,
			Host:            host,
			Location:        strings.Join(locations, "/"),
			ServiceProvider: operator,
			FromFilter:      filter,
			Fingerprints:    strings.Join(fps, "/"),
			Banner:          utils.ParseStringToVisible(banner),
		})
	}

	return results
}

func ZoneQuery(key string, query string, maxPage, maxRecord int, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	return ZoneQueryWithConfig(key, query, maxPage, maxRecord, nil, domains...)
}

func ZoneQueryWithConfig(key string, query string, maxPage, maxRecord int, config *base.QueryConfig, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	if config == nil {
		config = &base.QueryConfig{RandomDelayRange: 2} // 0.zone 建议 2 秒延迟避免限频
	}
	ch := make(chan *base.NetSpaceEngineResult)
	var client *zone.ZoneClient
	if len(domains) > 0 && domains[0] != "" {
		client = zone.NewClientEx(key, domains[0])
	} else {
		client = zone.NewClient(key)
	}

	// 0.zone 每页最大 40 条
	pageSize := 40

	go func() {
		defer close(ch)

		var nextFinished bool
		var count int
		for page := 1; page <= maxPage; page++ {
			if nextFinished {
				break
			}

			log.Infof("Start to query zone for %v", query)
			result, err := client.Query(query, "site", page, pageSize)
			if err != nil {
				log.Errorf("zone client query failed: %s", err)
				break
			}

			records := zoneResultToSpacengineList(query, result)

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
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
					break
				}
			}

			// 当前页不足 pageSize 说明没有更多数据
			if len(records) < pageSize {
				nextFinished = true
			}

			if !nextFinished {
				base.ApplyRandomDelay(config.RandomDelayRange)
			}
		}
	}()
	return ch, nil
}
