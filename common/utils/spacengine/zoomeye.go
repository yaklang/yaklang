package spacengine

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/zoomeye"
)

func zoomeyeResultToSpacengineList(filter string, result *gjson.Result) []*base.NetSpaceEngineResult {
	rData := result.Get("matches").Array()

	results := make([]*base.NetSpaceEngineResult, len(rData))
	for index, d := range rData {
		dataMap := d.Map()
		rGeoInfo := dataMap["geoinfo"]
		rPortInfo := dataMap["portinfo"]
		rProtocol := dataMap["protocol"]

		isTls := false
		protocol := ""
		host, port := dataMap["ip"].String(), rPortInfo.Get("port").String()
		asn, isp := rGeoInfo.Get("asn").String(), rGeoInfo.Get("isp").String()
		continent, country, city := rGeoInfo.Get("continent.names.zh-CN").String(), rGeoInfo.Get("country.names.zh-CN").String(), rGeoInfo.Get("city.names.zh-CN").String()
		os := rPortInfo.Get("os").String()
		app, version := rPortInfo.Get("app").String(), rPortInfo.Get("version").String()

		latitule, longitude := rGeoInfo.Get("location.lat").Float(), rGeoInfo.Get("location.lon").Float()
		banner := rPortInfo.Get("banner").String()
		title := strings.Join(lo.Map(rPortInfo.Get("title").Array(), func(item gjson.Result, _ int) string { return item.String() }), " | ")

		if rProtocol.Exists() {
			protocol = dataMap["protocol"].Str
		}

		if utils.MatchAnyOfSubString(strings.ToLower(protocol), "https", "tls") {
			isTls = true
		}

		var locations []string
		if continent != "" {
			locations = append(locations, continent)
		}
		if country != "" {
			locations = append(locations, country)
		}
		if city != "" {
			locations = append(locations, city)
		}

		var fps []string
		if os != "" {
			fps = append(fps, os)
		}

		if version != "" {
			fps = append(fps, fmt.Sprintf("%v[%v]", app, version))
		} else {
			fps = append(fps, app)
		}
		fps = utils.RemoveRepeatStringSlice(fps)

		results[index] = &base.NetSpaceEngineResult{
			Addr:            utils.HostPort(host, port),
			FromEngine:      "zoomeye",
			Latitude:        latitule,
			Longitude:       longitude,
			ConfirmHttps:    isTls,
			Host:            host,
			City:            city,
			Asn:             asn,
			Location:        strings.Join(locations, "/"),
			ServiceProvider: isp,
			FromFilter:      filter,
			Fingerprints:    strings.Join(fps, "/"),
			Banner:          utils.ParseStringToVisible(banner),
			HtmlTitle:       title,
		}
	}

	return results
}

func ZoomeyeQuery(key, query string, maxPage, maxRecord int, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	return ZoomeyeQueryWithConfig(key, query, maxPage, maxRecord, nil, domains...)
}

func ZoomeyeQueryWithConfig(key, query string, maxPage, maxRecord int, config *base.QueryConfig, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	if config == nil {
		config = &base.QueryConfig{RandomDelayRange: 3} // 默认3秒延迟，保持兼容性
	}
	var client *zoomeye.ZoomEyeClient
	if len(domains) > 0 && domains[0] != "" {
		client = zoomeye.NewClientEx(key, domains[0])
	} else {
		client = zoomeye.NewClient(key)
	}

	ch := make(chan *base.NetSpaceEngineResult)

	go func() {
		defer close(ch)

		var nextFinished bool
		var count int

		for page := 1; page <= maxPage; page++ {
			if nextFinished {
				break
			}

			log.Infof("Start to query zoomeye for %v", query)
			result, err := client.Query(query, page)
			if err != nil {
				log.Errorf("zoomeye client query next failed: %s", err)
				break
			}

			records := zoomeyeResultToSpacengineList(query, result)
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
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
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
